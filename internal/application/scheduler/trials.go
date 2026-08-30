package scheduler

import (
	"context"
	"fmt"

	"github.com/Lakshya5876/Raceveil/internal/application/oracle"
	"github.com/Lakshya5876/Raceveil/internal/domain"
	"github.com/Lakshya5876/Raceveil/internal/infrastructure/sync"
)

// runTrials runs the concurrent-trial loop: for each trial, re-run Setup
// (the fresh_code Reset Recipe) to obtain fresh state, prepare N Act-Request
// instances, release them via the concurrency engine, and classify the
// result (Design/ARCHITECTURE.md §2.5, §2.6). N escalates along
// concurrencyLadder if no violation appears after escalateAfter trials
// (Design/ARCHITECTURE.md §5 concurrency-level sweep), and the loop stops
// early once enough violations have been observed to already be evidence.
func runTrials(ctx context.Context, cfg RunConfig, baseURL string) ([]domain.ConcurrentTrial, error) {
	actSpec, ok := cfg.Candidate.Workflow.RequestByID(cfg.Candidate.Workflow.ActRequest)
	if !ok {
		return nil, fmt.Errorf("act_request %q not found in workflow.requests", cfg.Candidate.Workflow.ActRequest)
	}
	tc := trialContext{
		cfg:        cfg,
		baseURL:    baseURL,
		actSpec:    actSpec,
		strategy:   sync.NewH1LastByte(cfg.Guard), // Phase 1 fixture speaks HTTP/1.1
		success:    ruleFromMatcher(cfg.Candidate.SuccessWhen),
		reject:     ruleFromMatcher(cfg.Candidate.RejectWhen),
		stateIndep: stateIndependenceFor(cfg.Candidate),
	}

	var trials []domain.ConcurrentTrial
	ladderIdx := 0
	violationsSoFar := 0
	for trialNum := 1; trialNum <= maxTrials; trialNum++ {
		if trialNum > escalateAfter && violationsSoFar == 0 && ladderIdx < len(concurrencyLadder)-1 {
			ladderIdx++
		}
		trial, err := tc.runOne(ctx, trialNum, concurrencyLadder[ladderIdx])
		if err != nil {
			return nil, fmt.Errorf("trial %d: %w", trialNum, err)
		}
		if trial.Violation {
			violationsSoFar++
		}
		trials = append(trials, trial)
		if violationsSoFar >= 2 && trialNum >= 3 {
			break
		}
	}
	return trials, nil
}

// trialContext holds everything constant across the trial loop, so runOne
// only takes what varies per call (trial number, current ladder N).
type trialContext struct {
	cfg        RunConfig
	baseURL    string
	actSpec    domain.RequestSpec
	strategy   sync.SyncStrategy
	success    *oracle.Rule
	reject     *oracle.Rule
	stateIndep domain.StateIndependence
}

func (tc trialContext) runOne(ctx context.Context, trialNum, n int) (domain.ConcurrentTrial, error) {
	instances, err := prepareInstances(ctx, tc.cfg, tc.actSpec, tc.baseURL, n)
	if err != nil {
		return domain.ConcurrentTrial{}, err
	}
	responses, err := releaseBurst(ctx, tc.cfg, tc.strategy, instances)
	if err != nil {
		return domain.ConcurrentTrial{}, err
	}

	summary, s := summarizeResponses(responses, tc.success, tc.reject)
	for _, r := range responses {
		status := 0
		if r.Err == nil {
			status = r.StatusCode
		}
		auditSend(tc.cfg, tc.actSpec.Method, tc.baseURL+tc.actSpec.URL, status)
	}

	return domain.ConcurrentTrial{
		Trial:                    trialNum,
		N:                        n,
		SyncStrategy:             tc.strategy.Name(),
		InterarrivalDispersionMs: sync.Dispersion(responses),
		StateIndependence:        tc.stateIndep,
		ResponsesSummary:         summary,
		SuccessCountS:            s,
		Violation:                s > tc.cfg.Candidate.Invariant.Value,
		Observables: map[string]any{
			"success": summary.Success, "reject": summary.Reject, "error": summary.Error,
		},
	}, nil
}

func prepareInstances(ctx context.Context, cfg RunConfig, actSpec domain.RequestSpec, baseURL string, n int) ([]sync.PreparedRequest, error) {
	prior, err := runSetup(ctx, cfg, baseURL)
	if err != nil {
		return nil, fmt.Errorf("reset/setup: %w", err)
	}
	instances := make([]sync.PreparedRequest, n)
	for i := 0; i < n; i++ {
		prepared, err := prepareRequest(actSpec, baseURL, cfg.Session, prior)
		if err != nil {
			return nil, fmt.Errorf("instance %d: %w", i, err)
		}
		if err := cfg.Guard.Check(prepared.Method, prepared.URL); err != nil {
			return nil, fmt.Errorf("instance %d: %w", i, err)
		}
		instances[i] = prepared
	}
	return instances, nil
}

func releaseBurst(ctx context.Context, cfg RunConfig, strategy sync.SyncStrategy, instances []sync.PreparedRequest) ([]sync.Response, error) {
	releases := make([]func(), 0, len(instances))
	for range instances {
		release, err := cfg.Guard.AcquireSlot(ctx)
		if err != nil {
			for _, r := range releases {
				r()
			}
			return nil, fmt.Errorf("acquire concurrency slot: %w", err)
		}
		releases = append(releases, release)
	}
	defer func() {
		for _, r := range releases {
			r()
		}
	}()
	return strategy.Burst(ctx, instances)
}

func summarizeResponses(responses []sync.Response, success, reject *oracle.Rule) (domain.ResponsesSummary, int) {
	var summary domain.ResponsesSummary
	for _, r := range responses {
		if r.Err != nil {
			summary.Error++
			continue
		}
		switch oracle.Classify(oracle.ObservedResponse{StatusCode: r.StatusCode, Body: string(r.Body)}, success, reject) {
		case oracle.Success:
			summary.Success++
		case oracle.Reject:
			summary.Reject++
		default:
			summary.Error++
		}
	}
	return summary, summary.Success
}
