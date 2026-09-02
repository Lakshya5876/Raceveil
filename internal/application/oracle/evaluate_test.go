package oracle

import (
	"testing"

	"github.com/Lakshya5876/Raceveil/internal/domain"
)

func trial(n int, violation bool, indep domain.StateIndependence) domain.ConcurrentTrial {
	return domain.ConcurrentTrial{Trial: n, N: 2, Violation: violation, StateIndependence: indep}
}

func baseInput(trials []domain.ConcurrentTrial) EvaluateInput {
	return EvaluateInput{
		CandidateID:          "cand_redeem",
		Invariant:            domain.Invariant{Type: domain.InvariantMaxSuccesses, Value: 1},
		RequiredProofEffects: 2,
		ClassifierSeparation: "clean",
		Trials:               trials,
	}
}

func TestEvaluate_NoViolation_NoFinding(t *testing.T) {
	trials := []domain.ConcurrentTrial{
		trial(1, false, domain.Independent),
		trial(2, false, domain.Independent),
	}
	_, found := Evaluate(baseInput(trials))
	if found {
		t.Fatal("expected no Finding when every trial obeys the invariant (the safe-twin case)")
	}
}

func TestEvaluate_NonIdempotentNoInvariant_IsSchedulerConcern(t *testing.T) {
	// The FP gate (no invariant established -> no Candidate at all) is
	// enforced upstream in the scheduler before an Experiment exists; the
	// Oracle only ever sees Experiments that already have an Invariant, so
	// there's nothing to additionally assert here — documented, not tested.
	t.Skip("FP gate enforced in scheduler.LoadCandidate before an Oracle evaluation can occur")
}

func TestEvaluate_Level3Alone_CapsAtLikely(t *testing.T) {
	// TEST_PLAN.md Layer 3 corroboration matrix: Level 3 alone (no Level
	// 2/4, e.g. the Candidate declared neither a body_differential nor a
	// post_state_probe) can never reach CONFIRMED, even with strong
	// reproduction.
	trials := []domain.ConcurrentTrial{
		trial(1, true, domain.Independent),
		trial(2, true, domain.Independent),
		trial(3, true, domain.Independent),
		trial(4, true, domain.Independent),
		trial(5, true, domain.Independent),
		trial(6, false, domain.Independent),
	}
	result, found := Evaluate(baseInput(trials))
	if !found {
		t.Fatal("expected a Finding")
	}
	if result.Confidence != domain.Likely {
		t.Errorf("Confidence = %v, want LIKELY (no corroborating observable supplied)", result.Confidence)
	}
	if result.Trials.KIndependent != 6 || result.Trials.RViolations != 5 {
		t.Errorf("K/r = %d/%d, want 6/5", result.Trials.KIndependent, result.Trials.RViolations)
	}
}

func TestEvaluate_Level3PlusCorroboration_CanReachConfirmed(t *testing.T) {
	// Proves classifyConfidence's CONFIRMED path: with Level 3 (cross-request
	// consistency) plus a real Level 2/4 corroborating observable — wired by
	// the scheduler in trials.go's collectCorroboration/corroborationFromTrials
	// when the Candidate declares a body_differential or post_state_probe —
	// strong reproduction reaches CONFIRMED.
	in := baseInput([]domain.ConcurrentTrial{
		trial(1, true, domain.Independent),
		trial(2, true, domain.Independent),
		trial(3, true, domain.Independent),
		trial(4, true, domain.Independent),
		trial(5, true, domain.Independent),
		trial(6, false, domain.Independent),
	})
	in.CorroboratingObservables = []string{"post_state"}
	result, found := Evaluate(in)
	if !found {
		t.Fatal("expected a Finding")
	}
	if result.Confidence != domain.Confirmed {
		t.Errorf("Confidence = %v, want CONFIRMED with Level 3 + Level 4 corroboration", result.Confidence)
	}
}

func TestClassifyConfidence_NeverConfirmsWithoutCorroboration(t *testing.T) {
	// The corroboration-rule guarantee: classifyConfidence refuses CONFIRMED
	// for any r/k/wilsonLo combination once corroboration is empty — the
	// shape of any Experiment whose Candidate declared neither a
	// body_differential nor a post_state_probe.
	cases := []classifyInput{
		{rViolations: 2, kIndependent: 2, wilsonLo: 0.99, classifierSeparation: "clean"},
		{rViolations: 10, kIndependent: 10, wilsonLo: 0.80, classifierSeparation: "clean"},
		{rViolations: 5, kIndependent: 6, wilsonLo: 0.436, classifierSeparation: "clean"},
	}
	for _, c := range cases {
		if got := classifyConfidence(c); got == domain.Confirmed {
			t.Errorf("classifyConfidence(%+v) = CONFIRMED with no corroboration, want LIKELY at most", c)
		}
	}
}

func TestEvaluate_NoCorroboration_NeverConfirmed(t *testing.T) {
	// An Evaluate call with no corroborating observables (the Candidate
	// declared no body_differential/post_state_probe) must produce a
	// Confidence other than CONFIRMED, and must not panic doing so: the
	// panic in Evaluate is a defensive guard against a future
	// classifyConfidence regression, not a path this test should trigger.
	trials := make([]domain.ConcurrentTrial, 0, 10)
	for i := 0; i < 10; i++ {
		trials = append(trials, trial(i, true, domain.Independent))
	}
	in := baseInput(trials)
	in.CorroboratingObservables = nil
	result, found := Evaluate(in)
	if !found {
		t.Fatal("expected a Finding")
	}
	if result.Confidence == domain.Confirmed {
		t.Fatal("Evaluate returned CONFIRMED with no corroborating observable")
	}
}

func TestEvaluate_MixedIndependentAndDependent_ExcludesDependentFromStats(t *testing.T) {
	// TEST_PLAN.md Layer 4: 5 independent + 2 dependent -> K_independent=5,
	// r_violations counts only among the 5; dependent trials are retained as
	// evidence (anyViolation) but excluded from K/r.
	trials := []domain.ConcurrentTrial{
		trial(1, true, domain.Independent),
		trial(2, true, domain.Independent),
		trial(3, false, domain.Independent),
		trial(4, false, domain.Independent),
		trial(5, false, domain.Independent),
		trial(6, true, domain.Dependent),
		trial(7, true, domain.Dependent),
	}
	result, found := Evaluate(baseInput(trials))
	if !found {
		t.Fatal("expected a Finding (dependent trials are still Evidence)")
	}
	if result.Trials.KIndependent != 5 {
		t.Errorf("K_independent = %d, want 5", result.Trials.KIndependent)
	}
	if result.Trials.RViolations != 2 {
		t.Errorf("r_violations = %d, want 2 (dependent violations excluded)", result.Trials.RViolations)
	}
	if result.Confidence == domain.Confirmed {
		t.Error("must never be CONFIRMED without corroboration")
	}
}

func TestEvaluate_AllDependentOrUnknown_CapsAtLikely(t *testing.T) {
	trials := []domain.ConcurrentTrial{
		trial(1, true, domain.Dependent),
		trial(2, true, domain.UnknownIndependence),
	}
	result, found := Evaluate(baseInput(trials))
	if !found {
		t.Fatal("expected a Finding")
	}
	if result.Trials.KIndependent != 0 {
		t.Errorf("K_independent = %d, want 0", result.Trials.KIndependent)
	}
	if result.Confidence != domain.Likely {
		t.Errorf("Confidence = %v, want LIKELY (reproduction stats undefined with 0 independent trials)", result.Confidence)
	}
}

func TestEvaluate_WeakClassifierSeparation_CapsAtSuspected(t *testing.T) {
	in := baseInput([]domain.ConcurrentTrial{
		trial(1, true, domain.Independent),
		trial(2, true, domain.Independent),
	})
	in.ClassifierSeparation = "weak"
	result, found := Evaluate(in)
	if !found {
		t.Fatal("expected a Finding")
	}
	if result.Confidence != domain.Suspected {
		t.Errorf("Confidence = %v, want SUSPECTED with weak classifier separation", result.Confidence)
	}
}

func TestEvaluate_RequiredProofEffectsRecorded(t *testing.T) {
	in := baseInput([]domain.ConcurrentTrial{trial(1, true, domain.Independent)})
	result, found := Evaluate(in)
	if !found {
		t.Fatal("expected a Finding")
	}
	if result.RequiredProofEffects != 2 {
		t.Errorf("RequiredProofEffects = %d, want 2", result.RequiredProofEffects)
	}
	if len(result.PrimaryOracleLevels) != 1 || result.PrimaryOracleLevels[0] != 3 {
		t.Errorf("PrimaryOracleLevels = %v, want [3]", result.PrimaryOracleLevels)
	}
}
