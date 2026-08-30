// Package oracle implements RaceVeil's Observable-State Oracle: the leveled
// judgment subsystem (Levels 1-5, Design/ARCHITECTURE.md §4) that decides,
// from a Baseline plus ConcurrentTrials, whether a concurrency-induced
// Invariant violation occurred, and the Statistical Verification that turns
// per-trial observations into a Wilson-interval Confidence band (Design/
// ARCHITECTURE.md §5). Levels 1+3 land in Phase 1; Levels 2+4 and the
// corroboration rule land in Phase 2 (Context/ROADMAP.md).
package oracle
