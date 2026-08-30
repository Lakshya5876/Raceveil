package scheduler

import (
	"encoding/json"
	"fmt"
	"strings"

	"github.com/Lakshya5876/Raceveil/internal/domain"
	"github.com/Lakshya5876/Raceveil/internal/infrastructure/sync"
)

// priorResponses carries the raw response bodies of requests already
// executed earlier in the same trial/run, keyed by request id — the only
// data a Binding's "from: <request_id>.$.<path>" form can reference
// (Design/DOMAIN.md §Request variable bindings).
type priorResponses map[string][]byte

// prepareRequest builds a sync.PreparedRequest from a RequestSpec, resolving
// its Bindings against the Session and any prior responses in this trial.
func prepareRequest(spec domain.RequestSpec, baseURL string, sess domain.Session, prior priorResponses) (sync.PreparedRequest, error) {
	headers := make(map[string]string, len(spec.Headers)+len(sess.Headers)+1)
	for k, v := range sess.Headers {
		headers[k] = v
	}
	for k, v := range spec.Headers {
		headers[k] = v
	}
	if len(sess.Cookies) > 0 {
		headers["Cookie"] = cookieHeader(sess.Cookies)
	}

	body := spec.Body
	for _, b := range spec.Bindings {
		val, err := resolveBinding(b, prior)
		if err != nil {
			return sync.PreparedRequest{}, fmt.Errorf("request %q: binding %q: %w", spec.ID, b.Name, err)
		}
		placeholder := "${" + b.Name + "}"
		switch b.In {
		case "body":
			body = strings.ReplaceAll(body, placeholder, val)
		case "header":
			for hk, hv := range headers {
				headers[hk] = strings.ReplaceAll(hv, placeholder, val)
			}
		default:
			return sync.PreparedRequest{}, fmt.Errorf("request %q: binding %q: unsupported \"in\" %q", spec.ID, b.Name, b.In)
		}
	}

	return sync.PreparedRequest{
		Method:  spec.Method,
		URL:     baseURL + spec.URL,
		Headers: headers,
		Body:    []byte(body),
	}, nil
}

func cookieHeader(cookies []domain.Cookie) string {
	parts := make([]string, len(cookies))
	for i, c := range cookies {
		parts[i] = c.Name + "=" + c.Value
	}
	return strings.Join(parts, "; ")
}

// resolveBinding resolves one Binding's "from" reference (Design/
// DATA_MODEL.md candidate.yaml bindings). Phase 1 supports:
//   - "literal:VALUE"           -> VALUE
//   - "<request_id>.$.<path>"   -> a field extracted from that request's
//     JSON response body, already executed earlier in this trial.
//
// Session-field bindings (e.g. "session.csrf") are part of the general
// schema but not exercised by the Phase 1 fixture; Phase 1 does not
// implement them, and this returns a clear error if one is encountered.
func resolveBinding(b domain.Binding, prior priorResponses) (string, error) {
	if v, ok := strings.CutPrefix(b.From, "literal:"); ok {
		return v, nil
	}
	if idx := strings.Index(b.From, ".$"); idx >= 0 {
		requestID := b.From[:idx]
		jsonPath := b.From[idx+1:]
		body, ok := prior[requestID]
		if !ok {
			return "", fmt.Errorf("from %q: request %q has not produced a response yet", b.From, requestID)
		}
		return extractJSONPath(body, jsonPath)
	}
	return "", fmt.Errorf("from %q: unsupported binding source in Phase 1", b.From)
}

// extractJSONPath resolves a minimal dotted "$.field.field..." path against
// a JSON response body — enough for the Phase 1 fixture's "$.code" and
// generalizes to nested string fields without a full JSONPath engine.
func extractJSONPath(body []byte, path string) (string, error) {
	rest, ok := strings.CutPrefix(path, "$.")
	if !ok {
		return "", fmt.Errorf("json path %q must start with \"$.\"", path)
	}
	var v any
	if err := json.Unmarshal(body, &v); err != nil {
		return "", fmt.Errorf("parse JSON body for path %q: %w", path, err)
	}
	cur := v
	for _, part := range strings.Split(rest, ".") {
		m, ok := cur.(map[string]any)
		if !ok {
			return "", fmt.Errorf("json path %q: expected an object before %q", path, part)
		}
		cur, ok = m[part]
		if !ok {
			return "", fmt.Errorf("json path %q: field %q not found in response", path, part)
		}
	}
	s, ok := cur.(string)
	if !ok {
		return "", fmt.Errorf("json path %q: resolved value is not a string", path)
	}
	return s, nil
}
