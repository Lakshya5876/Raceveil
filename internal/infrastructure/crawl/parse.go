package crawl

import (
	"bytes"
	"crypto/sha1" // #nosec G505 -- short non-cryptographic id derivation, not a security control
	"encoding/hex"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"regexp"
	"strings"

	"golang.org/x/net/html"

	"github.com/Lakshya5876/Raceveil/internal/domain"
)

// capturedForm is one <form> found on a page, resolved to an absolute URL.
type capturedForm struct {
	method string
	action string
	fields []string
}

func (f capturedForm) toCaptured(session domain.Session) domain.CapturedRequest {
	u, _ := url.Parse(f.action)
	path := f.action
	if u != nil && u.Path != "" {
		path = u.Path
	}
	req := domain.CapturedRequest{
		ID:        requestID("form", f.method, path),
		Endpoint:  domain.Endpoint{Method: f.method, Path: path},
		URL:       f.action,
		SessionID: session.ID,
		Source:    "crawl",
	}
	if f.method == http.MethodPost && len(f.fields) > 0 {
		req.Headers = map[string]string{"content-type": "application/x-www-form-urlencoded"}
		parts := make([]string, 0, len(f.fields))
		for _, name := range f.fields {
			parts = append(parts, url.QueryEscape(name)+"=${"+strings.ToUpper(name)+"}")
			req.Bindings = append(req.Bindings, domain.Binding{
				Name: strings.ToUpper(name), In: "body", From: "literal:",
			})
		}
		req.Body = strings.Join(parts, "&")
	}
	return req
}

// jsEndpointPattern matches quoted URL-ish strings in inline JavaScript —
// fetch("/api/x"), axios.post('/cart/apply'), url: "/checkout". It is a
// deliberate heuristic (ADR-012: no headless browser), so it over-collects
// and lets ranking's invariant-inference gate discard what doesn't behave
// like a mutation.
var jsEndpointPattern = regexp.MustCompile(`["'](/[A-Za-z0-9._~\-/]{2,120})["']`)

// parsePage extracts same-page links, forms, and JS-referenced endpoints,
// all resolved against pageURL.
func parsePage(body []byte, pageURL string) (links []string, forms []capturedForm, jsEndpoints []string) {
	base, err := url.Parse(pageURL)
	if err != nil {
		return nil, nil, nil
	}
	doc, err := html.Parse(bytes.NewReader(body))
	if err != nil {
		return nil, nil, nil
	}

	var walk func(*html.Node)
	walk = func(n *html.Node) {
		if n.Type == html.ElementNode {
			switch n.Data {
			case "a":
				if href := attr(n, "href"); href != "" {
					if abs := resolve(base, href); abs != "" {
						links = append(links, abs)
					}
				}
			case "form":
				forms = append(forms, parseForm(n, base))
			case "script":
				jsEndpoints = append(jsEndpoints, extractJSEndpoints(textOf(n), base)...)
			}
		}
		for child := n.FirstChild; child != nil; child = child.NextSibling {
			walk(child)
		}
	}
	walk(doc)
	return links, forms, dedupe(jsEndpoints)
}

func parseForm(n *html.Node, base *url.URL) capturedForm {
	method := strings.ToUpper(attr(n, "method"))
	if method == "" {
		method = http.MethodGet
	}
	action := resolve(base, attr(n, "action"))
	if action == "" {
		action = base.String()
	}
	form := capturedForm{method: method, action: action}

	var collect func(*html.Node)
	collect = func(c *html.Node) {
		if c.Type == html.ElementNode && (c.Data == "input" || c.Data == "select" || c.Data == "textarea") {
			if name := attr(c, "name"); name != "" {
				form.fields = append(form.fields, name)
			}
		}
		for child := c.FirstChild; child != nil; child = child.NextSibling {
			collect(child)
		}
	}
	collect(n)
	return form
}

func extractJSEndpoints(script string, base *url.URL) []string {
	var out []string
	for _, m := range jsEndpointPattern.FindAllStringSubmatch(script, -1) {
		path := m[1]
		// Skip obvious static assets — they are never race candidates.
		if hasAnySuffix(path, ".js", ".css", ".png", ".jpg", ".jpeg", ".svg", ".gif", ".ico", ".woff", ".woff2", ".map") {
			continue
		}
		if abs := resolve(base, path); abs != "" {
			out = append(out, abs)
		}
	}
	return out
}

func hasAnySuffix(s string, suffixes ...string) bool {
	lower := strings.ToLower(s)
	for _, suf := range suffixes {
		if strings.HasSuffix(lower, suf) {
			return true
		}
	}
	return false
}

func attr(n *html.Node, key string) string {
	for _, a := range n.Attr {
		if strings.EqualFold(a.Key, key) {
			return a.Val
		}
	}
	return ""
}

func textOf(n *html.Node) string {
	var sb strings.Builder
	for c := n.FirstChild; c != nil; c = c.NextSibling {
		if c.Type == html.TextNode {
			sb.WriteString(c.Data)
		}
	}
	return sb.String()
}

// resolve turns a possibly-relative href into an absolute URL, dropping
// fragments and anything that isn't http(s).
func resolve(base *url.URL, href string) string {
	href = strings.TrimSpace(href)
	if href == "" || strings.HasPrefix(href, "#") ||
		strings.HasPrefix(strings.ToLower(href), "javascript:") ||
		strings.HasPrefix(strings.ToLower(href), "mailto:") ||
		strings.HasPrefix(strings.ToLower(href), "tel:") {
		return ""
	}
	u, err := url.Parse(href)
	if err != nil {
		return ""
	}
	abs := base.ResolveReference(u)
	if abs.Scheme != "http" && abs.Scheme != "https" {
		return ""
	}
	abs.Fragment = ""
	return abs.String()
}

func dedupe(in []string) []string {
	seen := make(map[string]bool, len(in))
	out := make([]string, 0, len(in))
	for _, v := range in {
		if !seen[v] {
			seen[v] = true
			out = append(out, v)
		}
	}
	return out
}

// requestID derives a short stable id from a request's shape, so repeated
// crawls of the same target produce the same ids (Design/ARCHITECTURE.md §8
// determinism where possible).
func requestID(prefix, method, path string) string {
	sum := sha1.Sum([]byte(method + " " + path)) // #nosec G401 -- id derivation, not a security control
	return fmt.Sprintf("%s_%s", prefix, hex.EncodeToString(sum[:])[:8])
}

// jsonPeek reports whether b looks like a JSON object/array, used by the
// importers to reject obviously wrong input with a clear message.
func jsonPeek(b []byte) bool {
	trimmed := bytes.TrimSpace(b)
	return len(trimmed) > 0 && (trimmed[0] == '{' || trimmed[0] == '[')
}

// decodeJSON is a small shared helper for the importers.
func decodeJSON(data []byte, v any) error {
	if !jsonPeek(data) {
		return fmt.Errorf("input does not look like JSON")
	}
	return json.Unmarshal(data, v)
}
