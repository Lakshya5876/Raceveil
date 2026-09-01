// Package minimize reduces a confirmed/likely violation to its smallest
// reproducible form (Design/ARCHITECTURE.md §6, ADR-014): a decreasing
// concurrency sweep with statistical re-verification at each step (not
// binary bisection — reproduction is probabilistic, so there is no crisp
// monotone threshold to bisect), plus greedy Workflow/request-set
// minimization. Runs only once reproduction is already trustworthy
// (Context/ROADMAP.md Phase 2), after the main trial loop has established a
// Finding.
package minimize

import (
	"context"
	"fmt"

	"github.com/Lakshya5876/Raceveil/internal/application/oracle"
	"github.com/Lakshya5876/Raceveil/internal/domain"
)

// TrialFunc runs one fresh trial at concurrency n and reports whether it
// violated the Invariant. The scheduler supplies the concrete
// implementation (a real Setup + burst + classify); this package has no
// I/O or infrastructure dependency of its own, so it stays independently
// testable with a fake.
type TrialFunc func(ctx context.Context, n int) (violated bool, err error)

// SweepConcurrency performs the decreasing-sweep concurrency minimization:
// starting at startN, step N down one at a time, run kMin fresh trials at
// each step, and keep the smallest N that still reproduces at least once.
// Stops at the first N with zero reproductions — Design/ARCHITECTURE.md's
// "decreasing sweep with statistical re-verification", bounded so a large
// startN doesn't force running kMin trials all the way down to 2 once the
// race has clearly stopped triggering.
func SweepConcurrency(ctx context.Context, run TrialFunc, startN, kMin int) (domain.ConcurrencyMinimization, error) {
	result := domain.ConcurrencyMinimization{StartN: startN, MinimalN: startN}
	if startN < 2 {
		return result, fmt.Errorf("minimize: startN must be >= 2, got %d", startN)
	}

	bestN := startN
	var bestRepro domain.MinReproStats
	for n := startN; n >= 2; n-- {
		r, k, err := runK(ctx, run, n, kMin)
		if err != nil {
			return result, fmt.Errorf("minimize: sweep at N=%d: %w", n, err)
		}
		if r == 0 {
			break
		}
		lo, _ := oracle.WilsonInterval(r, k, oracle.Z95)
		bestN = n
		bestRepro = domain.MinReproStats{RViolations: r, KIndependent: k, WilsonLo: lo}
	}
	result.MinimalN = bestN
	result.MinRepro = bestRepro
	return result, nil
}

func runK(ctx context.Context, run TrialFunc, n, k int) (r, kActual int, err error) {
	for i := 0; i < k; i++ {
		violated, err := run(ctx, n)
		if err != nil {
			return r, kActual, err
		}
		kActual++
		if violated {
			r++
		}
	}
	return r, kActual, nil
}
