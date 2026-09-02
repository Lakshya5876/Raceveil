package scheduler

import (
	"context"
	"fmt"
	"strings"

	"github.com/Lakshya5876/Raceveil/internal/domain"
)

// ProbeWorkflow executes one Workflow sequentially — Setup phase once, then
// the Act Request once — and returns the Act Request's response. This is
// the cheap traffic behind ranking's Phase B invariant probe
// (Design/ARCHITECTURE.md §2.4): a handful of sequential executions used to
// decide whether a limit exists at all, before any concurrent burst is
// considered.
//
// It is deliberately sequential and single-shot: nothing here races, so it
// can never itself cause the violation it is looking for.
func ProbeWorkflow(ctx context.Context, cfg RunConfig, wf domain.Workflow) (int, string, error) {
	baseURL := strings.TrimSuffix(cfg.Scope.Target, "/")

	probeCfg := cfg
	probeCfg.Candidate.Workflow = wf

	prior, err := runSetup(ctx, probeCfg, baseURL)
	if err != nil {
		return 0, "", fmt.Errorf("probe setup: %w", err)
	}
	actSpec, ok := wf.RequestByID(wf.ActRequest)
	if !ok {
		return 0, "", fmt.Errorf("probe: act_request %q not found in workflow.requests", wf.ActRequest)
	}
	resp, err := sendSpec(ctx, probeCfg, actSpec, baseURL, prior)
	if err != nil {
		return 0, "", fmt.Errorf("probe act: %w", err)
	}
	return resp.StatusCode, string(resp.Body), nil
}

// ProbeEndpoint issues one read-only GET through the Guard — used by
// `raceveil auth check` to confirm a Session is live without running an
// Experiment. It sends exactly one request and changes no state.
func ProbeEndpoint(ctx context.Context, guard Guard, session domain.Session, rawURL string) (int, string, error) {
	cfg := RunConfig{
		Scope:   domain.Scope{Target: rawURL},
		Session: session,
		Guard:   guard,
		Store:   discardStore{},
	}
	spec := domain.RequestSpec{ID: "auth_probe", Method: "GET", URL: ""}
	resp, err := sendSpec(ctx, cfg, spec, rawURL, priorResponses{})
	if err != nil {
		return 0, "", err
	}
	return resp.StatusCode, string(resp.Body), nil
}

// discardStore satisfies domain.RunStore for probes that must not write a
// run directory: `auth check` is a liveness check, not an Experiment.
type discardStore struct{}

func (discardStore) PersistRequests([]domain.CapturedRequest) error        { return nil }
func (discardStore) PersistCandidate(domain.Candidate) error               { return nil }
func (discardStore) PersistBaseline(string, domain.Baseline) error         { return nil }
func (discardStore) PersistTrial(string, domain.ConcurrentTrial) error     { return nil }
func (discardStore) PersistOracle(string, domain.OracleResult) error       { return nil }
func (discardStore) PersistMinimization(string, domain.Minimization) error { return nil }
func (discardStore) PersistFinding(domain.Finding) error                   { return nil }
func (discardStore) PersistAudit(domain.AuditEntry) error                  { return nil }
