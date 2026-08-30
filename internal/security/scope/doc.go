// Package scope implements RaceVeil's Scope/Authorization guard: the single
// chokepoint every outbound request must pass through before RaceVeil will
// send it (Design/SECURITY.md §3). It enforces, in order, the host/port/path
// allowlist (default-deny), IP pinning against DNS rebinding, private/
// loopback/metadata-IP blocking, per-scope caps, and destructive-verb rules.
//
// This is CORE_FILES (docs/ARCHITECTURE_DECISIONS.md GOV-004): a regression
// here is an SSRF/DNS-rebinding/scope-escape class bug, not an ordinary
// feature bug, so it always triggers a full-suite run on change.
//
// Implementation lands in Phase 0 (Context/ROADMAP.md); Guard below only
// fixes the package's public shape ahead of that work.
package scope

// Guard is the enforcement chokepoint every outbound request passes through.
// Its enforcement logic (allowlist, IP pinning, caps, destructive-verb
// rules) is implemented in Phase 0.
type Guard struct{}
