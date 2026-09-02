package sync

import (
	"bufio"
	"bytes"
	"context"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"strconv"
	"sync"
	"time"
)

// H1LastByte is the best-effort HTTP/1.1 synchronization strategy: one
// TCP(+TLS) connection per instance, written up to its final byte, then
// every instance's withheld final byte is flushed back-to-back
// (Design/ARCHITECTURE.md §2.6). Jitter is higher than H2SinglePacket by
// design — this is the documented, honest trade-off of HTTP/1.1 sync.
type H1LastByte struct {
	Dialer Dialer
}

// NewH1LastByte builds an H1LastByte strategy dialing through d.
func NewH1LastByte(d Dialer) *H1LastByte { return &H1LastByte{Dialer: d} }

// Name identifies this strategy in trials.jsonl (sync_strategy field).
func (h *H1LastByte) Name() string { return "H1LastByte" }

// Burst implements SyncStrategy.
func (h *H1LastByte) Burst(ctx context.Context, reqs []PreparedRequest) ([]Response, error) {
	type opened struct {
		conn net.Conn
		last byte
	}
	opens := make([]opened, len(reqs))
	for i, r := range reqs {
		u, err := url.Parse(r.URL)
		if err != nil {
			return nil, fmt.Errorf("h1lastbyte: invalid URL %q: %w", r.URL, err)
		}
		raw := buildRawHTTP1Request(r, u)
		conn, err := dialConn(ctx, h.Dialer, u, "http/1.1")
		if err != nil {
			return nil, fmt.Errorf("h1lastbyte: dial instance %d: %w", i, err)
		}
		if len(raw) == 0 {
			_ = conn.Close()
			return nil, fmt.Errorf("h1lastbyte: instance %d built an empty request", i)
		}
		head, last := raw[:len(raw)-1], raw[len(raw)-1]
		if _, err := conn.Write(head); err != nil {
			_ = conn.Close()
			return nil, fmt.Errorf("h1lastbyte: write head instance %d: %w", i, err)
		}
		opens[i] = opened{conn: conn, last: last}
	}

	// Release: write every instance's withheld final byte back-to-back so
	// the bursts land as close together as HTTP/1.1 allows.
	for _, o := range opens {
		_, _ = o.conn.Write([]byte{o.last})
	}

	responses := make([]Response, len(reqs))
	var wg sync.WaitGroup
	for i, o := range opens {
		wg.Add(1)
		go func(i int, conn net.Conn) {
			defer wg.Done()
			defer func() { _ = conn.Close() }()
			responses[i] = readHTTP1Response(conn)
		}(i, o.conn)
	}
	wg.Wait()
	return responses, nil
}

func buildRawHTTP1Request(r PreparedRequest, u *url.URL) []byte {
	var b bytes.Buffer
	path := u.RequestURI()
	if path == "" {
		path = "/"
	}
	fmt.Fprintf(&b, "%s %s HTTP/1.1\r\n", r.Method, path)
	fmt.Fprintf(&b, "Host: %s\r\n", u.Host)
	for k, v := range r.Headers {
		fmt.Fprintf(&b, "%s: %s\r\n", k, v)
	}
	if len(r.Body) > 0 {
		fmt.Fprintf(&b, "Content-Length: %d\r\n", len(r.Body))
	}
	b.WriteString("Connection: close\r\n\r\n")
	b.Write(r.Body)
	return b.Bytes()
}

// firstByteReader records the time of the first successful Read, so the
// caller can timestamp response arrival independent of how long body
// reading takes.
type firstByteReader struct {
	r        io.Reader
	arrived  time.Time
	recorded bool
}

func (fr *firstByteReader) Read(p []byte) (int, error) {
	n, err := fr.r.Read(p)
	if n > 0 && !fr.recorded {
		fr.arrived = time.Now()
		fr.recorded = true
	}
	return n, err
}

func readHTTP1Response(conn net.Conn) Response {
	fr := &firstByteReader{r: conn}
	br := bufio.NewReader(fr)
	resp, err := http.ReadResponse(br, nil)
	if err != nil {
		return Response{Err: fmt.Errorf("h1lastbyte: read response: %w", err), ArrivedAt: fr.arrived}
	}
	defer func() { _ = resp.Body.Close() }()
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return Response{Err: fmt.Errorf("h1lastbyte: read body: %w", err), ArrivedAt: fr.arrived}
	}
	headers := make(map[string]string, len(resp.Header))
	for k := range resp.Header {
		headers[k] = resp.Header.Get(k)
	}
	if resp.ContentLength >= 0 {
		headers["Content-Length"] = strconv.FormatInt(resp.ContentLength, 10)
	}
	return Response{StatusCode: resp.StatusCode, Headers: headers, Body: body, ArrivedAt: fr.arrived}
}
