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

	"github.com/Lakshya5876/Raceveil/internal/application/oracle"
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

// IsPreconditionFailure reports whether a response looks like "you skipped
// a step" rather than "the limit stopped you". Response meaning is the
// Oracle's vocabulary (Level 1 classification); discovery just applies it.
func IsPreconditionFailure(status int, body string) bool {
	return oracle.IsPreconditionFailure(status, body)
}

// IsLimitSignal reports whether a response looks like an enforced limit —
// the evidence that an Invariant exists at all.
func IsLimitSignal(status int, body string) bool {
	return oracle.IsLimitSignal(status, body)
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
	markers := variantMarkers(all)
	actVariant := variantOf(act.Endpoint.Path, markers)

	var candidates []scored
	for _, r := range all {
		if r.ID == act.ID || !IsMutating(r.Endpoint.Method) {
			continue
		}
		// A setup from a different variant of the same resource mints state
		// this Act Request cannot use, so it is not a candidate at all.
		if variantOf(r.Endpoint.Path, markers) != actVariant {
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
//
// A creation-shaped verb in the candidate's own path is a *necessary*
// condition, not just a bonus: without it, "shares a path prefix" would
// happily nominate /coupon/redeem as the setup for /coupon/issue, which is
// backwards and burns a probe proving it. An Act Request that already mints
// its own state likewise needs no setup at all.
func setupScore(act, candidate domain.CapturedRequest) int {
	if !hasSetupVerb(candidate.Endpoint.Path) {
		return 0
	}
	if hasSetupVerb(act.Endpoint.Path) {
		return 0 // the Act Request creates its own precondition
	}
	score := 2
	// Shared leading path segments: /coupon/issue is a far better setup for
	// /coupon/redeem than /newsletter/create is.
	score += 3 * sharedSegments(act.Endpoint.Path, candidate.Endpoint.Path)
	if actConsumesIdentifier(act) {
		score++ // the Act Request looks like it consumes an id something else mints
	}
	return score
}

// sharedSegments counts the leading path segments two paths have in common.
func sharedSegments(a, b string) int {
	as := strings.Split(strings.Trim(a, "/"), "/")
	bs := strings.Split(strings.Trim(b, "/"), "/")
	n := 0
	for n < len(as) && n < len(bs) && as[n] == bs[n] && as[n] != "" {
		n++
	}
	return n
}

// variantMarkers finds the trailing "-token" markers that this particular
// API actually uses to expose parallel variants of one resource — "-safe"
// where both /coupon/redeem and /coupon/redeem-safe exist, "-v2" where both
// /orders and /orders-v2 do.
//
// A marker is only real if the same stem appears both with and without it
// somewhere in the captured set. That corpus-relative test is what stops
// /issue-code from being misread as a "code variant" of /issue: nothing
// named /issue exists alongside it, so "-code" is just part of the name.
//
// This matters because pairing an Act Request with the *other* variant's
// setup mints state the Act Request cannot use, which then looks — wrongly
// — like "this endpoint has no invariant".
func variantMarkers(all []domain.CapturedRequest) map[string]bool {
	// Full paths, not bare last segments: variants are variants *of the same
	// resource*. Comparing last segments alone would let /signup/issue-username
	// pair off against an unrelated /coupon/issue and invent a "username"
	// variant that this API does not have.
	paths := make(map[string]bool, len(all))
	for _, r := range all {
		paths[strings.TrimRight(r.Endpoint.Path, "/")] = true
	}
	markers := map[string]bool{}
	for path := range paths {
		seg := lastSegment(path)
		i := strings.LastIndex(seg, "-")
		if i <= 0 {
			continue
		}
		stem := strings.TrimSuffix(path, "-"+seg[i+1:])
		if paths[stem] {
			markers[seg[i+1:]] = true
		}
	}
	return markers
}

// variantOf returns the variant marker a path carries, given the markers
// this API actually uses, or "" for the base variant.
func variantOf(path string, markers map[string]bool) string {
	seg := lastSegment(path)
	if i := strings.LastIndex(seg, "-"); i > 0 && markers[seg[i+1:]] {
		return seg[i+1:]
	}
	return ""
}

func lastSegment(path string) string {
	trimmed := strings.Trim(path, "/")
	if i := strings.LastIndex(trimmed, "/"); i >= 0 {
		return trimmed[i+1:]
	}
	return trimmed
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
