package scheduler

import (
	"context"
	"net"
	"path/filepath"
	"testing"
	"time"

	"github.com/Lakshya5876/Raceveil/internal/domain"
	"github.com/Lakshya5876/Raceveil/internal/infrastructure/persist"
	"github.com/Lakshya5876/Raceveil/internal/testutil/fixtureserver"
)

// passthroughGuard is a Guard test double that dials directly with no scope
// restriction — the scheduler's own scope.Guard integration is covered
// separately (security/scope package tests); these tests exercise the
// orchestration logic against the real fixture.
type passthroughGuard struct{}

func (passthroughGuard) Check(string, string) error { return nil }

func (passthroughGuard) Dial(ctx context.Context, network, addr string) (net.Conn, error) {
	var d net.Dialer
	return d.DialContext(ctx, network, addr)
}

func (passthroughGuard) AcquireSlot(context.Context) (func(), error) {
	return func() {}, nil
}

func redeemCandidate(issuePath, redeemPath string) domain.Candidate {
	return domain.Candidate{
		ID:     "cand_redeem",
		Source: "declared",
		Workflow: domain.Workflow{
			ID: "wf_redeem",
			Requests: []domain.RequestSpec{
				{ID: "issue_code", Method: "POST", URL: issuePath},
				{
					ID: "redeem", Method: "POST", URL: redeemPath,
					Headers: map[string]string{"content-type": "application/json"},
					Body:    `{"code":"${CODE}"}`,
					Bindings: []domain.Binding{
						{Name: "CODE", In: "body", From: "issue_code.$.code"},
					},
				},
			},
			ActRequest:    "redeem",
			SetupRequests: []string{"issue_code"},
		},
		SessionRef:  "session.json",
		Invariant:   domain.Invariant{Type: domain.InvariantMaxSuccesses, Value: 1, Source: "declared"},
		SuccessWhen: &domain.Matcher{Status: 200},
		RejectWhen:  &domain.Matcher{Status: 409},
		ResetRecipe: &domain.ResetRecipe{Kind: domain.ResetFreshCode, SetupRef: "issue_code"},
	}
}

func startFixture(t *testing.T) *fixtureserver.Server {
	t.Helper()
	srv := fixtureserver.New("127.0.0.1:0")
	if err := srv.Start(); err != nil {
		t.Fatalf("start fixture: %v", err)
	}
	t.Cleanup(func() {
		ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
		defer cancel()
		_ = srv.Stop(ctx)
	})
	return srv
}

func runConfig(t *testing.T, candidate domain.Candidate, target string) RunConfig {
	t.Helper()
	store, err := persist.NewWriter(filepath.Join(t.TempDir(), "run"))
	if err != nil {
		t.Fatalf("NewWriter: %v", err)
	}
	return RunConfig{
		Candidate: candidate,
		Scope: domain.Scope{
			Target:       target,
			AuthorizedBy: "test",
			Proof:        domain.Proof{MaxSuccessfulEffects: 2},
		},
		Session: domain.Session{IsolationGroup: "fixture"},
		Guard:   passthroughGuard{},
		Store:   store,
		RunDir:  store.RunDir(),
	}
}

func TestRun_VulnerableFixture_ProducesLikelyFinding(t *testing.T) {
	srv := startFixture(t)
	candidate := redeemCandidate("/issue-code", "/redeem")
	cfg := runConfig(t, candidate, "http://"+srv.Addr())

	outcome, err := Run(context.Background(), cfg)
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if !outcome.Found {
		t.Fatal("expected a Finding against the vulnerable check-then-act fixture")
	}
	if outcome.Oracle.Confidence != domain.Likely {
		t.Errorf("Confidence = %v, want LIKELY (Level 3 alone, no corroboration in Phase 1)", outcome.Oracle.Confidence)
	}
	if outcome.Finding == nil {
		t.Fatal("expected a non-nil Finding")
	}
	if outcome.Finding.Confidence != domain.Likely {
		t.Errorf("Finding.Confidence = %v, want LIKELY", outcome.Finding.Confidence)
	}
	if outcome.Finding.RequiredProofEffects != 2 {
		t.Errorf("RequiredProofEffects = %d, want 2", outcome.Finding.RequiredProofEffects)
	}
	if outcome.Finding.StateIndependence != domain.Independent {
		t.Errorf("StateIndependence = %v, want independent (fresh_code reset recipe)", outcome.Finding.StateIndependence)
	}
}

func TestRun_SafeTwinFixture_NoFinding(t *testing.T) {
	srv := startFixture(t)
	candidate := redeemCandidate("/issue-code-safe", "/redeem-safe")
	cfg := runConfig(t, candidate, "http://"+srv.Addr())

	outcome, err := Run(context.Background(), cfg)
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if outcome.Found {
		t.Fatalf("expected no Finding against the synchronized-safe twin, got Confidence=%v", outcome.Oracle.Confidence)
	}
	if outcome.Finding != nil {
		t.Fatal("expected a nil Finding for the safe twin")
	}
}

func TestRun_RefusesUnprovableUnderCap(t *testing.T) {
	srv := startFixture(t)
	candidate := redeemCandidate("/issue-code", "/redeem")
	candidate.Invariant.Value = 5 // required_proof_effects = 6
	cfg := runConfig(t, candidate, "http://"+srv.Addr())
	cfg.Scope.Proof.MaxSuccessfulEffects = 2

	_, err := Run(context.Background(), cfg)
	if err == nil {
		t.Fatal("expected Run to refuse an Invariant whose required_proof_effects exceeds the proof cap")
	}
}
