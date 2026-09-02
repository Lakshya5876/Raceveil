package domain

// Confidence is a graded assessment of a violation, derived from
// reproduction statistics and corroborating Observables (Design/DOMAIN.md
// §Confidence). Never hardcoded — every input is recorded in OracleResult.
type Confidence string

const (
	Suspected Confidence = "SUSPECTED"
	Likely    Confidence = "LIKELY"
	Confirmed Confidence = "CONFIRMED"
)

// Severity is an advisory impact label derived from the Invariant type and
// effect (Design/DOMAIN.md §Severity). It never gates a Finding.
type Severity struct {
	Label string `json:"label"`
	Basis string `json:"basis"`
}

// Reproduction is the reproduction-statistics block of an OracleResult:
// K_independent independent trials, r_violations among them, and the Wilson
// score interval computed from them (Design/ARCHITECTURE.md §5).
type ReproductionStats struct {
	KIndependent int        `json:"K_independent"`
	RViolations  int        `json:"r_violations"`
	PHat         float64    `json:"p_hat"`
	Wilson95     [2]float64 `json:"wilson95"`
}

// OracleResult is the Oracle's evaluation of one Experiment
// (Design/DATA_MODEL.md experiments/<id>/oracle.json).
type OracleResult struct {
	CandidateID                string            `json:"candidate_id"`
	Invariant                  Invariant         `json:"invariant"`
	RequiredProofEffects       int               `json:"required_proof_effects"`
	PrimaryOracleLevels        []int             `json:"primary_oracle_levels"`
	LevelsEvaluated            []int             `json:"levels_evaluated"`
	Trials                     ReproductionStats `json:"trials"`
	IndependentTrials          int               `json:"independent_trials"`
	DependentOrUnknownExcluded int               `json:"dependent_or_unknown_excluded"`
	Reproduction               ReproductionStats `json:"reproduction"`
	StateIndependence          StateIndependence `json:"state_independence"`
	ConfidenceBandsVersion     string            `json:"confidence_bands_version"`
	CorroboratingObservables   []string          `json:"corroborating_observables"`
	Confidence                 Confidence        `json:"confidence"`
	Severity                   Severity          `json:"severity"`
	Why                        string            `json:"why"`
}
