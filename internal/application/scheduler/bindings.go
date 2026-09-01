package scheduler

import (
	"encoding/json"
	"fmt"
	"strconv"
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

// resolveJSONPath walks a minimal dotted "$.field.field..." path against a
// JSON response body — enough for extraction needs across bindings,
// body-differential (Level 2), and post-state probes (Level 4), without a
// full JSONPath engine.
func resolveJSONPath(body []byte, path string) (any, error) {
	rest, ok := strings.CutPrefix(path, "$.")
	if !ok {
		return nil, fmt.Errorf("json path %q must start with \"$.\"", path)
	}
	var v any
	if err := json.Unmarshal(body, &v); err != nil {
		return nil, fmt.Errorf("parse JSON body for path %q: %w", path, err)
	}
	cur := v
	for _, part := range strings.Split(rest, ".") {
		m, ok := cur.(map[string]any)
		if !ok {
			return nil, fmt.Errorf("json path %q: expected an object before %q", path, part)
		}
		cur, ok = m[part]
		if !ok {
			return nil, fmt.Errorf("json path %q: field %q not found in response", path, part)
		}
	}
	return cur, nil
}

// extractJSONPath resolves path to a string field — used for variable
// bindings (e.g. "$.code").
func extractJSONPath(body []byte, path string) (string, error) {
	v, err := resolveJSONPath(body, path)
	if err != nil {
		return "", err
	}
	s, ok := v.(string)
	if !ok {
		return "", fmt.Errorf("json path %q: resolved value is not a string", path)
	}
	return s, nil
}

// extractJSONPathValue resolves path to a value usable as a Level 2
// body-differential signature: a JSON number is formatted as a string, a
// JSON string is returned as-is — either makes a fine "distinct effect"
// signature (Design/ARCHITECTURE.md §4 Level 2).
func extractJSONPathValue(body []byte, path string) (string, error) {
	v, err := resolveJSONPath(body, path)
	if err != nil {
		return "", err
	}
	switch t := v.(type) {
	case string:
		return t, nil
	case float64: // encoding/json decodes all JSON numbers as float64
		return strconv.FormatFloat(t, 'f', -1, 64), nil
	default:
		return "", fmt.Errorf("json path %q: resolved value is not a string or number", path)
	}
}

// extractJSONPathInt resolves path to an integer — used for the Level 4
// post-state probe's persisted count.
func extractJSONPathInt(body []byte, path string) (int, error) {
	v, err := resolveJSONPath(body, path)
	if err != nil {
		return 0, err
	}
	f, ok := v.(float64)
	if !ok {
		return 0, fmt.Errorf("json path %q: resolved value is not a number", path)
	}
	return int(f), nil
}
