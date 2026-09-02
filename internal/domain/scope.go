// Package domain holds RaceVeil's core entities as fixed by Design/DOMAIN.md:
// Target, Scope, Endpoint, Request, Workflow, Session, Invariant, Candidate,
// Experiment, Baseline, ConcurrentTrial, Observable, Oracle judgment types
// (Evidence, Confidence, Severity), Finding, and Reproduction, plus the
// RunStore/FindingReader persistence interfaces infrastructure implements.
//
// This package has zero external dependencies and imports from no other
// layer (Guide §2.2) — pure business vocabulary, no I/O, no framework types.
package domain

// HostRule is one authorized host in a Scope's default-deny allowlist
// (Design/DATA_MODEL.md scope.yaml).
type HostRule struct {
	Host         string   `yaml:"host" json:"host"`
	Ports        []int    `yaml:"ports" json:"ports"`
	PinIP        string   `yaml:"pin_ip" json:"pin_ip"`
	PathPrefixes []string `yaml:"path_prefixes" json:"path_prefixes"`
}

// Caps are the per-scope resource ceilings enforced by the Guard and
// scheduler (Design/SECURITY.md §8).
type Caps struct {
	MaxConcurrency   int `yaml:"max_concurrency" json:"max_concurrency"`
	MaxRequestsTotal int `yaml:"max_requests_total" json:"max_requests_total"`
	MaxRatePerSec    int `yaml:"max_rate_per_sec" json:"max_rate_per_sec"`
	MaxWallclockSec  int `yaml:"max_wallclock_sec" json:"max_wallclock_sec"`
}

// Destructive controls which verbs/paths are blocked by default
// (Design/SECURITY.md §6).
type Destructive struct {
	AllowVerbs []string `yaml:"allow_verbs" json:"allow_verbs"`
	DenyPaths  []string `yaml:"deny_paths" json:"deny_paths"`
}

// Proof carries the operator's safety ceiling on successful state-changing
// effects RaceVeil may cause while proving a violation (Design/SECURITY.md §6).
type Proof struct {
	MaxSuccessfulEffects int `yaml:"max_successful_effects" json:"max_successful_effects"`
}

// Scope is RaceVeil's explicit authorization boundary (Design/DOMAIN.md
// §Scope): a default-deny allowlist of hosts/ports/path-prefixes, plus caps
// and destructive-action rules. Every outbound request is checked against a
// Scope by security/scope.Guard.
type Scope struct {
	Target             string      `yaml:"target" json:"target"`
	AuthorizedBy       string      `yaml:"authorized_by" json:"authorized_by"`
	Hosts              []HostRule  `yaml:"hosts" json:"hosts"`
	AllowPrivateTarget bool        `yaml:"allow_private_target" json:"allow_private_target"`
	Caps               Caps        `yaml:"caps" json:"caps"`
	Destructive        Destructive `yaml:"destructive" json:"destructive"`
	Proof              Proof       `yaml:"proof" json:"proof"`
}
