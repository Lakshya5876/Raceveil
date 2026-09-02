package crawl

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/Lakshya5876/Raceveil/internal/domain"
)

type allowAllGuard struct{}

func (allowAllGuard) Check(string, string) error { return nil }

type denyAllGuard struct{}

func (denyAllGuard) Check(string, string) error { return errors.New("out of scope") }

const indexHTML = `<html><body>
  <a href="/cart">cart</a>
  <a href="/coupon">coupon</a>
  <a href="https://evil.example.com/other">offsite</a>
  <a href="#anchor">anchor</a>
  <a href="javascript:void(0)">js</a>
  <form method="POST" action="/coupon/redeem">
    <input name="code" />
    <input name="csrf" />
  </form>
  <script>
    fetch("/api/checkout", {method:"POST"});
    const u = '/api/reserve';
    loadStyles("/static/app.css");
  </script>
</body></html>`

func newTestSite(t *testing.T) *httptest.Server {
	t.Helper()
	mux := http.NewServeMux()
	mux.HandleFunc("/", func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "text/html")
		_, _ = w.Write([]byte(indexHTML))
	})
	mux.HandleFunc("/cart", func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte("<html><body>cart</body></html>"))
	})
	mux.HandleFunc("/coupon", func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte("<html><body>coupon</body></html>"))
	})
	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)
	return srv
}

// crawlFixture runs a crawl against the local test site and indexes the
// result by "METHOD path" so assertions stay readable.
func crawlFixture(t *testing.T) map[string]domain.CapturedRequest {
	t.Helper()
	srv := newTestSite(t)
	c := New(allowAllGuard{}, Options{MaxPages: 10, MaxDepth: 2})
	got, err := c.Crawl(context.Background(), srv.URL+"/", domain.Session{ID: "s1"})
	if err != nil {
		t.Fatalf("Crawl: %v", err)
	}
	index := make(map[string]domain.CapturedRequest, len(got))
	for _, r := range got {
		index[r.Endpoint.Method+" "+r.Endpoint.Path] = r
	}
	return index
}

func TestCrawl_CapturesFormSubmission(t *testing.T) {
	got := crawlFixture(t)
	form, ok := got[http.MethodPost+" /coupon/redeem"]
	if !ok {
		t.Fatalf("expected the <form> POST to be captured, got keys %v", keysOf(got))
	}
	if !strings.Contains(form.Body, "code=") {
		t.Errorf("form body should template its fields, got %q", form.Body)
	}
}

func TestCrawl_CapturesInlineJSEndpoints(t *testing.T) {
	got := crawlFixture(t)
	for _, want := range []string{http.MethodPost + " /api/checkout", http.MethodPost + " /api/reserve"} {
		if _, ok := got[want]; !ok {
			t.Errorf("expected inline-JS endpoint %q to be captured, got keys %v", want, keysOf(got))
		}
	}
}

func TestCrawl_FollowsSameHostLinks(t *testing.T) {
	got := crawlFixture(t)
	if _, ok := got[http.MethodGet+" /cart"]; !ok {
		t.Errorf("expected same-host links to be followed, got keys %v", keysOf(got))
	}
}

func TestCrawl_StaysOnHostAndSkipsAssets(t *testing.T) {
	for key := range crawlFixture(t) {
		if strings.Contains(key, "evil.example.com") {
			t.Errorf("crawler must not leave the start host: %s", key)
		}
		if strings.HasSuffix(key, ".css") || strings.HasSuffix(key, ".js") {
			t.Errorf("static assets must not be captured as candidates: %s", key)
		}
	}
}

func keysOf(m map[string]domain.CapturedRequest) []string {
	out := make([]string, 0, len(m))
	for k := range m {
		out = append(out, k)
	}
	return out
}

func TestCrawl_RespectsGuardRefusal(t *testing.T) {
	srv := newTestSite(t)
	c := New(denyAllGuard{}, Options{})
	got, err := c.Crawl(context.Background(), srv.URL+"/", domain.Session{})
	if err != nil {
		t.Fatalf("Crawl: %v", err)
	}
	if len(got) != 0 {
		t.Fatalf("a Guard that refuses everything must yield no captured requests, got %d", len(got))
	}
}

func TestCrawl_RespectsMaxPages(t *testing.T) {
	srv := newTestSite(t)
	c := New(allowAllGuard{}, Options{MaxPages: 1, MaxDepth: 5})
	got, err := c.Crawl(context.Background(), srv.URL+"/", domain.Session{})
	if err != nil {
		t.Fatalf("Crawl: %v", err)
	}
	pageGets := 0
	for _, r := range got {
		if r.Endpoint.Method == http.MethodGet && strings.HasPrefix(r.ID, "get_") {
			pageGets++
		}
	}
	if pageGets != 1 {
		t.Errorf("MaxPages=1 should fetch exactly one page, fetched %d", pageGets)
	}
}

func TestImportHAR(t *testing.T) {
	har := `{"log":{"entries":[
	  {"request":{"method":"POST","url":"http://x.test/coupon/redeem","headers":[
	     {"name":"Content-Type","value":"application/json"},
	     {"name":"Authorization","value":"Bearer supersecret"},
	     {"name":"Cookie","value":"session=abc"}],
	   "postData":{"mimeType":"application/json","text":"{\"code\":\"X\"}"}},
	   "response":{"status":200}},
	  {"request":{"method":"GET","url":"http://x.test/app.js","headers":[],"postData":{}},"response":{"status":200}},
	  {"request":{"method":"POST","url":"http://x.test/fails","headers":[],"postData":{}},"response":{"status":500}}
	]}}`
	path := filepath.Join(t.TempDir(), "traffic.har")
	if err := os.WriteFile(path, []byte(har), 0o600); err != nil {
		t.Fatalf("write har: %v", err)
	}

	got, err := ImportHAR(path)
	if err != nil {
		t.Fatalf("ImportHAR: %v", err)
	}
	if len(got) != 1 {
		t.Fatalf("expected 1 usable entry (asset and 5xx dropped), got %d: %+v", len(got), got)
	}
	r := got[0]
	if r.Endpoint.Path != "/coupon/redeem" || r.Endpoint.Method != "POST" {
		t.Errorf("unexpected endpoint: %+v", r.Endpoint)
	}
	if r.Body != `{"code":"X"}` {
		t.Errorf("body = %q", r.Body)
	}
	for name := range r.Headers {
		if isCredentialHeader(name) {
			t.Errorf("credential header %q must never be imported into a captured request", name)
		}
	}
}

func TestImportHAR_RejectsNonJSON(t *testing.T) {
	path := filepath.Join(t.TempDir(), "bad.har")
	if err := os.WriteFile(path, []byte("not json at all"), 0o600); err != nil {
		t.Fatalf("write: %v", err)
	}
	if _, err := ImportHAR(path); err == nil {
		t.Fatal("expected an error for a non-JSON HAR")
	}
}

func TestParseOpenAPI_V3UsesServersOrigin(t *testing.T) {
	v3 := `{"openapi":"3.0.0","servers":[{"url":"http://api.test"}],
	  "paths":{"/coupon/redeem":{"post":{"operationId":"redeem"}},"/health":{"get":{}}}}`
	got, err := parseOpenAPI([]byte(v3), "")
	if err != nil {
		t.Fatalf("parseOpenAPI v3: %v", err)
	}
	if len(got) != 2 {
		t.Fatalf("expected 2 operations, got %d", len(got))
	}
	post, ok := findOperation(got, "POST", "/coupon/redeem")
	if !ok {
		t.Fatal("expected the POST operation to be imported")
	}
	if post.URL != "http://api.test/coupon/redeem" {
		t.Errorf("URL = %q, want origin from servers[0]", post.URL)
	}
	if post.Headers["content-type"] != "application/json" {
		t.Error("a mutating operation should default to a JSON body")
	}
}

func TestParseOpenAPI_Swagger2AppliesBasePath(t *testing.T) {
	swagger2 := `{"swagger":"2.0","basePath":"/v1","paths":{"/orders":{"post":{}}}}`
	got, err := parseOpenAPI([]byte(swagger2), "http://legacy.test")
	if err != nil {
		t.Fatalf("parseOpenAPI swagger2: %v", err)
	}
	if len(got) != 1 || got[0].Endpoint.Path != "/v1/orders" {
		t.Fatalf("swagger2 basePath not applied: %+v", got)
	}
	if got[0].URL != "http://legacy.test/v1/orders" {
		t.Errorf("URL = %q, want the supplied base URL", got[0].URL)
	}
}

func findOperation(reqs []domain.CapturedRequest, method, path string) (domain.CapturedRequest, bool) {
	for _, r := range reqs {
		if r.Endpoint.Method == method && r.Endpoint.Path == path {
			return r, true
		}
	}
	return domain.CapturedRequest{}, false
}

func TestParseOpenAPI_RejectsEmptyDoc(t *testing.T) {
	if _, err := parseOpenAPI([]byte(`{"openapi":"3.0.0","paths":{}}`), ""); err == nil {
		t.Fatal("expected an error for a document with no paths")
	}
}

func TestRequestID_IsStable(t *testing.T) {
	a := requestID("form", "POST", "/coupon/redeem")
	b := requestID("form", "POST", "/coupon/redeem")
	c := requestID("form", "POST", "/other")
	if a != b {
		t.Errorf("ids must be stable across runs: %q vs %q", a, b)
	}
	if a == c {
		t.Error("different endpoints must not collide")
	}
}
