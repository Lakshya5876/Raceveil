package sync

import (
	"bytes"
	"context"
	"fmt"
	"net"
	"net/url"
	"strconv"
	"strings"
	"time"

	"golang.org/x/net/http2"
	"golang.org/x/net/http2/hpack"
)

// H2SinglePacket is the single-packet HTTP/2 synchronization strategy: open
// one h2 connection, send every instance's HEADERS (and DATA up to the
// final frame) withholding the last frame, then flush all final frames
// back-to-back so they coalesce into as few TCP segments as possible
// (TCP_NODELAY). Reimplemented from the published last-frame-synchronization
// technique under Apache-2.0 (Context/DECISIONS.md ADR-009).
type H2SinglePacket struct {
	Dialer Dialer
}

// NewH2SinglePacket builds an H2SinglePacket strategy dialing through d.
func NewH2SinglePacket(d Dialer) *H2SinglePacket { return &H2SinglePacket{Dialer: d} }

// Name identifies this strategy in trials.jsonl (sync_strategy field).
func (h *H2SinglePacket) Name() string { return "H2SinglePacket" }

// Burst implements SyncStrategy. All instances must target the same
// host:port (they are all Act-Request replicas of one Workflow), so one
// connection serves the whole burst.
func (h *H2SinglePacket) Burst(ctx context.Context, reqs []PreparedRequest) ([]Response, error) {
	if len(reqs) == 0 {
		return nil, nil
	}
	u, err := url.Parse(reqs[0].URL)
	if err != nil {
		return nil, fmt.Errorf("h2singlepacket: invalid URL %q: %w", reqs[0].URL, err)
	}
	conn, err := dialConn(ctx, h.Dialer, u, "h2")
	if err != nil {
		return nil, fmt.Errorf("h2singlepacket: dial: %w", err)
	}
	defer func() { _ = conn.Close() }()
	if tc, ok := conn.(*net.TCPConn); ok {
		_ = tc.SetNoDelay(true)
	}

	framer, err := handshakeH2(conn)
	if err != nil {
		return nil, err
	}

	streamIDs, err := sendWithheldHeaders(framer, reqs)
	if err != nil {
		return nil, err
	}
	if err := releaseFinalFrames(framer, streamIDs, reqs); err != nil {
		return nil, err
	}
	return readH2Responses(ctx, framer, streamIDs)
}

// handshakeH2 writes the client preface and initial SETTINGS, then waits for
// and acknowledges the server's SETTINGS frame — the minimal handshake
// needed before sending request frames.
func handshakeH2(conn net.Conn) (*http2.Framer, error) {
	if _, err := conn.Write([]byte(http2.ClientPreface)); err != nil {
		return nil, fmt.Errorf("h2singlepacket: write client preface: %w", err)
	}
	framer := http2.NewFramer(conn, conn)
	if err := framer.WriteSettings(); err != nil {
		return nil, fmt.Errorf("h2singlepacket: write initial settings: %w", err)
	}
	if err := awaitServerSettings(framer); err != nil {
		return nil, fmt.Errorf("h2singlepacket: handshake: %w", err)
	}
	return framer, nil
}

// sendWithheldHeaders sends every instance's HEADERS frame (Design/
// ARCHITECTURE.md §2.6 phase 1), withholding the final DATA frame, and
// returns the stream ID assigned to each instance in request order.
func sendWithheldHeaders(framer *http2.Framer, reqs []PreparedRequest) ([]uint32, error) {
	streamIDs := make([]uint32, len(reqs))
	var hbuf bytes.Buffer
	enc := hpack.NewEncoder(&hbuf)
	for i, r := range reqs {
		ru, err := url.Parse(r.URL)
		if err != nil {
			return nil, fmt.Errorf("h2singlepacket: invalid URL for instance %d: %w", i, err)
		}
		streamID := uint32(2*i + 1) // client-initiated streams are odd
		streamIDs[i] = streamID

		hbuf.Reset()
		writeRequestHeaderBlock(enc, r.Method, ru, r.Headers)
		if err := framer.WriteHeaders(http2.HeadersFrameParam{
			StreamID:      streamID,
			BlockFragment: hbuf.Bytes(),
			EndHeaders:    true,
			EndStream:     len(r.Body) == 0,
		}); err != nil {
			return nil, fmt.Errorf("h2singlepacket: write headers instance %d: %w", i, err)
		}
	}
	return streamIDs, nil
}

// releaseFinalFrames writes every instance's withheld final DATA frame
// back-to-back (Design/ARCHITECTURE.md §2.6 phase 2 — the release).
func releaseFinalFrames(framer *http2.Framer, streamIDs []uint32, reqs []PreparedRequest) error {
	for i, r := range reqs {
		if len(r.Body) == 0 {
			continue
		}
		if err := framer.WriteData(streamIDs[i], true, r.Body); err != nil {
			return fmt.Errorf("h2singlepacket: write data instance %d: %w", i, err)
		}
	}
	return nil
}

func awaitServerSettings(framer *http2.Framer) error {
	for {
		f, err := framer.ReadFrame()
		if err != nil {
			return fmt.Errorf("waiting for server SETTINGS: %w", err)
		}
		sf, ok := f.(*http2.SettingsFrame)
		if !ok {
			continue // ignore any other frame before the handshake completes
		}
		if sf.IsAck() {
			continue
		}
		return framer.WriteSettingsAck()
	}
}

func writeRequestHeaderBlock(enc *hpack.Encoder, method string, u *url.URL, headers map[string]string) {
	scheme := u.Scheme
	if scheme == "" {
		scheme = "http"
	}
	path := u.RequestURI()
	if path == "" {
		path = "/"
	}
	_ = enc.WriteField(hpack.HeaderField{Name: ":method", Value: method})
	_ = enc.WriteField(hpack.HeaderField{Name: ":scheme", Value: scheme})
	_ = enc.WriteField(hpack.HeaderField{Name: ":authority", Value: u.Host})
	_ = enc.WriteField(hpack.HeaderField{Name: ":path", Value: path})
	for k, v := range headers {
		_ = enc.WriteField(hpack.HeaderField{Name: strings.ToLower(k), Value: v})
	}
}

// h2ReadState accumulates per-stream response data as frames arrive on the
// shared connection, so readH2Responses' loop stays a flat dispatch.
type h2ReadState struct {
	idx       map[uint32]int
	responses []Response
	done      []bool
	remaining int
	fields    map[uint32][]hpack.HeaderField
	curStream uint32
	decoder   *hpack.Decoder
}

func newH2ReadState(streamIDs []uint32) *h2ReadState {
	s := &h2ReadState{
		idx:       make(map[uint32]int, len(streamIDs)),
		responses: make([]Response, len(streamIDs)),
		done:      make([]bool, len(streamIDs)),
		remaining: len(streamIDs),
		fields:    make(map[uint32][]hpack.HeaderField),
	}
	for i, id := range streamIDs {
		s.idx[id] = i
	}
	s.decoder = hpack.NewDecoder(4096, func(f hpack.HeaderField) {
		s.fields[s.curStream] = append(s.fields[s.curStream], f)
	})
	return s
}

func (s *h2ReadState) markDone(i int) {
	if !s.done[i] {
		s.done[i] = true
		s.remaining--
	}
}

func (s *h2ReadState) handleHeaders(fr *http2.HeadersFrame) error {
	i, ok := s.idx[fr.StreamID]
	if !ok {
		return nil
	}
	if s.responses[i].ArrivedAt.IsZero() {
		s.responses[i].ArrivedAt = time.Now()
	}
	s.curStream = fr.StreamID
	if _, err := s.decoder.Write(fr.HeaderBlockFragment()); err != nil {
		return fmt.Errorf("h2singlepacket: hpack decode: %w", err)
	}
	applyStatusFields(&s.responses[i], s.fields[fr.StreamID])
	if fr.StreamEnded() {
		s.markDone(i)
	}
	return nil
}

func (s *h2ReadState) handleData(fr *http2.DataFrame) {
	i, ok := s.idx[fr.StreamID]
	if !ok {
		return
	}
	s.responses[i].Body = append(s.responses[i].Body, fr.Data()...)
	if fr.StreamEnded() {
		s.markDone(i)
	}
}

func (s *h2ReadState) handleReset(fr *http2.RSTStreamFrame) {
	i, ok := s.idx[fr.StreamID]
	if !ok || s.done[i] {
		return
	}
	s.responses[i].Err = fmt.Errorf("h2singlepacket: stream %d reset: %v", fr.StreamID, fr.ErrCode)
	s.markDone(i)
}

// dispatch handles one received frame, returning an error only for
// connection-fatal conditions (HPACK corruption, GOAWAY).
func (s *h2ReadState) dispatch(framer *http2.Framer, f http2.Frame) error {
	switch fr := f.(type) {
	case *http2.HeadersFrame:
		return s.handleHeaders(fr)
	case *http2.DataFrame:
		s.handleData(fr)
	case *http2.SettingsFrame:
		if !fr.IsAck() {
			_ = framer.WriteSettingsAck()
		}
	case *http2.RSTStreamFrame:
		s.handleReset(fr)
	case *http2.GoAwayFrame:
		return fmt.Errorf("h2singlepacket: server sent GOAWAY: %v", fr.ErrCode)
	}
	return nil // ignore WINDOW_UPDATE, PING, PRIORITY, etc.
}

func readH2Responses(ctx context.Context, framer *http2.Framer, streamIDs []uint32) ([]Response, error) {
	s := newH2ReadState(streamIDs)
	for s.remaining > 0 {
		select {
		case <-ctx.Done():
			return s.responses, ctx.Err()
		default:
		}
		f, err := framer.ReadFrame()
		if err != nil {
			return s.responses, fmt.Errorf("h2singlepacket: read frame: %w", err)
		}
		if err := s.dispatch(framer, f); err != nil {
			return s.responses, err
		}
	}
	return s.responses, nil
}

func applyStatusFields(resp *Response, fields []hpack.HeaderField) {
	resp.Headers = make(map[string]string, len(fields))
	for _, f := range fields {
		if f.Name == ":status" {
			if code, err := strconv.Atoi(f.Value); err == nil {
				resp.StatusCode = code
			}
			continue
		}
		resp.Headers[f.Name] = f.Value
	}
}
