package domain

// ObservableStat records the sequential values and variability of one
// extracted Observable across Baseline runs (Design/DATA_MODEL.md
// baseline.json observables_baseline).
type ObservableStat struct {
	Seq         []int  `json:"seq"`
	Variability string `json:"variability"`
}

// Baseline is the sequential control phase of an Experiment: it calibrates
// the success/reject classifier and measures natural variability
// (Design/DOMAIN.md §Baseline). Baseline runs never enter the Wilson
// reproduction statistics.
type Baseline struct {
	CandidateID          string                    `json:"candidate_id"`
	Runs                 int                       `json:"runs"`
	ExpectedSuccessCount int                       `json:"expected_success_count"`
	SuccessSignature     Matcher                   `json:"success_signature"`
	RejectSignature      Matcher                   `json:"reject_signature"`
	ObservablesBaseline  map[string]ObservableStat `json:"observables_baseline"`
	ClassifierSeparation string                    `json:"classifier_separation"` // "clean" | "weak"
	BaselineStateReset   string                    `json:"baseline_state_reset"`
	ResetRecipe          *ResetRecipe              `json:"reset_recipe,omitempty"`
}
