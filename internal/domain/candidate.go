package domain

// Matcher classifies a response as success or rejection, either declared in
// candidate.yaml (success_when/reject_when) or learned from the Baseline
// (Design/DATA_MODEL.md candidate.yaml).
type Matcher struct {
	Status       int    `yaml:"status" json:"status"`
	BodyContains string `yaml:"body_contains,omitempty" json:"body_contains,omitempty"`
}

// PostStateProbe is a read-only Request issued after a burst to corroborate
// a violation via persisted state (Design/DOMAIN.md §Observable, Oracle
// Level 4).
type PostStateProbe struct {
	Method  string `yaml:"method" json:"method"`
	Path    string `yaml:"path" json:"path"`
	Extract string `yaml:"extract" json:"extract"`
}

// ResetRecipeKind is one of the four executable reset strategies that
// establish State Independence (Design/DOMAIN.md §State Independence).
// Phase 1 implements FreshCode and SetupRecipe only.
type ResetRecipeKind string

const (
	ResetFreshCode      ResetRecipeKind = "fresh_code"
	ResetEndpoint       ResetRecipeKind = "reset_endpoint"
	ResetFixtureCommand ResetRecipeKind = "fixture_command"
	ResetSetupRecipe    ResetRecipeKind = "setup_recipe"
)

// ResetRecipe describes how RaceVeil obtains fresh state between trials.
type ResetRecipe struct {
	Kind     ResetRecipeKind `yaml:"kind" json:"kind"`
	SetupRef string          `yaml:"setup_ref,omitempty" json:"setup_ref,omitempty"`
	How      string          `yaml:"how,omitempty" json:"how,omitempty"`
}

// Candidate is a (Workflow, Invariant) pair RaceVeil hypothesizes may
// exhibit a concurrency-induced violation (Design/DOMAIN.md §Candidate).
// Both declared (candidate.yaml, MVP) and inferred (v1 discovery) origins
// normalize to this one representation.
type Candidate struct {
	ID             string          `json:"id"`
	Source         string          `json:"source"` // "declared" | "inferred"
	Score          *float64        `json:"score"`
	Workflow       Workflow        `json:"workflow"`
	SessionRef     string          `yaml:"session_ref" json:"session_ref"`
	Invariant      Invariant       `json:"invariant"`
	SuccessWhen    *Matcher        `yaml:"success_when,omitempty" json:"success_when,omitempty"`
	RejectWhen     *Matcher        `yaml:"reject_when,omitempty" json:"reject_when,omitempty"`
	PostStateProbe *PostStateProbe `yaml:"post_state_probe,omitempty" json:"post_state_probe,omitempty"`
	ResetRecipe    *ResetRecipe    `yaml:"reset_recipe,omitempty" json:"reset_recipe,omitempty"`
}
