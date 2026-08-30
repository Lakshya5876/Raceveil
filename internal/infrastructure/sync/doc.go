// Package sync implements RaceVeil's synchronized-concurrency engine: the
// H2SinglePacket and H1LastByte strategies that release N prepared Act-
// request instances' final bytes/frames as close to simultaneously as
// possible (Design/ARCHITECTURE.md §2.6). Prior-art infrastructure,
// reimplemented under Apache-2.0 to avoid a GPL-3 dependency (Context/
// DECISIONS.md ADR-009). Implementation lands in Phase 1 (Context/ROADMAP.md).
//
// Named "sync" to match Design/ARCHITECTURE.md's own naming; callers that
// also need the stdlib sync package import it under an alias.
package sync
