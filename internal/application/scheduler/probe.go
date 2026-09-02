package scheduler

import (
	"context"
	"fmt"
	"strings"

	"github.com/Lakshya5876/Raceveil/internal/domain"
)

// ProbeObservation is one sequential Act-Request execution during Phase B.
type ProbeObservation struct {
	Status int
	Body   string
}

// ProbeWorkflow runs the cheap sequential probe behind ranking's Phase B
// invariant gate (Design/ARCHITECTURE.md §2.4): the Setup phase runs
// **once**, then the Act Request runs `attempts` times against that one
// shared state.
//
// Running setup once is the whole point. Re-running it per attempt would
// mint fresh state every time — a new coupon, a new cart — so the limit
// could never be observed and every endpoint would look unlimited. This
// mirrors the Baseline's structure (Design/DOMAIN.md §Baseline) and the Act
// Request definition (setup once, act replicated), just sequentially
// instead of concurrently. Nothing here races, so the probe can never
// itself cause the violation it is looking for.
func ProbeWorkflow(ctx context.Context, cfg RunConfig, wf domain.Workflow, attempts int) ([]ProbeObservation, error) {
	if attempts < 2 {
		attempts = 2 // a limit cannot be observed without a second execution
	}
	baseURL := strings.TrimSuffix(cfg.Scope.Target, "/")

	probeCfg := cfg
	probeCfg.Candidate.Workflow = wf

	actSpec, ok := wf.RequestByID(wf.ActRequest)
	if !ok {
		return nil, fmt.Errorf("probe: act_request %q not found in workflow.requests", wf.ActRequest)
	}
	prior, err := runSetup(ctx, probeCfg, baseURL)
	if err != nil {
		return nil, fmt.Errorf("probe setup: %w", err)
	}

	observations := make([]ProbeObservation, 0, attempts)
	for i := 0; i < attempts; i++ {
		resp, err := sendSpec(ctx, probeCfg, actSpec, baseURL, prior)
		if err != nil {
			return observations, fmt.Errorf("probe act %d: %w", i+1, err)
		}
		observations = append(observations, ProbeObservation{
			Status: resp.StatusCode, Body: string(resp.Body),
		})
	}
	return observations, nil
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

// SendOneRequest sends a single prepared RequestSpec through the Guard and
// returns its status and body. Discovery uses it to learn what a Setup
// request actually returns, so the Act Request's binding is inferred from
// observed evidence rather than guessed from an endpoint name.
func SendOneRequest(ctx context.Context, cfg RunConfig, spec domain.RequestSpec) (int, []byte, error) {
	baseURL := strings.TrimSuffix(cfg.Scope.Target, "/")
	resp, err := sendSpec(ctx, cfg, spec, baseURL, priorResponses{})
	if err != nil {
		return 0, nil, err
	}
	return resp.StatusCode, resp.Body, nil
}
