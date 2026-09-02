package domain

import "time"

// InvariantType is one of the four canonical invariant types fixed by
// Design/DOMAIN.md §Invariant. Phase 1 implements MaxSuccesses only; the
// others are recognized as valid types but not yet evaluable.
type InvariantType string

const (
	InvariantMaxSuccesses     InvariantType = "max_successes"
	InvariantUniqueness       InvariantType = "uniqueness"
	InvariantMonotonicLimit   InvariantType = "monotonic_limit"
	InvariantSingleTransition InvariantType = "single_transition"
)

// Window is the optional temporal scope of a max_successes Invariant
// (Design/DOMAIN.md §Invariant: rate-limit bypass is windowed max_successes,
// not a separate type). MVP/v1 use a fixed window beginning at the first Act
// Request; sliding-window semantics are deferred.
type Window struct {
	Duration time.Duration `yaml:"duration" json:"duration"`
}

// Invariant is a predicate expected to hold over a Workflow's Observed
// behavior (Design/DOMAIN.md §Invariant).
type Invariant struct {
	Type   InvariantType `yaml:"type" json:"type"`
	Value  int           `yaml:"value" json:"value"`
	Window *Window       `yaml:"window,omitempty" json:"window,omitempty"`
	Source string        `yaml:"source,omitempty" json:"source,omitempty"` // "declared" | "inferred"
}

// RequiredProofEffects returns the minimum number of successful effects
// needed to establish a violation of this Invariant, per the type-specific
// rule in Design/DOMAIN.md §Invariant (Proof requirement). Only
// max_successes is implemented in Phase 1; the other three canonical types
// return their documented minimums for completeness even though their
// oracle evaluation is not yet built.
func RequiredProofEffects(inv Invariant) int {
	switch inv.Type {
	case InvariantMaxSuccesses:
		return inv.Value + 1
	case InvariantUniqueness:
		return 2
	case InvariantMonotonicLimit:
		return 1
	case InvariantSingleTransition:
		return 2
	default:
		return 0
	}
}
