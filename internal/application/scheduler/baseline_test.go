package scheduler

import (
	"context"
	"testing"

	"github.com/Lakshya5876/Raceveil/internal/domain"
)

// TestRunBaseline_MonotonicLimit_ClassifierSeparationIsClean is a
// regression test: runBaseline used to calibrate with
// required_proof_effects calls, which is a constant 1 for monotonic_limit
// regardless of the bound — so it never observed a reject response and
// always reported "weak" separation, capping every monotonic_limit finding
// at SUSPECTED even with perfect reproduction (found via the Phase 2
// corpus's inventory-overrun archetype). Calibration must use
// Invariant.Value+1 calls instead.
func TestRunBaseline_MonotonicLimit_ClassifierSeparationIsClean(t *testing.T) {
	srv := startFixture(t)
	candidate := redeemCandidate("/issue-code", "/redeem")
	candidate.Invariant.Type = domain.InvariantMonotonicLimit // required_proof_effects=1, but calibration still needs value+1=2 calls
	cfg := runConfig(t, candidate, "http://"+srv.Addr())

	baseline, err := runBaseline(context.Background(), cfg, cfg.Scope.Target)
	if err != nil {
		t.Fatalf("runBaseline: %v", err)
	}
	if baseline.ClassifierSeparation != "clean" {
		t.Fatalf("ClassifierSeparation = %q, want \"clean\" — calibration must observe both a success and a reject", baseline.ClassifierSeparation)
	}
}
