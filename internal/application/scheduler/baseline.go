package scheduler

import (
	"context"
	"fmt"

	"github.com/Lakshya5876/Raceveil/internal/application/oracle"
	"github.com/Lakshya5876/Raceveil/internal/domain"
)

// runBaseline performs baselineRuns sequential control runs
// (Design/DOMAIN.md §Baseline). Each run gets fresh state from Setup, then
// calls the Act Request Invariant.Value+1 times sequentially on that one
// fresh resource: the first L calls are expected to succeed and the last to
// be rejected, directly demonstrating the Invariant holds under purely
// sequential execution and calibrating the success/reject classifier.
//
// This is deliberately Invariant.Value+1, not required_proof_effects — they
// coincide for max_successes (both are L+1) but not for e.g.
// monotonic_limit, whose required_proof_effects is a constant 1 regardless
// of the bound; calibration still needs L successes plus one rejection to
// ever see a reject signature.
//
// When the operator declared success_when/reject_when, those are
// authoritative (Oracle Level 5). When they did not — the normal case for a
// discovered Candidate — the classifier is *learned* here from what the
// Baseline actually observed, which is what makes Levels 1-4 possible at
// all (Design/ARCHITECTURE.md §4 "Baseline calibration").
func runBaseline(ctx context.Context, cfg RunConfig, baseURL string) (domain.Baseline, error) {
	actSpec, ok := cfg.Candidate.Workflow.RequestByID(cfg.Candidate.Workflow.ActRequest)
	if !ok {
		return domain.Baseline{}, fmt.Errorf("act_request %q not found in workflow.requests", cfg.Candidate.Workflow.ActRequest)
	}
	calibrationCalls := cfg.Candidate.Invariant.Value + 1

	var successStatuses, rejectStatuses []int
	for run := 0; run < baselineRuns; run++ {
		prior, err := runSetup(ctx, cfg, baseURL)
		if err != nil {
			return domain.Baseline{}, fmt.Errorf("run %d: %w", run, err)
		}
		for call := 0; call < calibrationCalls; call++ {
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

	// Separation is what the Oracle's confidence ceiling keys on: without
	// both halves observed, the classifier cannot be trusted and Confidence
	// caps at SUSPECTED (Design/ARCHITECTURE.md §4).
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

// classifyResponse applies the operator's declared classifier when there is
// one, and otherwise falls back to the Oracle's structural reading of the
// response. The fallback is what lets a discovered Candidate — which by
// definition has no hand-written success_when/reject_when — still be
// calibrated (Design/ARCHITECTURE.md §4).
func classifyResponse(c domain.Candidate, status int, body string) oracle.Classification {
	if c.SuccessWhen == nil && c.RejectWhen == nil {
		return oracle.ClassifyStructural(status, body)
	}
	return oracle.Classify(
		oracle.ObservedResponse{StatusCode: status, Body: body},
		ruleFromMatcher(c.SuccessWhen),
		ruleFromMatcher(c.RejectWhen),
	)
}

// classifierFromBaseline turns the Baseline's calibrated signatures into
// the rules every subsequent trial classifies against. Declared matchers
// reach here through the Baseline too, so trials and the Baseline always
// judge responses by exactly the same standard.
func classifierFromBaseline(b domain.Baseline) (success, reject *oracle.Rule) {
	return ruleFromMatcher(nonZero(b.SuccessSignature)), ruleFromMatcher(nonZero(b.RejectSignature))
}

func nonZero(m domain.Matcher) *domain.Matcher {
	if m.Status == 0 && m.BodyContains == "" {
		return nil
	}
	return &m
}
