package domain

import "time"

// ScopeRef is the provenance/audit metadata carried into a .rv so a reader
// knows the finding's original authorization claim (Design/SECURITY.md §2:
// not verified by RaceVeil).
type ScopeRef struct {
	AuthorizedBy string `json:"authorized_by"`
}

// SessionBootstrap describes how a replay/verify run re-establishes a
// Session (Design/DATA_MODEL.md .rv session_bootstrap).
type SessionBootstrap struct {
	Recipe         string `json:"recipe,omitempty"`
	IsolationGroup string `json:"isolation_group"`
}

// FindingOracleSummary is the oracle sub-block embedded in a .rv
// (Design/DATA_MODEL.md .rv oracle).
type FindingOracleSummary struct {
	PrimaryLevels       []int           `json:"primary_levels"`
	CorroboratingLevels []int           `json:"corroborating_levels"`
	SuccessSignature    Matcher         `json:"success_signature"`
	RejectSignature     Matcher         `json:"reject_signature"`
	PostStateProbe      *PostStateProbe `json:"post_state_probe,omitempty"`
}

// ExpectedInvariant records the invariant bound a Finding proves was broken
// (Design/DATA_MODEL.md .rv expected).
type ExpectedInvariant struct {
	MaxSuccesses int `json:"max_successes"`
}

// ObservedAtCreation freezes the reproduction statistics at the moment a
// Finding was created, distinct from a later verify re-computation
// (Design/DATA_MODEL.md .rv observed_at_creation).
type ObservedAtCreation struct {
	S                      int               `json:"S"`
	Reproduction           ReproductionStats `json:"reproduction"`
	ConfidenceBandsVersion string            `json:"confidence_bands_version"`
}

// EvidenceRefs points back into the run directory for full Evidence
// re-derivation (Design/DOMAIN.md §Evidence).
type EvidenceRefs struct {
	RunDir string `json:"run_dir"`
}

// Finding is a reproduced concurrency-induced violation packaged as a
// self-contained reproduction bundle (Design/DOMAIN.md §Finding,
// §Reproduction; Design/DATA_MODEL.md §9 findings/<id>.rv). It stores no
// live secrets — credentials are templated as ${ENV} placeholders.
type Finding struct {
	RVVersion            string               `json:"rv_version"`
	FindingID            string               `json:"finding_id"`
	Target               string               `json:"target"`
	ScopeRef             ScopeRef             `json:"scope_ref"`
	Workflow             Workflow             `json:"workflow"`
	SessionBootstrap     SessionBootstrap     `json:"session_bootstrap"`
	Invariant            Invariant            `json:"invariant"`
	RequiredProofEffects int                  `json:"required_proof_effects"`
	SyncStrategy         string               `json:"sync_strategy"`
	ConcurrencyN         int                  `json:"concurrency_N"`
	Oracle               FindingOracleSummary `json:"oracle"`
	Expected             ExpectedInvariant    `json:"expected"`
	StateIndependence    StateIndependence    `json:"state_independence"`
	ResetRecipe          *ResetRecipe         `json:"reset_recipe,omitempty"`
	ObservedAtCreation   ObservedAtCreation   `json:"observed_at_creation"`
	Confidence           Confidence           `json:"confidence"`
	Severity             Severity             `json:"severity"`
	EvidenceRefs         EvidenceRefs         `json:"evidence_refs"`
	Created              time.Time            `json:"created"`
	RNGSeed              int64                `json:"rng_seed"`
}
