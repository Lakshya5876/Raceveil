// Package discovery groups captured Requests into Workflows via identity and
// setup-dependency heuristics (Design/ARCHITECTURE.md §2.3). This is a
// v1/Phase 3 capability — out of scope for this build (docs/PRD.md §3). The
// MVP path supplies a Candidate directly via candidate.yaml and never runs
// this stage. Package exists now only so the layering in docs/
// SYSTEM_DESIGN.md is right from day one; it is not implemented in Phase 0-2.
package discovery
