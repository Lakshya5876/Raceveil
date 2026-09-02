package minimize

import (
	"context"
	"fmt"

	"github.com/Lakshya5876/Raceveil/internal/domain"
)

// WorkflowCheckFunc re-verifies whether the Workflow still reproduces the
// violation with the given Setup request set (a subset of the original).
// The scheduler's implementation decides how many fresh trials to run
// internally; this package only needs the boolean verdict.
type WorkflowCheckFunc func(ctx context.Context, setupRequests []string) (violated bool, err error)

// MinimizeWorkflow greedily drops Setup requests one at a time — delta-
// debugging style — re-verifying after each removal, and keeps the removal
// only if the violation still reproduces without it
// (Design/ARCHITECTURE.md §6).
func MinimizeWorkflow(ctx context.Context, check WorkflowCheckFunc, startRequests []string) (domain.WorkflowMinimization, error) {
	minimal := append([]string(nil), startRequests...)
	for i := 0; i < len(minimal); {
		candidate := dropAt(minimal, i)
		violated, err := check(ctx, candidate)
		if err != nil {
			return domain.WorkflowMinimization{}, fmt.Errorf("minimize: workflow check without %q: %w", minimal[i], err)
		}
		if violated {
			minimal = candidate // still reproduces without this request — drop it and re-check the same index
			continue
		}
		i++ // this request was required; keep it
	}
	return domain.WorkflowMinimization{StartRequests: startRequests, MinimalRequests: minimal}, nil
}

func dropAt(s []string, i int) []string {
	out := make([]string, 0, len(s)-1)
	out = append(out, s[:i]...)
	out = append(out, s[i+1:]...)
	return out
}
