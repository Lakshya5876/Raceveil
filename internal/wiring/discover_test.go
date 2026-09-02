package wiring

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"

	"github.com/Lakshya5876/Raceveil/internal/domain"
)

// discoverySite is a miniature app with three endpoints that exercise the
// whole discovery path end to end:
//
//   - POST /coupon/redeem — a real check-then-act race behind a limit
//   - POST /coupon/issue  — the setup endpoint that mints a fresh code
//   - POST /comment       — legitimately non-idempotent, no limit at all;
//     the false-positive gate must drop it rather than test it
type discoverySite struct {
	mu    sync.Mutex
	codes map[string]bool
	seq   int
}

func newDiscoverySite(t *testing.T) *httptest.Server {
	t.Helper()
	site := &discoverySite{codes: map[string]bool{}}
	mux := http.NewServeMux()

	mux.HandleFunc("/", func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "text/html")
		_, _ = w.Write([]byte(`<html><body>
			<script>
				fetch("/coupon/issue", {method:"POST"});
				fetch("/coupon/redeem", {method:"POST"});
				fetch("/comment", {method:"POST"});
			</script>
		</body></html>`))
	})

	mux.HandleFunc("/coupon/issue", func(w http.ResponseWriter, _ *http.Request) {
		site.mu.Lock()
		site.seq++
		code := fmt.Sprintf("code-%d", site.seq)
		site.codes[code] = true
		site.mu.Unlock()
		w.Header().Set("Content-Type", "application/json")
		_, _ = fmt.Fprintf(w, `{"code":%q}`, code)
	})

	mux.HandleFunc("/coupon/redeem", func(w http.ResponseWriter, r *http.Request) {
		code := extractCodeField(r)
		site.mu.Lock()
		valid := site.codes[code]
		site.mu.Unlock()
		if !valid {
			http.Error(w, "already redeemed", http.StatusConflict)
			return
		}
		// Deliberate check-then-act window, exactly like the corpus fixtures.
		site.mu.Lock()
		site.codes[code] = false
		site.mu.Unlock()
		w.Header().Set("Content-Type", "application/json")
		_, _ = fmt.Fprintf(w, `{"status":"redeemed"}`)
	})

	// No limit, ever: posting a comment N times is correct behavior.
	mux.HandleFunc("/comment", func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = fmt.Fprintf(w, `{"status":"created"}`)
	})

	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)
	return srv
}

func extractCodeField(r *http.Request) string {
	buf := make([]byte, 512)
	n, _ := r.Body.Read(buf)
	body := string(buf[:n])
	const key = `"code":"`
	i := strings.Index(body, key)
	if i < 0 {
		return ""
	}
	rest := body[i+len(key):]
	if j := strings.Index(rest, `"`); j >= 0 {
		return rest[:j]
	}
	return ""
}

func discoveryFiles(t *testing.T, addr string, port int) (scopePath, sessionPath string) {
	t.Helper()
	dir := t.TempDir()
	scopePath = writeFile(t, dir, "scope.yaml", fmt.Sprintf(scopeTemplate, addr, port))
	sessionPath = writeFile(t, dir, "session.json", sessionJSON)
	return scopePath, sessionPath
}

func TestRunDiscoverScan_DropsEndpointsWithNoInvariant(t *testing.T) {
	srv := newDiscoverySite(t)
	addr := strings.TrimPrefix(srv.URL, "http://")
	scopePath, sessionPath := discoveryFiles(t, addr, mustSplitPort(t, addr))

	summary, err := RunDiscoverScan(context.Background(), DiscoverOptions{
		Target: srv.URL, ScopePath: scopePath, AuthPath: sessionPath,
		OutDir: filepath.Join(t.TempDir(), "run"), SafeMode: true, Mode: "crawl",
	})
	if err != nil {
		t.Fatalf("RunDiscoverScan: %v", err)
	}

	if summary.Endpoints == 0 {
		t.Fatal("expected the crawler to capture at least one endpoint")
	}
	if summary.Workflows == 0 {
		t.Fatal("expected at least one mutating workflow to be extracted")
	}

	// The false-positive gate is the point: /comment always succeeds, so it
	// establishes no Invariant and must be skipped, never tested.
	var commentSkipped bool
	for _, s := range summary.SkippedNoLimit {
		if strings.Contains(s, "/comment") {
			commentSkipped = true
		}
	}
	if !commentSkipped {
		t.Errorf("a non-idempotent endpoint with no limit must be skipped by the invariant gate; skipped=%v", summary.SkippedNoLimit)
	}
	for _, f := range summary.Findings {
		if f.Finding != nil && strings.Contains(ActRequestLine(f.Finding.Workflow), "/comment") {
			t.Error("/comment must never produce a finding — it has no invariant to violate")
		}
	}
}

func TestRunDiscoverScan_WritesRunDirectory(t *testing.T) {
	srv := newDiscoverySite(t)
	addr := strings.TrimPrefix(srv.URL, "http://")
	scopePath, sessionPath := discoveryFiles(t, addr, mustSplitPort(t, addr))

	summary, err := RunDiscoverScan(context.Background(), DiscoverOptions{
		Target: srv.URL, ScopePath: scopePath, AuthPath: sessionPath,
		OutDir: filepath.Join(t.TempDir(), "run"), SafeMode: true, Mode: "crawl",
	})
	if err != nil {
		t.Fatalf("RunDiscoverScan: %v", err)
	}
	for _, name := range []string{"config.yaml", "requests.jsonl"} {
		if _, err := os.Stat(filepath.Join(summary.RunDir, name)); err != nil {
			t.Errorf("expected %s in the run directory: %v", name, err)
		}
	}

	// The run directory must be readable back as a report.
	view, err := LoadRunView(summary.RunDir)
	if err != nil {
		t.Fatalf("LoadRunView: %v", err)
	}
	if view.RunDir != summary.RunDir {
		t.Errorf("RunView.RunDir = %q, want %q", view.RunDir, summary.RunDir)
	}
}

func TestRunDiscoverScan_ImportModeRequiresImportFile(t *testing.T) {
	srv := newDiscoverySite(t)
	addr := strings.TrimPrefix(srv.URL, "http://")
	scopePath, sessionPath := discoveryFiles(t, addr, mustSplitPort(t, addr))

	_, err := RunDiscoverScan(context.Background(), DiscoverOptions{
		Target: srv.URL, ScopePath: scopePath, AuthPath: sessionPath,
		OutDir: filepath.Join(t.TempDir(), "run"), SafeMode: true, Mode: "import",
	})
	if err == nil {
		t.Fatal("expected --discovery import without --import to be an error")
	}
}

func TestRunDiscoverScan_UserInvariantIsAuthoritative(t *testing.T) {
	srv := newDiscoverySite(t)
	addr := strings.TrimPrefix(srv.URL, "http://")
	scopePath, sessionPath := discoveryFiles(t, addr, mustSplitPort(t, addr))

	// Declaring an invariant on /comment overrides inference: Level 5 is
	// authoritative, so it gets tested even though probing would drop it.
	invariants := writeFile(t, t.TempDir(), "invariants.yaml", `
- match: {method: POST, path: /comment}
  invariant: {type: max_successes, value: 1}
  success_when: {status: 200}
  reject_when: {status: 409}
`)

	summary, err := RunDiscoverScan(context.Background(), DiscoverOptions{
		Target: srv.URL, ScopePath: scopePath, AuthPath: sessionPath,
		OutDir: filepath.Join(t.TempDir(), "run"), SafeMode: true, Mode: "crawl",
		InvariantsRef: invariants,
	})
	if err != nil {
		t.Fatalf("RunDiscoverScan: %v", err)
	}
	if summary.CandidatesTested == 0 {
		t.Error("a declared Level 5 invariant should promote its endpoint to a tested Candidate")
	}
}

func TestLoadUserInvariants_ParsesDeclarations(t *testing.T) {
	path := writeFile(t, t.TempDir(), "invariants.yaml", `
- match: {method: POST, path: /coupon/redeem}
  invariant: {type: max_successes, value: 1}
  success_when: {status: 200}
  reject_when: {status: 409}
  post_state_probe: {method: GET, path: /account/redemptions, extract: "$.count"}
`)
	got, err := loadUserInvariants(path)
	if err != nil {
		t.Fatalf("loadUserInvariants: %v", err)
	}
	if len(got) != 1 {
		t.Fatalf("expected 1 declaration, got %d", len(got))
	}
	d := got[0]
	if d.Match.Path != "/coupon/redeem" || d.Invariant.Type != domain.InvariantMaxSuccesses {
		t.Errorf("unexpected declaration: %+v", d)
	}
	if d.PostStateProbe == nil || d.PostStateProbe.Extract != "$.count" {
		t.Errorf("post_state_probe not parsed: %+v", d.PostStateProbe)
	}
}

func TestLoadUserInvariants_EmptyPathIsNotAnError(t *testing.T) {
	got, err := loadUserInvariants("")
	if err != nil || got != nil {
		t.Fatalf("no invariants file should be a no-op, got %v / %v", got, err)
	}
}

func TestGuessBindField(t *testing.T) {
	cases := map[string]string{
		"/coupon/issue":      "code",
		"/cart/create":       "id",
		"/inventory/restock": "sku",
		"/thing/new":         "id",
	}
	for path, want := range cases {
		got := guessBindField(domain.CapturedRequest{Endpoint: domain.Endpoint{Path: path}})
		if got != want {
			t.Errorf("guessBindField(%q) = %q, want %q", path, got, want)
		}
	}
}
