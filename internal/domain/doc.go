// Package domain holds RaceVeil's core entities as fixed by Design/DOMAIN.md:
// Target, Scope, Endpoint, Request, Workflow, Session, Invariant, Candidate,
// Experiment, Baseline, ConcurrentTrial, Observable, Oracle judgment types
// (Evidence, Confidence, Severity), Finding, and Reproduction.
//
// This package has zero external dependencies and imports from no other
// layer (Guide §2.2). It is pure business vocabulary — no I/O, no framework
// types. Entity implementation lands phase-by-phase per Context/ROADMAP.md;
// this file only proves the import path compiles ahead of Phase 0 work.
package domain
