// Package persist writes RaceVeil's run directory to the filesystem:
// config.yaml, audit.jsonl, requests.jsonl, candidates.jsonl, per-experiment
// baseline/trials/oracle/minimization files, and findings/*.rv (Design/
// DATA_MODEL.md, Design/ARCHITECTURE.md §2.10). No database — filesystem
// only (Context/DECISIONS.md ADR-011). Implementation lands in Phase 0-1.
package persist
