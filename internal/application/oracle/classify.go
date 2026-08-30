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
