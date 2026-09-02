// Package discovery groups captured Requests into Workflows
// (Design/ARCHITECTURE.md §2.3). On the MVP `scan --candidate` path this
// stage never runs — the Workflow is supplied. On the v1 `scan <url>` path
// it is mostly identity (one Request = one Workflow) plus a
// setup-dependency heuristic: if the Act Request alone returns a
// precondition error, search captured traffic for the predecessor Requests
// that satisfy it and attach them as the Workflow prefix.
package discovery

import (
	"regexp"
	"sort"
	"strings"

	"github.com/Lakshya5876/Raceveil/internal/domain"
)

// IsMutating reports whether a method can change server state, and so
// whether the Endpoint can carry a concurrency-integrity violation worth
// testing at all (Design/ARCHITECTURE.md §2.4 Phase A). The verb set itself
// is domain vocabulary — see domain.IsMutating.
func IsMutating(method string) bool {
	return domain.IsMutating(method)
}

// setupVerbs name path segments that typically create the precondition a
// later Act Request consumes (a cart, a code, an order, a session).
var setupVerbs = []string{
	"create", "new", "issue", "add", "start", "init", "register",
	"open", "begin", "prepare", "generate", "restock", "seed",
}

// preconditionMarkers are body substrings that suggest a request failed
// because a prerequisite was missing rather than because a limit was
// enforced. Distinguishing the two is what stops discovery from mistaking
// "you have no cart" for "this coupon is already used".
var preconditionMarkers = []string{
	"not found", "no such", "missing", "required", "empty",
	"does not exist", "no cart", "no order", "no session",
	"must be", "invalid", "unknown",
}

// limitMarkers are body substrings that suggest a limit was enforced —
// the signal an Invariant actually exists (Design/ARCHITECTURE.md §2.4
// Phase B).
var limitMarkers = []string{
	"already", "used", "redeemed", "duplicate", "exists", "taken",
	"limit", "exceeded", "too many", "out of stock", "sold out",
	"insufficient", "not pending", "conflict", "rate",
}

// IsPreconditionFailure reports whether a response looks like "you skipped
// a step" rather than "the limit stopped you".
func IsPreconditionFailure(status int, body string) bool {
	if status < 400 || status >= 500 {
		return false
	}
	lower := strings.ToLower(body)
	// A limit signal wins: "already redeemed" is a limit, not a missing
	// precondition, even though both are 4xx.
	if containsAny(lower, limitMarkers) {
		return false
	}
	if status == 404 || status == 412 || status == 422 || status == 400 {
		return true
	}
	return containsAny(lower, preconditionMarkers)
}

// IsLimitSignal reports whether a response looks like an enforced limit —
// the rejection half of a Baseline classifier, and the evidence that an
// Invariant exists at all.
func IsLimitSignal(status int, body string) bool {
	if status == 409 || status == 429 {
		return true
	}
	if status < 400 || status >= 500 {
		return false
	}
	return containsAny(strings.ToLower(body), limitMarkers)
}

func containsAny(haystack string, needles []string) bool {
	for _, n := range needles {
		if strings.Contains(haystack, n) {
			return true
		}
	}
	return false
}

// idPattern matches identifier-shaped tokens in a path or body — hex ids,
// UUIDs, and long digit runs — used to detect a data dependency between a
// setup Request and an Act Request.
var idPattern = regexp.MustCompile(`[0-9a-fA-F]{8,}|\b\d{4,}\b`)

// ExtractWorkflows turns captured Requests into candidate Workflows, one
// per mutating Request (identity extraction). Setup requests are attached
// separately by AttachSetup once a probe shows the Act Request needs one.
func ExtractWorkflows(requests []domain.CapturedRequest) []domain.Workflow {
	var workflows []domain.Workflow
	for _, r := range requests {
		if !IsMutating(r.Endpoint.Method) {
			continue // read-only endpoints cannot violate an integrity invariant
		}
		workflows = append(workflows, domain.Workflow{
			ID:         "wf_" + r.ID,
			Requests:   []domain.RequestSpec{specFromCaptured(r)},
			ActRequest: r.ID,
		})
	}
	return workflows
}

func specFromCaptured(r domain.CapturedRequest) domain.RequestSpec {
	return domain.RequestSpec{
		ID:       r.ID,
		Method:   r.Endpoint.Method,
		URL:      r.Endpoint.Path,
		Headers:  r.Headers,
		Body:     r.Body,
		Bindings: r.Bindings,
	}
}

// FindSetupCandidates ranks the captured Requests most likely to establish
// the precondition act needs, best first. The heuristic is deliberately
// conservative and explainable: a plausible setup shares a path prefix with
// the Act Request, uses a creation-shaped verb in its path, and is itself a
// mutation.
func FindSetupCandidates(act domain.CapturedRequest, all []domain.CapturedRequest) []domain.CapturedRequest {
	type scored struct {
		req   domain.CapturedRequest
		score int
	}
	var candidates []scored
	for _, r := range all {
		if r.ID == act.ID || !IsMutating(r.Endpoint.Method) {
			continue
		}
		if score := setupScore(act, r); score > 0 {
			candidates = append(candidates, scored{req: r, score: score})
		}
	}

	// Stable ordering: higher score first, then by id so repeated runs agree
	// (Design/ARCHITECTURE.md §8 determinism where possible).
	sort.SliceStable(candidates, func(i, j int) bool {
		if candidates[i].score != candidates[j].score {
			return candidates[i].score > candidates[j].score
		}
		return candidates[i].req.ID < candidates[j].req.ID
	})

	out := make([]domain.CapturedRequest, 0, len(candidates))
	for _, c := range candidates {
		out = append(out, c.req)
	}
	return out
}

// setupScore rates how plausibly candidate establishes the precondition act
// needs. Higher is better; 0 means "not a plausible setup".
func setupScore(act, candidate domain.CapturedRequest) int {
	score := 0
	if prefix := pathPrefix(act.Endpoint.Path); prefix != "" && pathPrefix(candidate.Endpoint.Path) == prefix {
		score += 3 // sibling under the same resource, e.g. /coupon/issue for /coupon/redeem
	}
	if hasSetupVerb(candidate.Endpoint.Path) {
		score += 2
	}
	if actConsumesIdentifier(act) {
		score++ // the Act Request looks like it consumes an id something else mints
	}
	return score
}

func actConsumesIdentifier(act domain.CapturedRequest) bool {
	return strings.Contains(act.Body, "${") ||
		idPattern.MatchString(act.Body) ||
		idPattern.MatchString(act.Endpoint.Path)
}

// AttachSetup returns a copy of wf with setup prepended as its Setup phase,
// wiring a binding from setup's response into the Act Request when the Act
// Request carries an unresolved ${PLACEHOLDER} (Design/DOMAIN.md §Act
// Request: setup runs once per trial and its output is bound into every Act
// instance).
func AttachSetup(wf domain.Workflow, setup domain.CapturedRequest, bindField string) domain.Workflow {
	setupSpec := specFromCaptured(setup)
	out := wf
	out.Requests = append([]domain.RequestSpec{setupSpec}, wf.Requests...)
	out.SetupRequests = append([]string{setup.ID}, wf.SetupRequests...)

	if bindField == "" {
		return out
	}
	for i := range out.Requests {
		if out.Requests[i].ID != wf.ActRequest {
			continue
		}
		placeholder := placeholderIn(out.Requests[i].Body)
		if placeholder == "" {
			continue
		}
		out.Requests[i].Bindings = append(out.Requests[i].Bindings, domain.Binding{
			Name: placeholder,
			In:   "body",
			From: setup.ID + ".$." + bindField,
		})
	}
	return out
}

var placeholderPattern = regexp.MustCompile(`\$\{([A-Za-z0-9_]+)\}`)

func placeholderIn(body string) string {
	m := placeholderPattern.FindStringSubmatch(body)
	if len(m) < 2 {
		return ""
	}
	return m[1]
}

func hasSetupVerb(path string) bool {
	lower := strings.ToLower(path)
	for _, v := range setupVerbs {
		if strings.Contains(lower, v) {
			return true
		}
	}
	return false
}

// pathPrefix returns the first path segment, the crude "resource" a request
// belongs to (/coupon/redeem and /coupon/issue share "coupon").
func pathPrefix(path string) string {
	trimmed := strings.Trim(path, "/")
	if trimmed == "" {
		return ""
	}
	if i := strings.Index(trimmed, "/"); i >= 0 {
		return trimmed[:i]
	}
	return trimmed
}
