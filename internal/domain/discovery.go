package domain

// ScoreSignals records why a discovered Candidate was ranked the way it was
// (Design/DATA_MODEL.md §4 score_signals). Populated only for
// source="inferred" Candidates; declared Candidates carry a nil pointer.
type ScoreSignals struct {
	Verb                 string   `json:"verb"`
	NameHits             []string `json:"name_hits"`
	IdempotencyKeyAbsent bool     `json:"idempotency_key_absent"`
	LimitObservedInProbe bool     `json:"limit_observed_in_probe"`
	ReusedIdentifier     bool     `json:"reused_identifier"`
}

// UserInvariant is one entry of invariants.yaml — an operator-declared
// Oracle Level 5 configuration that constrains or overrides what discovery
// infers (Design/DATA_MODEL.md §User invariants, Design/ARCHITECTURE.md §4
// Level 5). Level 5 is configuration, never a source of corroboration.
type UserInvariant struct {
	Match          EndpointMatch     `yaml:"match" json:"match"`
	Invariant      Invariant         `yaml:"invariant" json:"invariant"`
	SuccessWhen    *Matcher          `yaml:"success_when,omitempty" json:"success_when,omitempty"`
	RejectWhen     *Matcher          `yaml:"reject_when,omitempty" json:"reject_when,omitempty"`
	PostStateProbe *PostStateProbe   `yaml:"post_state_probe,omitempty" json:"post_state_probe,omitempty"`
	BodyDiff       *BodyDifferential `yaml:"body_differential,omitempty" json:"body_differential,omitempty"`
}

// EndpointMatch selects which discovered Endpoint a UserInvariant applies
// to.
type EndpointMatch struct {
	Method string `yaml:"method" json:"method"`
	Path   string `yaml:"path" json:"path"`
}

// Matches reports whether this match selects the given endpoint. Method
// comparison is case-insensitive; an empty Method matches any method. Path
// matching supports a trailing "*" wildcard.
func (m EndpointMatch) Matches(method, path string) bool {
	if m.Method != "" && !equalFold(m.Method, method) {
		return false
	}
	if m.Path == "" {
		return true
	}
	if suffix, ok := cutSuffix(m.Path, "*"); ok {
		return hasPrefix(path, suffix)
	}
	return m.Path == path
}

// The three helpers below keep this package dependency-free (Design/
// CLAUDE.md §1: internal/domain has zero external dependencies and imports
// from no other layer — including, by convention here, no stdlib string
// helpers that would invite richer logic to creep into the domain).
func equalFold(a, b string) bool {
	if len(a) != len(b) {
		return false
	}
	for i := 0; i < len(a); i++ {
		if lower(a[i]) != lower(b[i]) {
			return false
		}
	}
	return true
}

func lower(c byte) byte {
	if c >= 'A' && c <= 'Z' {
		return c + ('a' - 'A')
	}
	return c
}

func cutSuffix(s, suffix string) (string, bool) {
	if len(s) < len(suffix) || s[len(s)-len(suffix):] != suffix {
		return s, false
	}
	return s[:len(s)-len(suffix)], true
}

func hasPrefix(s, prefix string) bool {
	return len(s) >= len(prefix) && s[:len(prefix)] == prefix
}
