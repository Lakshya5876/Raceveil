// Package crawl implements RaceVeil's request-capture infrastructure: a
// lightweight authenticated crawler plus OpenAPI and HAR importers
// (Design/ARCHITECTURE.md §2.2). It deliberately uses no headless browser
// (Context/DECISIONS.md ADR-012) — an HTML+JS-heuristic crawler and traffic
// import cover SPAs more reliably at a fraction of the dependency weight.
//
// Everything here is capture only: it produces Endpoints and Requests. It
// never decides what is worth testing (that is internal/application/ranking)
// and never sends a concurrent burst (that is internal/infrastructure/sync).
package crawl

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/Lakshya5876/Raceveil/internal/domain"
)

// Guard is the subset of security/scope.Guard the crawler needs. Declared
// locally so this package never imports security/scope directly; wiring
// supplies the concrete *scope.Guard, which satisfies it structurally.
// Every fetched URL passes Check before it is requested.
type Guard interface {
	Check(method, url string) error
}

// Options tune a crawl. Zero values fall back to conservative defaults —
// RaceVeil is a verification tool, not a spider, so the ceilings are low.
type Options struct {
	MaxPages   int
	MaxDepth   int
	Timeout    time.Duration
	UserAgent  string
	HTTPClient *http.Client
}

func (o Options) withDefaults() Options {
	if o.MaxPages <= 0 {
		o.MaxPages = 50
	}
	if o.MaxDepth <= 0 {
		o.MaxDepth = 3
	}
	if o.Timeout <= 0 {
		o.Timeout = 10 * time.Second
	}
	if o.UserAgent == "" {
		o.UserAgent = "RaceVeil/1.0 (+authorized concurrency verification)"
	}
	if o.HTTPClient == nil {
		o.HTTPClient = &http.Client{Timeout: o.Timeout}
	}
	return o
}

// Crawler walks an authorized target within Scope, collecting the Requests
// a human would otherwise capture by hand.
type Crawler struct {
	guard Guard
	opts  Options
}

// New builds a Crawler that checks every URL against guard before fetching.
func New(guard Guard, opts Options) *Crawler {
	return &Crawler{guard: guard, opts: opts.withDefaults()}
}

// Crawl walks from startURL, breadth-first, and returns every Request it
// could capture: page GETs, form submissions, and endpoints referenced from
// inline JavaScript. Out-of-scope URLs are skipped silently — the Guard is
// authoritative about what may be touched, and a crawl refusing a link is
// normal operation, not an error.
func (c *Crawler) Crawl(ctx context.Context, startURL string, session domain.Session) ([]domain.CapturedRequest, error) {
	start, err := url.Parse(startURL)
	if err != nil {
		return nil, fmt.Errorf("crawl: invalid start URL %q: %w", startURL, err)
	}

	type queued struct {
		u     string
		depth int
	}
	seen := map[string]bool{startURL: true}
	queue := []queued{{u: startURL, depth: 0}}
	var captured []domain.CapturedRequest
	pages := 0

	for len(queue) > 0 && pages < c.opts.MaxPages {
		select {
		case <-ctx.Done():
			return captured, ctx.Err()
		default:
		}
		item := queue[0]
		queue = queue[1:]

		body, err := c.fetch(ctx, item.u, session)
		if err != nil {
			continue // an unreachable or refused page is not a crawl failure
		}
		pages++
		captured = append(captured, capturedGET(item.u, session))

		if item.depth >= c.opts.MaxDepth {
			continue
		}
		links, found := c.harvest(body, item.u, start, session, seen)
		captured = append(captured, found...)
		for _, l := range links {
			queue = append(queue, queued{u: l, depth: item.depth + 1})
		}
	}
	return captured, nil
}

// harvest parses one fetched page into the links worth queueing and the
// Requests worth capturing, marking both as seen so the caller's loop stays
// a flat breadth-first walk.
func (c *Crawler) harvest(body []byte, pageURL string, start *url.URL, session domain.Session, seen map[string]bool) ([]string, []domain.CapturedRequest) {
	links, forms, jsEndpoints := parsePage(body, pageURL)

	var nextLinks []string
	for _, l := range links {
		if seen[l] || !sameHost(start, l) {
			continue
		}
		seen[l] = true
		nextLinks = append(nextLinks, l)
	}

	captured := make([]domain.CapturedRequest, 0, len(forms)+len(jsEndpoints))
	for _, f := range forms {
		captured = append(captured, f.toCaptured(session))
	}
	for _, e := range jsEndpoints {
		if seen["js:"+e] {
			continue
		}
		seen["js:"+e] = true
		captured = append(captured, capturedJSEndpoint(e, session))
	}
	return nextLinks, captured
}

func (c *Crawler) fetch(ctx context.Context, rawURL string, session domain.Session) ([]byte, error) {
	if err := c.guard.Check(http.MethodGet, rawURL); err != nil {
		return nil, err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, rawURL, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("User-Agent", c.opts.UserAgent)
	applySession(req, session)

	resp, err := c.opts.HTTPClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode >= 400 {
		return nil, fmt.Errorf("crawl: %s returned %d", rawURL, resp.StatusCode)
	}
	// Cap the body: a crawler that streams an unbounded response is a
	// memory-exhaustion foot-gun, and pages beyond this size are not the
	// small HTML/JS documents this heuristic parser is for.
	return io.ReadAll(io.LimitReader(resp.Body, 2<<20))
}

func applySession(req *http.Request, session domain.Session) {
	for k, v := range session.Headers {
		req.Header.Set(k, v)
	}
	// Build the Cookie request header directly rather than via AddCookie:
	// a request cookie only ever serializes name=value, so constructing a
	// full http.Cookie here would imply Secure/HttpOnly/SameSite semantics
	// that do not exist on this side of the exchange.
	if len(session.Cookies) > 0 {
		parts := make([]string, 0, len(session.Cookies))
		for _, ck := range session.Cookies {
			parts = append(parts, ck.Name+"="+ck.Value)
		}
		req.Header.Set("Cookie", strings.Join(parts, "; "))
	}
}

func sameHost(base *url.URL, candidate string) bool {
	u, err := url.Parse(candidate)
	if err != nil {
		return false
	}
	return strings.EqualFold(u.Host, base.Host)
}

func capturedGET(rawURL string, session domain.Session) domain.CapturedRequest {
	u, _ := url.Parse(rawURL)
	path := "/"
	if u != nil {
		path = u.Path
	}
	return domain.CapturedRequest{
		ID:        requestID("get", http.MethodGet, path),
		Endpoint:  domain.Endpoint{Method: http.MethodGet, Path: path},
		URL:       rawURL,
		SessionID: session.ID,
		Source:    "crawl",
	}
}

func capturedJSEndpoint(rawURL string, session domain.Session) domain.CapturedRequest {
	u, _ := url.Parse(rawURL)
	path := rawURL
	if u != nil && u.Path != "" {
		path = u.Path
	}
	// A URL referenced from JS is most often an XHR/fetch API call; POST is
	// the verb worth capturing for race testing, and ranking's Phase B probe
	// will discard it if it doesn't behave like a mutation.
	return domain.CapturedRequest{
		ID:        requestID("js", http.MethodPost, path),
		Endpoint:  domain.Endpoint{Method: http.MethodPost, Path: path},
		URL:       rawURL,
		Headers:   map[string]string{"content-type": "application/json"},
		SessionID: session.ID,
		Source:    "crawl",
	}
}
