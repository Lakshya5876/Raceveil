package ranking

import (
	"context"
	"errors"
	"testing"

	"github.com/Lakshya5876/Raceveil/internal/domain"
)

func captured(id, method, path string, headers map[string]string) domain.CapturedRequest {
	return domain.CapturedRequest{
		ID:       id,
		Endpoint: domain.Endpoint{Method: method, Path: path},
		URL:      path,
		Headers:  headers,
	}
}

func TestStaticScore_ReadOnlyScoresZero(t *testing.T) {
	score, _ := StaticScore(captured("r", "GET", "/coupon/redeem", nil), nil)
	if score != 0 {
		t.Errorf("a GET must score 0 regardless of how race-prone its name looks, got %v", score)
	}
}

func TestStaticScore_RacePronePathOutranksBlandOne(t *testing.T) {
	all := []domain.CapturedRequest{}
	redeem, redeemSignals := StaticScore(captured("a", "POST", "/coupon/redeem", nil), all)
	bland, _ := StaticScore(captured("b", "POST", "/telemetry/ping", nil), all)
	if redeem <= bland {
		t.Errorf("race-prone path scored %v, bland path %v — expected the race-prone one higher", redeem, bland)
	}
	if len(redeemSignals.NameHits) == 0 {
		t.Error("expected name hits to be recorded so the ranking is explainable")
	}
	if !redeemSignals.IdempotencyKeyAbsent {
		t.Error("expected IdempotencyKeyAbsent to be true when no idempotency header is present")
	}
}

func TestStaticScore_IdempotencyKeyLowersScore(t *testing.T) {
	withKey, signals := StaticScore(
		captured("a", "POST", "/coupon/redeem", map[string]string{"Idempotency-Key": "abc"}), nil)
	without, _ := StaticScore(captured("b", "POST", "/coupon/redeem", nil), nil)
	if withKey >= without {
		t.Errorf("an endpoint carrying an idempotency key should score lower (%v) than one without (%v)", withKey, without)
	}
	if signals.IdempotencyKeyAbsent {
		t.Error("IdempotencyKeyAbsent must be false when the header is present")
	}
}

func TestStaticScore_ReusedIdentifierSignal(t *testing.T) {
	act := captured("a", "POST", "/coupon/redeem", nil)
	all := []domain.CapturedRequest{act, captured("b", "POST", "/coupon/issue", nil)}
	_, signals := StaticScore(act, all)
	if !signals.ReusedIdentifier {
		t.Error("expected ReusedIdentifier when a sibling endpoint shares the resource prefix")
	}
}

// repeat builds a ProbeSender that returns the same observation for every
// attempt — the shape of an endpoint with no limit.
func repeat(r ProbeResult) ProbeSender {
	return func(_ context.Context, _ domain.Workflow, attempts int) ([]ProbeResult, error) {
		out := make([]ProbeResult, attempts)
		for i := range out {
			out[i] = r
		}
		return out, nil
	}
}

// sequence builds a ProbeSender that replays a fixed observation sequence,
// modelling one shared Setup followed by repeated Act executions.
func sequence(results ...ProbeResult) ProbeSender {
	return func(_ context.Context, _ domain.Workflow, attempts int) ([]ProbeResult, error) {
		if attempts > len(results) {
			attempts = len(results)
		}
		return results[:attempts], nil
	}
}

func workflowFor(id, path string) domain.Workflow {
	return domain.Workflow{
		ID:         "wf_" + id,
		Requests:   []domain.RequestSpec{{ID: id, Method: "POST", URL: path}},
		ActRequest: id,
	}
}

// The FP gate: an endpoint that always succeeds has no observable limit, so
// no Invariant can be established and the Candidate must be dropped
// (Context/DECISIONS.md ADR-015).
func TestProbeInvariant_NoLimitObserved_DropsCandidate(t *testing.T) {
	send := repeat(ProbeResult{Status: 201, Body: `{"id":1}`})
	_, ok, err := ProbeInvariant(context.Background(), send, workflowFor("comment", "/comment"), 3)
	if err != nil {
		t.Fatalf("ProbeInvariant: %v", err)
	}
	if ok {
		t.Fatal("a non-idempotent endpoint with no limit must NOT yield an Invariant — this is the false-positive gate")
	}
}

func TestProbeInvariant_LimitObserved_InfersInvariant(t *testing.T) {
	send := sequence(
		ProbeResult{Status: 200, Body: `{"status":"redeemed"}`},
		ProbeResult{Status: 409, Body: `already redeemed`},
		ProbeResult{Status: 409, Body: `already redeemed`},
	)
	inv, ok, err := ProbeInvariant(context.Background(), send, workflowFor("redeem", "/coupon/redeem"), 3)
	if err != nil {
		t.Fatalf("ProbeInvariant: %v", err)
	}
	if !ok {
		t.Fatal("a rejection after a success establishes a limit and must infer an Invariant")
	}
	if inv.Type != domain.InvariantMaxSuccesses {
		t.Errorf("Type = %q, want max_successes", inv.Type)
	}
	if inv.Value != 1 {
		t.Errorf("Value = %d, want 1 (one success observed before the limit bit)", inv.Value)
	}
	if inv.Source != "inferred" {
		t.Errorf("Source = %q, want inferred", inv.Source)
	}
}

func TestProbeInvariant_PreconditionFailure_DropsRatherThanInfers(t *testing.T) {
	send := repeat(ProbeResult{Status: 404, Body: "no cart found"})
	_, ok, err := ProbeInvariant(context.Background(), send, workflowFor("apply", "/cart/coupon"), 3)
	if err != nil {
		t.Fatalf("ProbeInvariant: %v", err)
	}
	if ok {
		t.Fatal("a missing precondition is not a limit and must not infer an Invariant")
	}
}

func TestProbeInvariant_RejectedBeforeAnySuccess_DropsCandidate(t *testing.T) {
	send := repeat(ProbeResult{Status: 409, Body: "already used"})
	_, ok, err := ProbeInvariant(context.Background(), send, workflowFor("redeem", "/coupon/redeem"), 3)
	if err != nil {
		t.Fatalf("ProbeInvariant: %v", err)
	}
	if ok {
		t.Fatal("without ever observing a success there is no calibrated limit to violate")
	}
}

func TestProbeInvariant_PropagatesSendError(t *testing.T) {
	wantErr := errors.New("dial refused")
	send := func(context.Context, domain.Workflow, int) ([]ProbeResult, error) {
		return nil, wantErr
	}
	if _, _, err := ProbeInvariant(context.Background(), send, workflowFor("x", "/x"), 2); !errors.Is(err, wantErr) {
		t.Fatalf("expected the send error to propagate, got %v", err)
	}
}

func TestInferType_MapsResponsesToCanonicalTypes(t *testing.T) {
	cases := []struct {
		name string
		path string
		res  ProbeResult
		want domain.InvariantType
	}{
		{"rate limit is windowed max_successes, not a new type", "/login",
			ProbeResult{Status: 429, Body: "too many requests"}, domain.InvariantMaxSuccesses},
		{"out of stock is monotonic_limit", "/inventory/reserve",
			ProbeResult{Status: 409, Body: "out of stock"}, domain.InvariantMonotonicLimit},
		{"username taken is uniqueness", "/signup",
			ProbeResult{Status: 409, Body: "username taken"}, domain.InvariantUniqueness},
		{"not pending is single_transition", "/order/confirm",
			ProbeResult{Status: 409, Body: "not pending"}, domain.InvariantSingleTransition},
		{"generic duplicate is max_successes", "/coupon/redeem",
			ProbeResult{Status: 409, Body: "already redeemed"}, domain.InvariantMaxSuccesses},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got := inferType(workflowFor("act", c.path), c.res)
			if got != c.want {
				t.Errorf("inferType = %q, want %q", got, c.want)
			}
		})
	}
}

func TestRankWorkflows_OrdersByScoreDeterministically(t *testing.T) {
	all := []domain.CapturedRequest{
		captured("ping", "POST", "/telemetry/ping", nil),
		captured("redeem", "POST", "/coupon/redeem", nil),
		captured("read", "GET", "/coupon/list", nil),
	}
	workflows := []domain.Workflow{
		workflowFor("ping", "/telemetry/ping"),
		workflowFor("redeem", "/coupon/redeem"),
		workflowFor("read", "/coupon/list"),
	}
	got := RankWorkflows(workflows, all)
	if len(got) != 2 {
		t.Fatalf("read-only workflows should be dropped; got %d ranked", len(got))
	}
	if got[0].Workflow.ActRequest != "redeem" {
		t.Errorf("expected the race-prone workflow ranked first, got %q", got[0].Workflow.ActRequest)
	}

	// Determinism: the same input must produce the same order.
	again := RankWorkflows(workflows, all)
	for i := range got {
		if got[i].Workflow.ID != again[i].Workflow.ID {
			t.Fatalf("ranking is not deterministic at position %d", i)
		}
	}
}

func TestApplyUserInvariants_DeclaredOutranksInference(t *testing.T) {
	declared := []domain.UserInvariant{{
		Match:     domain.EndpointMatch{Method: "POST", Path: "/coupon/redeem"},
		Invariant: domain.Invariant{Type: domain.InvariantMaxSuccesses, Value: 3},
	}}
	got, ok := ApplyUserInvariants(declared, "POST", "/coupon/redeem")
	if !ok {
		t.Fatal("expected the declared invariant to match")
	}
	if got.Invariant.Value != 3 || got.Invariant.Source != "declared" {
		t.Errorf("declared invariant = %+v, want value 3 marked declared", got.Invariant)
	}
	if _, ok := ApplyUserInvariants(declared, "POST", "/other"); ok {
		t.Error("a non-matching endpoint must not pick up the declaration")
	}
}

func TestEndpointMatch_WildcardPath(t *testing.T) {
	m := domain.EndpointMatch{Method: "POST", Path: "/api/*"}
	if !m.Matches("post", "/api/coupon/redeem") {
		t.Error("wildcard path should match a deeper path, case-insensitively on method")
	}
	if m.Matches("POST", "/other/thing") {
		t.Error("wildcard must not match outside its prefix")
	}
}
