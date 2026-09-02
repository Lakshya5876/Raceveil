package oracle

import (
	"math/rand"
	"testing"

	"github.com/Lakshya5876/Raceveil/internal/domain"
)

// This file is TEST_PLAN.md Layer 4's statistical calibration: simulated
// trial streams with known ground truth, run through the exact same
// reproduction-statistics/stopping machinery the scheduler uses
// (oracle.BandFor / oracle.BandCanStillChange), so the numbers it reports
// are evidence about the real decision rule, not a toy model of it.
//
// simMaxTrials/simWarmUpTrials mirror scheduler.maxTrials/warmUpTrials
// (internal/application/scheduler/run.go). They are re-declared here rather
// than imported because scheduler imports oracle — importing back would be
// a cycle — and because Design/ARCHITECTURE.md §5's stopping criteria is
// itself oracle-owned logic; the scheduler's constant is the operational
// knob, this one is what calibration measures against. If the scheduler
// constant changes, adjust this one to match so the calibration stays
// representative.
const (
	simMaxTrials    = 6
	simWarmUpTrials = 1 // simulated but not counted as evidence, matching runTrials
)

// simulateOneExperiment runs one synthetic Experiment: up to simMaxTrials
// Bernoulli(trueRate) trials (every trial independent, since Layer 4's
// mixed-independence handling is already covered by TestEvaluate_Mixed*),
// stopping as soon as oracle.BandCanStillChange says no remaining trial
// could move the verdict — exactly scheduler.runLoop's rule. Returns the
// final Confidence band, or "" if no violation was ever observed.
func simulateOneExperiment(rng *rand.Rand, trueRate float64, corroboration []string) domain.Confidence {
	// Warm-up trials are generated and discarded, matching production: they
	// exist to prime state, not to be counted.
	for i := 0; i < simWarmUpTrials; i++ {
		_ = rng.Float64() < trueRate
	}

	r, k := 0, 0
	for trial := 1; trial <= simMaxTrials; trial++ {
		if rng.Float64() < trueRate {
			r++
		}
		k++
		remaining := simMaxTrials - trial
		if !BandCanStillChange(r, k, remaining, "clean", corroboration) {
			break
		}
	}
	if r == 0 {
		return "" // no violation observed at all -> not a Finding, any rate
	}
	return BandFor(r, k, "clean", corroboration)
}

// bandCounts tabulates simulateOneExperiment outcomes over n runs.
type bandCounts struct {
	none, suspected, likely, confirmed, total int
}

func runSimulation(seed int64, trueRate float64, corroboration []string, n int) bandCounts {
	rng := rand.New(rand.NewSource(seed))
	var c bandCounts
	for i := 0; i < n; i++ {
		switch simulateOneExperiment(rng, trueRate, corroboration) {
		case domain.Confirmed:
			c.confirmed++
		case domain.Likely:
			c.likely++
		case domain.Suspected:
			c.suspected++
		default:
			c.none++
		}
		c.total++
	}
	return c
}

func (c bandCounts) atLeastLikelyRate() float64 {
	return float64(c.likely+c.confirmed) / float64(c.total)
}

func (c bandCounts) confirmedRate() float64 {
	return float64(c.confirmed) / float64(c.total)
}

// TestCalibration_TrueRateZero_NeverFires is the false-positive half of
// TEST_PLAN.md Layer 4: a stream from a correct (synchronized-safe) system
// — true violation rate 0 — must never reach any Confidence band, at any
// corroboration profile. This is deterministic, not merely low-probability:
// rng.Float64() < 0.0 is never true, so r stays 0 and simulateOneExperiment
// always returns "".
func TestCalibration_TrueRateZero_NeverFires(t *testing.T) {
	for _, corrob := range [][]string{nil, {"body_differential"}, {"post_state"}} {
		c := runSimulation(1, 0.0, corrob, 500)
		if c.none != c.total {
			t.Errorf("corroboration=%v: %d/%d simulated safe-system runs produced a Confidence band, want 0 (CONFIRMED-FP and LIKELY-FP must both be 0 at true rate 0)",
				corrob, c.total-c.none, c.total)
		}
	}
}

// TestCalibration_NoCorroboration_NeverConfirmed is the corroboration-rule
// guarantee restated as a population statistic: across the full range of
// true violation rates, with no Level 2/4 observable available, CONFIRMED
// never fires — not even at true rate 1.0 with maximal reproduction.
func TestCalibration_NoCorroboration_NeverConfirmed(t *testing.T) {
	for _, rate := range []float64{0.05, 0.15, 0.30, 0.50, 0.80, 1.0} {
		c := runSimulation(2, rate, nil, 500)
		if c.confirmed != 0 {
			t.Errorf("true rate %.2f, no corroboration: %d/%d runs reached CONFIRMED, want 0", rate, c.confirmed, c.total)
		}
	}
}

// TestCalibration_HighRateWithCorroboration_ReachesConfirmed is the other
// half of the corroboration rule: given a real, strongly-reproducing
// violation (true rate 0.8+) and a corroborating observable, CONFIRMED must
// actually be reachable — the corroboration requirement is a bar, not a
// permanent lock.
func TestCalibration_HighRateWithCorroboration_ReachesConfirmed(t *testing.T) {
	c := runSimulation(3, 0.9, []string{"body_differential"}, 500)
	if rate := c.confirmedRate(); rate < 0.90 {
		t.Errorf("true rate 0.9 with corroboration: CONFIRMED rate = %.3f (%d/%d), want >= 0.90 — high-reproduction real violations should reliably reach CONFIRMED",
			rate, c.confirmed, c.total)
	}
}

// TestCalibration_LowRateWithCorroboration_ReachesAtLeastLikely is
// TEST_PLAN.md Layer 4's false-negative check: "low-reproduction real
// violations (e.g. true rate 15%) must still reach at least LIKELY within
// K_max." Detecting a 15%-probability event in simMaxTrials=6 draws is
// fundamentally probabilistic — no finite protocol detects it every time —
// so this asserts a floor on statistical power (P(at least one hit in 6
// draws at p=0.15) = 1-0.85^6 ≈ 0.623) rather than 100%, and reports the
// measured rate as the calibration evidence TEST_PLAN.md asks for.
func TestCalibration_LowRateWithCorroboration_ReachesAtLeastLikely(t *testing.T) {
	c := runSimulation(4, 0.15, []string{"body_differential"}, 2000)
	if rate := c.atLeastLikelyRate(); rate < 0.55 {
		t.Errorf("true rate 0.15: at-least-LIKELY detection rate = %.3f (%d/%d), want >= 0.55 (theoretical ceiling at 6 draws ≈ 0.623)",
			rate, c.likely+c.confirmed, c.total)
	}
	t.Logf("true rate 0.15, K_max=%d: found=%.3f likely+=%.3f confirmed=%.3f (n=%d)",
		simMaxTrials, 1-float64(c.none)/float64(c.total), c.atLeastLikelyRate(), c.confirmedRate(), c.total)
}

// TestCalibration_PrecisionRecallReport runs the full rate grid at both
// corroboration profiles and logs a table — the precision/recall evidence
// TEST_PLAN.md Layer 4 asks for to justify (or replace) the provisional-v0
// band thresholds. It does not itself assert new behavior beyond the tests
// above; it exists so `go test -v ./internal/application/oracle -run
// Calibration_PrecisionRecall` produces a reviewable report on demand.
//
// Current disposition (recorded here rather than only in a throwaway log):
// this synthetic sweep shows CONFIRMED-FP = 0 at every rate and profile, and
// CONFIRMED reachable with high probability once a real violation
// corroborates and reproduces at a moderate-to-high rate. Combined with the
// zero-false-positive / zero-false-negative result on the live two-stack
// corpus (corpus/node, corpus/spring-boot — 5/5 vulnerable archetypes
// CONFIRMED, 5/5 safe twins clean), there is no evidence yet to justify
// moving provisional-v0's numeric cutoffs (r>=2, Wilson95 lower bound
// >=0.10). Revisit once Layer 7/8 (PortSwigger regression, real-world OSS
// validation) supply a larger, more adversarial sample.
func TestCalibration_PrecisionRecallReport(t *testing.T) {
	rates := []float64{0.0, 0.05, 0.10, 0.15, 0.20, 0.30, 0.50, 0.80, 1.0}
	profiles := map[string][]string{"no_corroboration": nil, "with_corroboration": {"body_differential"}}
	for _, name := range []string{"no_corroboration", "with_corroboration"} {
		t.Logf("--- profile=%s (K_max=%d, n=1000 per rate) ---", name, simMaxTrials)
		for i, rate := range rates {
			c := runSimulation(int64(100+i), rate, profiles[name], 1000)
			t.Logf("true_rate=%.2f  found=%.3f  >=LIKELY=%.3f  CONFIRMED=%.3f",
				rate, 1-float64(c.none)/float64(c.total), c.atLeastLikelyRate(), c.confirmedRate())
		}
	}
}
