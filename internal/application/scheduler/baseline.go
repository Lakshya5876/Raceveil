package scheduler

import (
	"context"
	"fmt"

	"github.com/Lakshya5876/Raceveil/internal/application/oracle"
	"github.com/Lakshya5876/Raceveil/internal/domain"
)

// runBaseline performs baselineRuns sequential control runs
// (Design/DOMAIN.md §Baseline). Each run gets fresh state from Setup, then
// calls the Act Request `required` (= L+1) times sequentially on that one
// fresh resource: the first L calls are expected to succeed and the last to
// be rejected, directly demonstrating the Invariant holds under purely
// sequential execution and calibrating the success/reject classifier.
func runBaseline(ctx context.Context, cfg RunConfig, baseURL string, required int) (domain.Baseline, error) {
	actSpec, ok := cfg.Candidate.Workflow.RequestByID(cfg.Candidate.Workflow.ActRequest)
	if !ok {
		return domain.Baseline{}, fmt.Errorf("act_request %q not found in workflow.requests", cfg.Candidate.Workflow.ActRequest)
	}

	var successStatuses, rejectStatuses []int
	for run := 0; run < baselineRuns; run++ {
		prior, err := runSetup(ctx, cfg, baseURL)
		if err != nil {
			return domain.Baseline{}, fmt.Errorf("run %d: %w", run, err)
		}
		for call := 0; call < required; call++ {
			resp, err := sendSpec(ctx, cfg, actSpec, baseURL, prior)
			if err != nil {
				return domain.Baseline{}, fmt.Errorf("run %d call %d: %w", run, call, err)
			}
			switch classifyResponse(cfg.Candidate, resp.StatusCode, string(resp.Body)) {
			case oracle.Success:
				successStatuses = append(successStatuses, resp.StatusCode)
			case oracle.Reject:
				rejectStatuses = append(rejectStatuses, resp.StatusCode)
			}
		}
	}

	separation := "clean"
	if len(successStatuses) == 0 || len(rejectStatuses) == 0 {
		separation = "weak"
	}

	return domain.Baseline{
		CandidateID:          cfg.Candidate.ID,
		Runs:                 baselineRuns,
		ExpectedSuccessCount: cfg.Candidate.Invariant.Value,
		SuccessSignature:     matcherOrInferred(cfg.Candidate.SuccessWhen, successStatuses),
		RejectSignature:      matcherOrInferred(cfg.Candidate.RejectWhen, rejectStatuses),
		ClassifierSeparation: separation,
		BaselineStateReset:   resetDescription(cfg.Candidate.ResetRecipe),
		ResetRecipe:          cfg.Candidate.ResetRecipe,
	}, nil
}

func classifyResponse(c domain.Candidate, status int, body string) oracle.Classification {
	return oracle.Classify(
		oracle.ObservedResponse{StatusCode: status, Body: body},
		ruleFromMatcher(c.SuccessWhen),
		ruleFromMatcher(c.RejectWhen),
	)
}
