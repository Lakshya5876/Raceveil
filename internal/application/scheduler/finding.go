package scheduler

import (
	"strings"
	"time"

	"github.com/Lakshya5876/Raceveil/internal/domain"
)

// buildFinding packages a confirmed-or-likely violation as a self-contained
// Reproduction bundle (Design/DOMAIN.md §Reproduction, Design/DATA_MODEL.md
// §9 findings/<id>.rv). Only called when Evaluate reported found=true.
func buildFinding(cfg RunConfig, result domain.OracleResult, trials []domain.ConcurrentTrial) domain.Finding {
	minimalN, maxS, syncStrategy := summarizeTrialsForFinding(trials)

	return domain.Finding{
		RVVersion:            "1",
		FindingID:            "find_" + strings.TrimPrefix(cfg.Candidate.ID, "cand_"),
		Target:               cfg.Scope.Target,
		ScopeRef:             domain.ScopeRef{AuthorizedBy: cfg.Scope.AuthorizedBy},
		Workflow:             cfg.Candidate.Workflow,
		SessionBootstrap:     domain.SessionBootstrap{IsolationGroup: cfg.Session.IsolationGroup},
		Invariant:            cfg.Candidate.Invariant,
		RequiredProofEffects: result.RequiredProofEffects,
		SyncStrategy:         syncStrategy,
		ConcurrencyN:         minimalN,
		Oracle: domain.FindingOracleSummary{
			PrimaryLevels:       result.PrimaryOracleLevels,
			CorroboratingLevels: nil,
			SuccessSignature:    matcherOrInferred(cfg.Candidate.SuccessWhen, nil),
			RejectSignature:     matcherOrInferred(cfg.Candidate.RejectWhen, nil),
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
