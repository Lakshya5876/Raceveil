package sync

import (
	"context"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"testing"
)

// plainDialer dials a local test server directly, bypassing
// security/scope.Guard — used only in tests, which don't need scope
// enforcement against RaceVeil's own test fixtures.
type plainDialer struct{}

func (plainDialer) Dial(ctx context.Context, network, addr string) (net.Conn, error) {
	var d net.Dialer
	return d.DialContext(ctx, network, addr)
}

func TestH1LastByte_Burst(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("ok"))
	}))
	defer srv.Close()

	strategy := NewH1LastByte(plainDialer{})
	reqs := make([]PreparedRequest, 5)
	for i := range reqs {
		reqs[i] = PreparedRequest{Method: "GET", URL: srv.URL + "/"}
	}
	responses, err := strategy.Burst(context.Background(), reqs)
	if err != nil {
		t.Fatalf("Burst: %v", err)
	}
	if len(responses) != len(reqs) {
		t.Fatalf("got %d responses, want %d", len(responses), len(reqs))
	}
	for i, r := range responses {
		if r.Err != nil {
			t.Errorf("response %d: %v", i, r.Err)
		}
		if r.StatusCode != http.StatusOK {
			t.Errorf("response %d: status = %d, want 200", i, r.StatusCode)
		}
	}
	if got := strategy.Name(); got != "H1LastByte" {
		t.Errorf("Name() = %q, want H1LastByte", got)
	}
	t.Logf("H1LastByte dispersion: %.3fms (best-effort, no hard bound)", Dispersion(responses))
}

func TestH2SinglePacket_Burst(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("/echo", func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write(body)
	})
	srv := httptest.NewUnstartedServer(mux)
	protocols := new(http.Protocols)
	protocols.SetUnencryptedHTTP2(true)
	protocols.SetHTTP1(true)
	srv.Config.Protocols = protocols
	srv.Start()
	defer srv.Close()

	strategy := NewH2SinglePacket(plainDialer{})
	const n = 5
	reqs := make([]PreparedRequest, n)
	for i := range reqs {
		reqs[i] = PreparedRequest{
			Method:  "POST",
			URL:     srv.URL + "/echo",
			Headers: map[string]string{"content-type": "text/plain"},
			Body:    []byte(fmt.Sprintf("instance-%d", i)),
		}
	}
	responses, err := strategy.Burst(context.Background(), reqs)
	if err != nil {
		t.Fatalf("Burst: %v", err)
	}
	if len(responses) != n {
		t.Fatalf("got %d responses, want %d", len(responses), n)
	}
	for i, r := range responses {
		if r.Err != nil {
			t.Errorf("response %d: %v", i, r.Err)
			continue
		}
		if r.StatusCode != http.StatusOK {
			t.Errorf("response %d: status = %d, want 200", i, r.StatusCode)
		}
		want := fmt.Sprintf("instance-%d", i)
		if string(r.Body) != want {
			t.Errorf("response %d: body = %q, want %q", i, r.Body, want)
		}
	}
	if got := strategy.Name(); got != "H2SinglePacket" {
		t.Errorf("Name() = %q, want H2SinglePacket", got)
	}
	dispersion := Dispersion(responses)
	t.Logf("H2SinglePacket dispersion: %.3fms", dispersion)
	// Design/TEST_PLAN.md Layer 2 wants sub-ms on loopback; the top-level
	// task's Windows environment notes explicitly say coarser timer
	// granularity is a measurement to record, not a hard gate here.
}

func TestDispersion_EmptyAndSingle(t *testing.T) {
	if got := Dispersion(nil); got != 0 {
		t.Errorf("Dispersion(nil) = %v, want 0", got)
	}
	if got := Dispersion([]Response{{}}); got != 0 {
		t.Errorf("Dispersion(single all-error) = %v, want 0", got)
	}
}
