package scheduler

import (
	"context"
	"fmt"
	"strings"

	"github.com/Lakshya5876/Raceveil/internal/domain"
	"github.com/Lakshya5876/Raceveil/internal/infrastructure/sync"
)

// CandidateFromFinding reconstructs the Candidate a Finding was produced
// from, so verify/replay can re-run the exact same Workflow and Invariant
// (Design/DOMAIN.md §Reproduction: a .rv is self-contained).
func CandidateFromFinding(f domain.Finding) domain.Candidate {
	return domain.Candidate{
		ID:             "cand_" + strings.TrimPrefix(f.FindingID, "find_"),
		Source:         "declared",
		Workflow:       f.Workflow,
		Invariant:      f.Invariant,
		SuccessWhen:    nonZeroMatcher(f.Oracle.SuccessSignature),
		RejectWhen:     nonZeroMatcher(f.Oracle.RejectSignature),
		PostStateProbe: f.Oracle.PostStateProbe,
		ResetRecipe:    f.ResetRecipe,
	}
}

func nonZeroMatcher(m domain.Matcher) *domain.Matcher {
	if m.Status == 0 && m.BodyContains == "" {
		return nil
	}
	return &m
}

// ReplayOnce runs the Candidate's Setup phase once and releases exactly one
// burst of n Act-Request instances, returning the raw observation without
// computing any Confidence — replay demonstrates the recorded violation, it
// does not re-verify it statistically (Design/API.md replay).
func ReplayOnce(ctx context.Context, cfg RunConfig, n int) (domain.ConcurrentTrial, error) {
	baseURL := strings.TrimSuffix(cfg.Scope.Target, "/")
	actSpec, ok := cfg.Candidate.Workflow.RequestByID(cfg.Candidate.Workflow.ActRequest)
	if !ok {
		return domain.ConcurrentTrial{}, fmt.Errorf("act_request %q not found in workflow.requests", cfg.Candidate.Workflow.ActRequest)
	}
	tc := trialContext{
		cfg:        cfg,
		baseURL:    baseURL,
		actSpec:    actSpec,
		strategy:   sync.NewH1LastByte(cfg.Guard),
		success:    ruleFromMatcher(cfg.Candidate.SuccessWhen),
		reject:     ruleFromMatcher(cfg.Candidate.RejectWhen),
		stateIndep: stateIndependenceFor(cfg.Candidate),
	}
	// A .rv records the classifier the original run calibrated, so a replay
	// judges responses by exactly the same standard (Design/DATA_MODEL.md §9).
	return tc.runOne(ctx, 1, n)
}
