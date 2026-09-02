package crawl

import (
	"fmt"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strings"

	"github.com/Lakshya5876/Raceveil/internal/domain"
)

// harFile is the subset of the HAR 1.2 schema RaceVeil needs. A HAR is how
// an operator hands RaceVeil the traffic from driving a hard target (SPA,
// heavy JS) by hand — the pragmatic path ADR-012 chose over a headless
// browser.
type harFile struct {
	Log struct {
		Entries []struct {
			Request struct {
				Method  string `json:"method"`
				URL     string `json:"url"`
				Headers []struct {
					Name  string `json:"name"`
					Value string `json:"value"`
				} `json:"headers"`
				PostData struct {
					MimeType string `json:"mimeType"`
					Text     string `json:"text"`
				} `json:"postData"`
			} `json:"request"`
			Response struct {
				Status int `json:"status"`
			} `json:"response"`
		} `json:"entries"`
	} `json:"log"`
}

// ImportHAR loads Requests from a HAR file captured by a browser devtools
// panel or an intercepting proxy (Design/ARCHITECTURE.md §2.2 Import).
// Static-asset entries and non-2xx/3xx entries are dropped: they are noise
// for candidate discovery.
func ImportHAR(path string) ([]domain.CapturedRequest, error) {
	data, err := os.ReadFile(filepath.Clean(path)) // #nosec G304 -- operator-supplied CLI path, expected
	if err != nil {
		return nil, fmt.Errorf("import har: read %s: %w", path, err)
	}
	var har harFile
	if err := decodeJSON(data, &har); err != nil {
		return nil, fmt.Errorf("import har: parse %s: %w", path, err)
	}

	var out []domain.CapturedRequest
	seen := make(map[string]bool)
	for _, entry := range har.Log.Entries {
		u, ok := usableHAREntry(entry.Request.Method, entry.Request.URL, entry.Response.Status)
		if !ok {
			continue
		}
		key := strings.ToUpper(entry.Request.Method) + " " + u.Path
		if seen[key] {
			continue
		}
		seen[key] = true

		headers := make(map[string]string, len(entry.Request.Headers))
		for _, h := range entry.Request.Headers {
			// Never import credential headers from a HAR: they belong in the
			// Session the operator supplies, not in a persisted Request
			// (Design/SECURITY.md §5).
			if isCredentialHeader(h.Name) {
				continue
			}
			headers[strings.ToLower(h.Name)] = h.Value
		}
		method := strings.ToUpper(entry.Request.Method)
		out = append(out, domain.CapturedRequest{
			ID:       requestID("har", method, u.Path),
			Endpoint: domain.Endpoint{Method: method, Path: u.Path},
			URL:      entry.Request.URL,
			Headers:  headers,
			Body:     entry.Request.PostData.Text,
			Source:   "import",
		})
	}
	return out, nil
}

// usableHAREntry filters the noise out of a HAR: entries without a method
// or URL, static assets, and failed responses are not race candidates.
func usableHAREntry(method, rawURL string, status int) (*url.URL, bool) {
	if rawURL == "" || method == "" || status >= 400 {
		return nil, false
	}
	u, err := url.Parse(rawURL)
	if err != nil {
		return nil, false
	}
	if hasAnySuffix(u.Path, ".js", ".css", ".png", ".jpg", ".jpeg", ".svg", ".gif", ".ico", ".woff", ".woff2", ".map") {
		return nil, false
	}
	return u, true
}

// isCredentialHeader reports whether a header name carries authentication
// material that must never be persisted into requests.jsonl.
func isCredentialHeader(name string) bool {
	switch strings.ToLower(name) {
	case "authorization", "cookie", "set-cookie", "proxy-authorization",
		"x-api-key", "x-auth-token", "x-csrf-token", "x-xsrf-token":
		return true
	}
	return false
}

// openAPIDoc is the subset of OpenAPI 3 / Swagger 2 needed to enumerate
// operations. Both versions put paths in the same place; the difference
// that matters here (host/basePath vs servers) is handled by the caller
// passing a base URL.
type openAPIDoc struct {
	OpenAPI  string `json:"openapi"`
	Swagger  string `json:"swagger"`
	BasePath string `json:"basePath"`
	Servers  []struct {
		URL string `json:"url"`
	} `json:"servers"`
	Paths map[string]map[string]struct {
		OperationID string `json:"operationId"`
		Summary     string `json:"summary"`
	} `json:"paths"`
}

// ImportOpenAPI loads Requests from an OpenAPI 3 or Swagger 2 document.
// baseURL supplies the origin when the document's own servers/basePath
// don't (a very common case for locally-served specs).
func ImportOpenAPI(path, baseURL string) ([]domain.CapturedRequest, error) {
	data, err := os.ReadFile(filepath.Clean(path)) // #nosec G304 -- operator-supplied CLI path, expected
	if err != nil {
		return nil, fmt.Errorf("import openapi: read %s: %w", path, err)
	}
	return parseOpenAPI(data, baseURL)
}

func parseOpenAPI(data []byte, baseURL string) ([]domain.CapturedRequest, error) {
	var doc openAPIDoc
	if err := decodeJSON(data, &doc); err != nil {
		return nil, fmt.Errorf("import openapi: parse: %w", err)
	}
	if len(doc.Paths) == 0 {
		return nil, fmt.Errorf("import openapi: document declares no paths")
	}

	origin := strings.TrimSuffix(baseURL, "/")
	if origin == "" && len(doc.Servers) > 0 {
		origin = strings.TrimSuffix(doc.Servers[0].URL, "/")
	}
	prefix := strings.TrimSuffix(doc.BasePath, "/")

	var out []domain.CapturedRequest
	for rawPath, operations := range doc.Paths {
		for method := range operations {
			if m := strings.ToUpper(method); isHTTPMethod(m) {
				out = append(out, openAPIOperation(m, prefix+rawPath, origin))
			}
		}
	}
	return out, nil
}

func openAPIOperation(method, fullPath, origin string) domain.CapturedRequest {
	req := domain.CapturedRequest{
		ID:       requestID("oas", method, fullPath),
		Endpoint: domain.Endpoint{Method: method, Path: fullPath},
		URL:      origin + fullPath,
		Source:   "import",
	}
	if method == http.MethodPost || method == http.MethodPut || method == http.MethodPatch {
		req.Headers = map[string]string{"content-type": "application/json"}
		req.Body = "{}"
	}
	return req
}

func isHTTPMethod(m string) bool {
	switch m {
	case http.MethodGet, http.MethodPost, http.MethodPut,
		http.MethodPatch, http.MethodDelete, http.MethodHead, http.MethodOptions:
		return true
	}
	return false
}
