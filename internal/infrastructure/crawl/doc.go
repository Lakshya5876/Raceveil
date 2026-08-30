// Package crawl implements RaceVeil's authenticated crawler and HAR/proxy
// traffic import (Design/ARCHITECTURE.md §2.2) — request-capture
// infrastructure for the v1/Phase 3 automatic-discovery path. Out of scope
// for this build (docs/PRD.md §3); the MVP path supplies a Candidate
// directly and never runs this stage. No headless browser (Context/
// DECISIONS.md ADR-012). Package exists now only so the layering in docs/
// SYSTEM_DESIGN.md is right from day one; it is not implemented in Phase 0-2.
package crawl
