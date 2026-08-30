// Package wiring assembles RaceVeil's CLI commands with their concrete
// dependencies: the Experiment Scheduler, Oracle, Scope guard, and the
// filesystem run-directory writer (docs/SYSTEM_DESIGN.md §2). This is the
// single place that constructs concrete types and hands them to
// internal/presentation/cli as interfaces — CORE_FILES (docs/
// ARCHITECTURE_DECISIONS.md GOV-004), since editing it can silently change
// what every command actually runs against.
package wiring
