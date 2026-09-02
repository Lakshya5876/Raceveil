package scheduler

import (
	"strings"
	"time"

	"github.com/Lakshya5876/Raceveil/internal/domain"
)

// buildFinding packages a confirmed-or-likely violation as a self-contained
// Reproduction bundle (Design/DOMAIN.md §Reproduction, Design/DATA_MODEL.md
// §9 findings/<id>.rv). Only called when Evaluate reported found=true.
// ConcurrencyN and the Workflow reflect the minimized reproducer, not the
// starting N/request set (Design/ARCHITECTURE.md §6: minimization exists so
// the .rv hands a human the smallest useful reproduction).
func buildFinding(cfg RunConfig, baseline domain.Baseline, result domain.OracleResult, trials []domain.ConcurrentTrial, m domain.Minimization) domain.Finding {
	_, maxS, syncStrategy := summarizeTrialsForFinding(trials)

	workflow := cfg.Candidate.Workflow
	workflow.SetupRequests = m.Workflow.MinimalRequests

	return domain.Finding{
		RVVersion:            "1",
		FindingID:            "find_" + strings.TrimPrefix(cfg.Candidate.ID, "cand_"),
		Target:               cfg.Scope.Target,
		ScopeRef:             domain.ScopeRef{AuthorizedBy: cfg.Scope.AuthorizedBy},
		Workflow:             workflow,
		SessionBootstrap:     domain.SessionBootstrap{IsolationGroup: cfg.Session.IsolationGroup},
		Invariant:            cfg.Candidate.Invariant,
		RequiredProofEffects: result.RequiredProofEffects,
		SyncStrategy:         syncStrategy,
		ConcurrencyN:         m.Concurrency.MinimalN,
		Oracle: domain.FindingOracleSummary{
			PrimaryLevels:       result.PrimaryOracleLevels,
			CorroboratingLevels: corroboratingLevels(result.CorroboratingObservables),
			SuccessSignature:    baseline.SuccessSignature,
			RejectSignature:     baseline.RejectSignature,
			PostStateProbe:      cfg.Candidate.PostStateProbe,
		},
		Expected:          domain.ExpectedInvariant{MaxSuccesses: cfg.Candidate.Invariant.Value},
		StateIndependence: result.StateIndependence,
		ResetRecipe:       cfg.Candidate.ResetRecipe,
		ObservedAtCreation: domain.ObservedAtCreation{
			S:                      maxS,
			Reproduction:           result.Reproduction,
			ConfidenceBandsVersion: result.ConfidenceBandsVersion,
		},
		Confidence:   result.Confidence,
		Severity:     result.Severity,
		EvidenceRefs: domain.EvidenceRefs{RunDir: cfg.RunDir},
		Created:      time.Now().UTC(),
		RNGSeed:      cfg.Seed,
	}
}

// corroboratingLevels maps corroborating-observable names to their Oracle
// level numbers (Design/ARCHITECTURE.md §4: Level 2 body-differential,
// Level 4 post-state).
func corroboratingLevels(observables []string) []int {
	var levels []int
	for _, o := range observables {
		switch o {
		case "body_differential":
			levels = append(levels, 2)
		case "post_state":
			levels = append(levels, 4)
		}
	}
	return levels
}

// summarizeTrialsForFinding picks the smallest N among violating trials (the
// most useful reproduction burst size), the largest observed success count,
// and the sync strategy name used.
func summarizeTrialsForFinding(trials []domain.ConcurrentTrial) (minimalN, maxS int, syncStrategy string) {
	for _, t := range trials {
		if syncStrategy == "" {
			syncStrategy = t.SyncStrategy
		}
		if t.SuccessCountS > maxS {
			maxS = t.SuccessCountS
		}
		if t.Violation && (minimalN == 0 || t.N < minimalN) {
			minimalN = t.N
		}
	}
	return minimalN, maxS, syncStrategy
}
