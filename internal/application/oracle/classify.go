// Package oracle implements RaceVeil's Observable-State Oracle: the leveled
// judgment subsystem (Levels 1-5, Design/ARCHITECTURE.md §4) that decides,
// from a Baseline plus ConcurrentTrials, whether a concurrency-induced
// Invariant violation occurred, and the Statistical Verification that turns
// per-trial observations into a Wilson-interval Confidence band (Design/
// ARCHITECTURE.md §5). Levels 1+3 land in Phase 1; Levels 2+4 and the
// corroboration rule land in Phase 2 (Context/ROADMAP.md).
package oracle

import "strings"

// Classification is the Level 1 verdict for one observed response.
type Classification string

const (
	Success      Classification = "success"
	Reject       Classification = "reject"
	Unclassified Classification = "error"
)

// ObservedResponse is the minimal response shape the Oracle classifies —
// deliberately decoupled from infrastructure/sync.Response so this package
// stays pure decision logic with no I/O-layer dependency.
type ObservedResponse struct {
	StatusCode int
	Body       string
}

// SuccessRule and RejectRule mirror Design/DATA_MODEL.md's Matcher
// (success_when/reject_when): a status code and/or a body substring. A zero
// Status means "don't constrain on status"; an empty BodyContains means
// "don't constrain on body".
type Rule struct {
	Status       int
	BodyContains string
}

// Classify is the Level 1 direct-response classifier
// (Design/ARCHITECTURE.md §4): it labels one response success, reject, or
// unclassified using the declared or baseline-learned success/reject rules.
// Success is checked first — a response can't be both.
func Classify(resp ObservedResponse, success, reject *Rule) Classification {
	if success != nil && ruleMatches(resp, *success) {
		return Success
	}
	if reject != nil && ruleMatches(resp, *reject) {
		return Reject
	}
	return Unclassified
}

func ruleMatches(resp ObservedResponse, r Rule) bool {
	if r.Status != 0 && resp.StatusCode != r.Status {
		return false
	}
	if r.BodyContains != "" && !strings.Contains(resp.Body, r.BodyContains) {
		return false
	}
	return true
}

// limitMarkers are body substrings that indicate an enforced limit — the
// rejection half of a classifier, and the evidence that an Invariant exists
// at all (Design/ARCHITECTURE.md §2.4 Phase B, §4 baseline calibration).
var limitMarkers = []string{
	"already", "used", "redeemed", "duplicate", "exists", "taken",
	"limit", "exceeded", "too many", "out of stock", "sold out",
	"insufficient", "not pending", "conflict", "rate",
}

// preconditionMarkers are body substrings indicating a request failed
// because a prerequisite was missing rather than because a limit was
// enforced. Telling these apart is what stops "you have no cart" from being
// mistaken for "this coupon is already used".
var preconditionMarkers = []string{
	"not found", "no such", "missing", "required", "empty",
	"does not exist", "no cart", "no order", "no session",
	"must be", "invalid", "unknown",
}

// IsLimitSignal reports whether a response looks like an enforced limit.
func IsLimitSignal(status int, body string) bool {
	if status == 409 || status == 429 {
		return true
	}
	if status < 400 || status >= 500 {
		return false
	}
	return containsAny(strings.ToLower(body), limitMarkers)
}

// IsPreconditionFailure reports whether a response looks like "you skipped
// a step" rather than "the limit stopped you". A limit signal always wins:
// "already redeemed" is a limit even though it is also a 4xx.
func IsPreconditionFailure(status int, body string) bool {
	if status < 400 || status >= 500 {
		return false
	}
	lower := strings.ToLower(body)
	if containsAny(lower, limitMarkers) {
		return false
	}
	if status == 404 || status == 412 || status == 422 || status == 400 {
		return true
	}
	return containsAny(lower, preconditionMarkers)
}

func containsAny(haystack string, needles []string) bool {
	for _, n := range needles {
		if strings.Contains(haystack, n) {
			return true
		}
	}
	return false
}

// ClassifyStructural is the Baseline's classifier of last resort: when the
// operator declared no success_when/reject_when, the Oracle must still
// learn one from what the Baseline observed (Design/ARCHITECTURE.md §4
// "Baseline calibration (prerequisite for all levels)"). A 2xx is a
// success, a limit-shaped 4xx is a rejection, and anything else stays
// unclassified rather than being forced into a bucket.
func ClassifyStructural(status int, body string) Classification {
	switch {
	case status >= 200 && status < 300:
		return Success
	case IsLimitSignal(status, body):
		return Reject
	default:
		return Unclassified
	}
}

// CountSuccesses is the Level 1 aggregate: S = #{concurrent responses
// classified success} (Design/ARCHITECTURE.md §4).
func CountSuccesses(responses []ObservedResponse, success, reject *Rule) int {
	s := 0
	for _, r := range responses {
		if Classify(r, success, reject) == Success {
			s++
		}
	}
	return s
}
