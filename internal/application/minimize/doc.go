// Package minimize reduces a confirmed/likely violation to its smallest
// reproducible form: a decreasing concurrency sweep with statistical
// re-verification, then delta-debugging on the request set (Design/
// ARCHITECTURE.md §6). Implementation lands in Phase 2, after reproduction
// is trustworthy (Context/ROADMAP.md — deliberately not built in Phase 1).
package minimize
