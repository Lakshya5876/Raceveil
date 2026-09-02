// Package ranking turns discovered Workflows into ranked Candidates
// (Design/ARCHITECTURE.md §2.4). It is two-phase by design, so discovery
// prioritizes experiments instead of hammering everything:
//
//   - Phase A — static signals, no traffic: mutation verb, path/field name
//     semantics, absence of an idempotency key, reused resource identifiers.
//   - Phase B — invariant probe, cheap traffic: a tiny sequential probe that
//     looks for a rejection signal. If no limit is observed and none is
//     user-declared, the Candidate is dropped.
//
// Phase B is the invariant-inference gate — RaceVeil's primary
// false-positive defense (Context/DECISIONS.md ADR-015). Without an
// established Invariant there is nothing to violate: S = N successes on a
// non-idempotent POST /comment is correct behavior, not a finding.
package ranking

import (
	"context"
	"fmt"
	"sort"
	"strings"

	"github.com/Lakshya5876/Raceveil/internal/application/discovery"
	"github.com/Lakshya5876/Raceveil/internal/domain"
)

// raceProneNames are path/field tokens that historically correlate with
// limit-overrun, duplicate-effect, and uniqueness races
// (Design/ARCHITECTURE.md §2.4 Phase A).
var raceProneNames = []string{
	"redeem", "coupon", "voucher", "apply", "claim", "reserve", "checkout",
	"vote", "transfer", "withdraw", "balance", "quantity", "limit", "once",
	"purchase", "order", "confirm", "cancel", "book", "signup", "register",
	"invite", "referral", "giftcard", "credit", "payout", "cashout",
}

// idempotencyHeaders, when present, mean the endpoint already defends
// itself against duplicate submission — a strong negative signal.
var idempotencyHeaders = []string{"idempotency-key", "x-idempotency-key", "x-request-id"}

// StaticScore computes Phase A's prior for one captured Request: a
// [0,1]-normalized score plus the signals that produced it, so a ranking
// decision is always explainable in candidates.jsonl.
func StaticScore(r domain.CapturedRequest, all []domain.CapturedRequest) (float64, domain.ScoreSignals) {
	signals := domain.ScoreSignals{Verb: strings.ToUpper(r.Endpoint.Method)}
	if !discovery.IsMutating(r.Endpoint.Method) {
		return 0, signals // read-only endpoints are never race candidates
	}

	score := 0.35 // being a mutation at all is the entry ticket

	haystack := strings.ToLower(r.Endpoint.Path + " " + r.Body)
	for _, name := range raceProneNames {
		if strings.Contains(haystack, name) {
			signals.NameHits = append(signals.NameHits, name)
		}
	}
	// Diminishing returns: three hits is as meaningful as ten.
	switch {
	case len(signals.NameHits) >= 3:
		score += 0.30
	case len(signals.NameHits) == 2:
		score += 0.22
	case len(signals.NameHits) == 1:
		score += 0.12
	}

	signals.IdempotencyKeyAbsent = !hasIdempotencyKey(r)
	if signals.IdempotencyKeyAbsent {
		score += 0.15
	}

	signals.ReusedIdentifier = sharesIdentifierWithOthers(r, all)
	if signals.ReusedIdentifier {
		score += 0.10
	}

	if score > 1 {
		score = 1
	}
	return score, signals
}

func hasIdempotencyKey(r domain.CapturedRequest) bool {
	for name := range r.Headers {
		lower := strings.ToLower(name)
		for _, k := range idempotencyHeaders {
			if lower == k {
				return true
			}
		}
	}
	return false
}

// sharesIdentifierWithOthers reports whether this Request's resource prefix
// appears in other captured Requests — a hint that some other endpoint
// mints the identifier this one consumes, which is the shape a
// setup-then-act race takes.
func sharesIdentifierWithOthers(r domain.CapturedRequest, all []domain.CapturedRequest) bool {
	prefix := firstSegment(r.Endpoint.Path)
	if prefix == "" {
		return false
	}
	for _, other := range all {
		if other.ID != r.ID && firstSegment(other.Endpoint.Path) == prefix {
			return true
		}
	}
	return false
}

func firstSegment(path string) string {
	trimmed := strings.Trim(path, "/")
	if i := strings.Index(trimmed, "/"); i >= 0 {
		return trimmed[:i]
	}
	return trimmed
}

// ProbeResult is one sequential execution observed during Phase B.
type ProbeResult struct {
	Status int
	Body   string
}

// ProbeSender executes one Workflow instance sequentially and reports what
// came back. Wiring supplies an implementation that runs the Setup phase
// then the Act Request through the Scope Guard; tests supply a fake.
type ProbeSender func(ctx context.Context, wf domain.Workflow) (ProbeResult, error)

// ProbeInvariant is Phase B: run the Workflow sequentially up to `attempts`
// times on the same state and look for a rejection. A rejection after at
// least one success means a limit exists, so an Invariant can be inferred;
// no rejection means no established Invariant, and the Candidate must be
// dropped rather than tested (ADR-015).
//
// The returned bool is the gate: false means "no Invariant, drop this
// Workflow", and is the normal, correct outcome for a legitimately
// non-idempotent endpoint.
func ProbeInvariant(ctx context.Context, send ProbeSender, wf domain.Workflow, attempts int) (domain.Invariant, bool, error) {
	if attempts < 2 {
		attempts = 2 // a limit cannot be observed without at least a second execution
	}
	successes := 0
	for i := 0; i < attempts; i++ {
		res, err := send(ctx, wf)
		if err != nil {
			return domain.Invariant{}, false, fmt.Errorf("ranking: probe execution %d: %w", i+1, err)
		}
		switch {
		case res.Status >= 200 && res.Status < 300:
			successes++
		case discovery.IsLimitSignal(res.Status, res.Body):
			if successes == 0 {
				// Rejected before ever succeeding: the probe never established
				// the endpoint works at all, so there is no calibrated limit —
				// not an Invariant, just an unusable candidate.
				return domain.Invariant{}, false, nil
			}
			return domain.Invariant{
				Type:   inferType(wf, res),
				Value:  successes,
				Source: "inferred",
			}, true, nil
		case discovery.IsPreconditionFailure(res.Status, res.Body):
			// Missing precondition, not a limit — the Workflow needs a Setup
			// phase before it can be judged at all.
			return domain.Invariant{}, false, nil
		}
	}
	// Every execution succeeded: no limit is observable, so there is no
	// Invariant and therefore no possible Finding. This is the FP gate doing
	// its job on a non-idempotent endpoint.
	return domain.Invariant{}, false, nil
}

// typeRule maps observed rejection wording and endpoint naming onto one of
// the four canonical invariant types (Design/DOMAIN.md §Invariant). Rules
// are evaluated in order, so the most specific signal wins.
type typeRule struct {
	invariant  domain.InvariantType
	bodyTokens []string
	pathTokens []string
}

var typeRules = []typeRule{
	// A rate-limit rejection is a *windowed* max_successes, not a separate
	// invariant type (Context/DECISIONS.md ADR-003). Listed first so the
	// wording wins over any coincidental path match.
	{domain.InvariantMaxSuccesses, []string{"too many", "rate limit", "rate-limit"}, nil},
	{domain.InvariantMonotonicLimit,
		[]string{"out of stock", "insufficient", "sold out", "overdraw"},
		[]string{"reserve", "stock", "inventory", "withdraw", "balance"}},
	{domain.InvariantUniqueness,
		[]string{"taken", "already exists", "duplicate key"},
		[]string{"signup", "register", "create-account"}},
	{domain.InvariantSingleTransition,
		[]string{"not pending", "already confirmed", "invalid state", "invalid transition"},
		[]string{"confirm", "cancel", "transition", "approve"}},
}

// inferType picks the canonical invariant type that best fits what the
// probe saw. Falls back to max_successes — the most general "at most L
// successes" shape — when nothing more specific matches.
func inferType(wf domain.Workflow, res ProbeResult) domain.InvariantType {
	path := strings.ToLower(actPath(wf))
	body := strings.ToLower(res.Body)
	if res.Status == 429 {
		return domain.InvariantMaxSuccesses
	}
	for _, rule := range typeRules {
		if containsAny(body, rule.bodyTokens) || containsAny(path, rule.pathTokens) {
			return rule.invariant
		}
	}
	return domain.InvariantMaxSuccesses
}

func containsAny(haystack string, needles []string) bool {
	for _, n := range needles {
		if strings.Contains(haystack, n) {
			return true
		}
	}
	return false
}

func actPath(wf domain.Workflow) string {
	if spec, ok := wf.RequestByID(wf.ActRequest); ok {
		return spec.URL
	}
	return ""
}

// Ranked is a Workflow with its Phase A prior attached, before Phase B has
// decided whether it survives the invariant gate.
type Ranked struct {
	Workflow domain.Workflow
	Score    float64
	Signals  domain.ScoreSignals
}

// RankWorkflows applies Phase A to every Workflow and returns them best
// first. Stable on ties by workflow ID so repeated scans of the same target
// queue experiments in the same order.
func RankWorkflows(workflows []domain.Workflow, captured []domain.CapturedRequest) []Ranked {
	byID := make(map[string]domain.CapturedRequest, len(captured))
	for _, c := range captured {
		byID[c.ID] = c
	}

	ranked := make([]Ranked, 0, len(workflows))
	for _, wf := range workflows {
		act, ok := byID[wf.ActRequest]
		if !ok {
			continue
		}
		score, signals := StaticScore(act, captured)
		if score <= 0 {
			continue
		}
		ranked = append(ranked, Ranked{Workflow: wf, Score: score, Signals: signals})
	}
	sort.SliceStable(ranked, func(i, j int) bool {
		if ranked[i].Score != ranked[j].Score {
			return ranked[i].Score > ranked[j].Score
		}
		return ranked[i].Workflow.ID < ranked[j].Workflow.ID
	})
	return ranked
}

// ApplyUserInvariants returns the operator-declared Level 5 invariant for
// an endpoint, if one matches. User declarations are authoritative over
// inference and always outrank it (Design/ARCHITECTURE.md §4 Level 5).
func ApplyUserInvariants(declared []domain.UserInvariant, method, path string) (domain.UserInvariant, bool) {
	for _, d := range declared {
		if d.Match.Matches(method, path) {
			d.Invariant.Source = "declared"
			return d, true
		}
	}
	return domain.UserInvariant{}, false
}
