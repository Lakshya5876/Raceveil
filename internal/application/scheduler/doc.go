// Package scheduler owns the Experiment lifecycle: acquire fresh Session/
// state, run the Baseline, run the Workflow's Setup phase once, hand N
// prepared Act instances to the concurrency engine, evaluate via the Oracle,
// verify, minimize, and record everything to the run directory (Design/
// ARCHITECTURE.md §2.5). Implementation lands in Phase 1 (Context/ROADMAP.md).
package scheduler
