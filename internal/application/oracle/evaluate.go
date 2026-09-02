package oracle

import (
	"fmt"

	"github.com/Lakshya5876/Raceveil/internal/domain"
)

// ConfirmedBandsVersion identifies the provisional confidence-band numeric
// thresholds in use (Design/ARCHITECTURE.md §5 provisional-v0) — bumped
// when Layer 4 statistical calibration replaces these with tuned defaults.
const ConfirmedBandsVersion = "provisional-v0"

// EvaluateInput is everything the Oracle needs to judge one Experiment.
// Trials must already carry a per-trial StateIndependence, SuccessCountS,
// and Violation (Design/DOMAIN.md §ConcurrentTrial) — computed by the
// scheduler from Classify's Level 1 verdicts against the Invariant.
type EvaluateInput struct {
	CandidateID          string
	Invariant            domain.Invariant
	RequiredProofEffects int
	ClassifierSeparation string // "clean" | "weak" (Design/DATA_MODEL.md baseline.json)
	Trials               []domain.ConcurrentTrial
	// CorroboratingObservables lists Level 2/4 observables that corroborate
	// the Level 3 violation (Design/ARCHITECTURE.md §4 corroboration rule).
	// Empty when the Candidate declares neither a body_differential nor a
	// post_state_probe, which caps the verdict at LIKELY by design.
	CorroboratingObservables []string
}

// Evaluate judges one Experiment's trials and returns the Oracle's verdict.
// found reports whether any violation was observed at all (independent or
// not) — the scheduler only emits a Finding when found is true; a candidate
// with zero violations across every trial (the synchronized-safe case) is
// not a Finding at any Confidence.
func Evaluate(in EvaluateInput) (result domain.OracleResult, found bool) {
	anyViolation := false
	kIndependent, rViolations := 0, 0
	for _, t := range in.Trials {
		if t.Violation {
			anyViolation = true
		}
		if t.StateIndependence == domain.Independent {
			kIndependent++
			if t.Violation {
				rViolations++
			}
		}
	}

	result = domain.OracleResult{
		CandidateID:                in.CandidateID,
		Invariant:                  in.Invariant,
		RequiredProofEffects:       in.RequiredProofEffects,
		PrimaryOracleLevels:        []int{3},
		LevelsEvaluated:            levelsEvaluated(in.CorroboratingObservables),
		StateIndependence:          aggregateStateIndependence(in.Trials),
		ConfidenceBandsVersion:     ConfirmedBandsVersion,
		CorroboratingObservables:   in.CorroboratingObservables,
		IndependentTrials:          kIndependent,
		DependentOrUnknownExcluded: len(in.Trials) - kIndependent,
	}
	if !anyViolation {
		return result, false
	}

	var wilsonLo, wilsonHi, pHat float64
	if kIndependent > 0 {
		pHat = float64(rViolations) / float64(kIndependent)
		wilsonLo, wilsonHi = WilsonInterval(rViolations, kIndependent, Z95)
	}
	stats := domain.ReproductionStats{
		KIndependent: kIndependent,
		RViolations:  rViolations,
		PHat:         pHat,
		Wilson95:     [2]float64{wilsonLo, wilsonHi},
	}
	result.Trials = stats
	result.Reproduction = stats

	confidence := classifyConfidence(classifyInput{
		rViolations:          rViolations,
		kIndependent:         kIndependent,
		wilsonLo:             wilsonLo,
		classifierSeparation: in.ClassifierSeparation,
		corroboration:        in.CorroboratingObservables,
	})
	if confidence == domain.Confirmed && len(in.CorroboratingObservables) == 0 {
		// CONFIRMED requires a Level 2/4 corroborating observable
		// (Design/ARCHITECTURE.md §4 corroboration rule) — reaching this
		// branch with none present means classifyConfidence has a bug, not
		// that the evidence is weak. Fail loudly rather than silently
		// mislabel a Finding as more certain than its evidence supports.
		panic(fmt.Sprintf("oracle: CONFIRMED reached without a corroborating observable (candidate %s) — classifyConfidence logic error", in.CandidateID))
	}
	result.Confidence = confidence
	result.Severity = severityFor(in.Invariant)
	result.Why = explain(confidence, rViolations, kIndependent, wilsonLo, in.Invariant)
	return result, true
}

// levelsEvaluated reports which Oracle levels actually produced evidence for
// this Experiment: Level 1 (structural classification) and Level 3
// (cross-request consistency) always run; Level 2/4 are added only when the
// Candidate declared the corresponding corroboration source and it fired.
func levelsEvaluated(corroboration []string) []int {
	levels := []int{1, 3}
	for _, o := range corroboration {
		switch o {
		case "body_differential":
			levels = append(levels, 2)
		case "post_state":
			levels = append(levels, 4)
		}
	}
	return levels
}

func aggregateStateIndependence(trials []domain.ConcurrentTrial) domain.StateIndependence {
	if len(trials) == 0 {
		return domain.UnknownIndependence
	}
	weakest := domain.Independent
	for _, t := range trials {
		switch t.StateIndependence {
		case domain.Dependent:
			weakest = domain.Dependent
		case domain.UnknownIndependence:
			if weakest != domain.Dependent {
				weakest = domain.UnknownIndependence
			}
		}
	}
	return weakest
}

type classifyInput struct {
	rViolations          int
	kIndependent         int
	wilsonLo             float64
	classifierSeparation string
	corroboration        []string
}

// classifyConfidence maps reproduction statistics to a Confidence band per
// the provisional-v0 rules (Design/ARCHITECTURE.md §5, Design/UX.md
// Evidence hierarchy). Assumes anyViolation is already true (called only
// when at least one trial violated the Invariant).
func classifyConfidence(in classifyInput) domain.Confidence {
	if in.classifierSeparation == "weak" {
		return domain.Suspected
	}
	if in.kIndependent == 0 {
		// All trials dependent/unknown: evidence exists but reproduction
		// statistics are undefined — never CONFIRMED (Design/DOMAIN.md
		// §State Independence).
		return domain.Likely
	}
	if in.rViolations >= 2 && in.wilsonLo >= 0.10 && len(in.corroboration) >= 1 {
		return domain.Confirmed
	}
	return domain.Likely
}

func severityFor(inv domain.Invariant) domain.Severity {
	switch inv.Type {
	case domain.InvariantMaxSuccesses:
		return domain.Severity{Label: "HIGH", Basis: "duplicate-effect integrity violation"}
	case domain.InvariantUniqueness:
		return domain.Severity{Label: "HIGH", Basis: "duplicate resource creation"}
	case domain.InvariantMonotonicLimit:
		return domain.Severity{Label: "HIGH", Basis: "consumable limit overrun"}
	default:
		return domain.Severity{Label: "MEDIUM", Basis: "duplicate state transition"}
	}
}

func explain(confidence domain.Confidence, r, k int, wilsonLo float64, inv domain.Invariant) string {
	switch confidence {
	case domain.Confirmed:
		return fmt.Sprintf("S>%d in %d/%d independent trials (Wilson95 lower bound %.3f), corroborated by an independent observable", inv.Value, r, k, wilsonLo)
	case domain.Likely:
		if k == 0 {
			return fmt.Sprintf("S>%d observed but no independent trials were available — reproduction statistics undefined, capped at LIKELY", inv.Value)
		}
		return fmt.Sprintf("S>%d in %d/%d independent trials (Wilson95 lower bound %.3f); no corroborating observable available, or bar not met for CONFIRMED", inv.Value, r, k, wilsonLo)
	default:
		return "signal consistent with a violation, but classifier separation was weak on the Baseline"
	}
}
