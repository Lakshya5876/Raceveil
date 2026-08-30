// Package ranking turns discovered Workflows into ranked Candidates via the
// two-phase static-signal + invariant-probe process (Design/ARCHITECTURE.md
// §2.4). This is a v1/Phase 3 capability — out of scope for this build
// (docs/PRD.md §3); the MVP path's Candidates are supplied directly and
// carry no score. Package exists now only so the layering in docs/
// SYSTEM_DESIGN.md is right from day one; it is not implemented in Phase 0-2.
package ranking
