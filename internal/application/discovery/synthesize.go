package discovery

import (
	"encoding/json"
	"fmt"
	"strings"

	"github.com/Lakshya5876/Raceveil/internal/domain"
)

// preferredBindFields are the response field names most likely to name the
// resource a follow-up Act Request consumes, best first. A setup endpoint
// that mints something almost always returns it under one of these.
var preferredBindFields = []string{
	"code", "id", "sku", "token", "username", "order_id", "orderId",
	"cart_id", "cartId", "reference", "voucher", "key",
}

// SynthesizeActBinding fills in an Act Request's body when discovery found
// the endpoint but not its request shape — the normal case for an endpoint
// scraped from inline JavaScript, where `fetch("/coupon/redeem")` reveals
// the URL and nothing else.
//
// The heuristic is the one a human would try first and can check at a
// glance: whatever field the Setup phase just minted is probably the field
// the Act Request consumes. setupResponse is the setup's *observed*
// response, so this is inference from evidence rather than a guess about
// an API's schema.
//
// It only ever fills an empty or placeholder-free body. A Workflow that
// already carries a real body (from a HAR import, or a hand-written
// candidate.yaml) is returned untouched — observed traffic always beats
// inference.
func SynthesizeActBinding(wf domain.Workflow, setupID string, setupResponse []byte) domain.Workflow {
	field, ok := bindableField(setupResponse)
	if !ok {
		return wf
	}

	out := wf
	out.Requests = append([]domain.RequestSpec(nil), wf.Requests...)
	for i := range out.Requests {
		spec := &out.Requests[i]
		if spec.ID != wf.ActRequest {
			continue
		}
		if !needsSynthesizedBody(*spec) {
			return wf // observed traffic already told us the shape
		}
		placeholder := strings.ToUpper(field)
		spec.Body = fmt.Sprintf(`{%q:"${%s}"}`, field, placeholder)
		if spec.Headers == nil {
			spec.Headers = map[string]string{}
		}
		if spec.Headers["content-type"] == "" {
			spec.Headers["content-type"] = "application/json"
		}
		spec.Bindings = append(spec.Bindings, domain.Binding{
			Name: placeholder,
			In:   "body",
			From: setupID + ".$." + field,
		})
		return out
	}
	return wf
}

// needsSynthesizedBody reports whether an Act Request's body carries no
// usable information yet.
func needsSynthesizedBody(spec domain.RequestSpec) bool {
	trimmed := strings.TrimSpace(spec.Body)
	return trimmed == "" || trimmed == "{}" || trimmed == "null"
}

// bindableField picks the field of a setup response most likely to name the
// resource the Act Request operates on: a known conventional name first,
// then any string-valued field, so an unconventional API still works.
func bindableField(setupResponse []byte) (string, bool) {
	var payload map[string]any
	if err := json.Unmarshal(setupResponse, &payload); err != nil || len(payload) == 0 {
		return "", false
	}
	if field, ok := preferredField(payload); ok {
		return field, true
	}
	return firstBindableField(payload)
}

func preferredField(payload map[string]any) (string, bool) {
	for _, want := range preferredBindFields {
		for key, value := range payload {
			if strings.EqualFold(key, want) && isBindableValue(value) {
				return key, true
			}
		}
	}
	return "", false
}

// firstBindableField is the deterministic fallback: the alphabetically
// first usable field, so repeated scans of the same target infer the same
// Workflow (Design/ARCHITECTURE.md §8 determinism where possible).
func firstBindableField(payload map[string]any) (string, bool) {
	best := ""
	for key, value := range payload {
		if isBindableValue(value) && (best == "" || key < best) {
			best = key
		}
	}
	return best, best != ""
}

func isBindableValue(v any) bool {
	switch t := v.(type) {
	case string:
		return t != ""
	case float64:
		return true
	default:
		return false
	}
}
