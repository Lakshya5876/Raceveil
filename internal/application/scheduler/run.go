package scheduler

import (
	"context"
	"errors"
	"fmt"
	"net"
	"net/url"
	"strings"
	"time"

	"github.com/Lakshya5876/Raceveil/internal/application/minimize"
	"github.com/Lakshya5876/Raceveil/internal/application/oracle"
	"github.com/Lakshya5876/Raceveil/internal/domain"
	"github.com/Lakshya5876/Raceveil/internal/infrastructure/sync"
)

// ErrUnprovableUnderCap is returned when an Invariant's required_proof_effects
// exceeds the Scope's proof.max_successful_effects safety ceiling
// (Design/DOMAIN.md §Invariant, Design/SECURITY.md §6). RaceVeil refuses the
// Experiment rather than exceeding the ceiling to force a proof.
var ErrUnprovableUnderCap = errors.New("unprovable-under-cap")

// IsUnprovable reports whether err is (or wraps) ErrUnprovableUnderCap.
// The discovery path uses it to skip one Candidate the Scope's proof
// ceiling cannot cover, rather than aborting the whole scan.
func IsUnprovable(err error) bool { return errors.Is(err, ErrUnprovableUnderCap) }

const (
	baselineRuns  = 2
	maxTrials     = 6
	escalateAfter = 3

	// warmUpTrials prime connection/JIT/cache state before any trial counts
	// as evidence, so a cold first burst is not mistaken for signal
	// (Design/ARCHITECTURE.md §5). Their results are discarded.
	warmUpTrials = 1

	// minimizeTrialsPerStep is K_min: fresh trials run at each concurrency
	// step during minimization (Design/ARCHITECTURE.md §6 ADR-014).
	minimizeTrialsPerStep = 3
)

var concurrencyLadder = []int{2, 5}

// Guard is the subset of security/scope.Guard the scheduler needs — every
// outbound request passes through Check before Dial (Design/SECURITY.md
// §3). Declared locally so scheduler never imports security/scope directly;
// wiring supplies the concrete *scope.Guard, which satisfies this
// structurally.
type Guard interface {
	Check(method, url string) error
	Dial(ctx context.Context, network, addr string) (net.Conn, error)
	AcquireSlot(ctx context.Context) (func(), error)
}

// RunConfig is everything one Experiment run needs.
type RunConfig struct {
	Candidate domain.Candidate
	Scope     domain.Scope
	Session   domain.Session
	Guard     Guard
	Store     domain.RunStore
	RunDir    string
	Seed      int64
}

// Outcome is the scheduler's result for one Experiment.
type Outcome struct {
	Oracle  domain.OracleResult
	Finding *domain.Finding
	Found   bool
}

// Run executes the full Phase 1 pipeline for one supplied Candidate:
// proof-cap check, Baseline calibration, the concurrent-trial loop (with
// concurrency-ladder escalation and the fresh_code Reset Recipe), Oracle
// evaluation, and Finding emission (Design/ARCHITECTURE.md §2.5).
func Run(ctx context.Context, cfg RunConfig) (Outcome, error) {
	baseURL := strings.TrimSuffix(cfg.Scope.Target, "/")
	required := domain.RequiredProofEffects(cfg.Candidate.Invariant)
	if required > cfg.Scope.Proof.MaxSuccessfulEffects {
		return Outcome{}, fmt.Errorf("%w: invariant requires %d successful effects but scope.proof.max_successful_effects is %d",
			ErrUnprovableUnderCap, required, cfg.Scope.Proof.MaxSuccessfulEffects)
	}
	if err := persistCandidateInputs(cfg); err != nil {
		return Outcome{}, err
	}

	baseline, err := runBaseline(ctx, cfg, baseURL)
	if err != nil {
		return Outcome{}, fmt.Errorf("baseline: %w", err)
	}
	if err := cfg.Store.PersistBaseline(cfg.Candidate.ID, baseline); err != nil {
		return Outcome{}, fmt.Errorf("persist baseline: %w", err)
	}

	trials, err := runTrials(ctx, cfg, baseURL, baseline)
	if err != nil {
		return Outcome{}, fmt.Errorf("trials: %w", err)
	}
	if err := persistTrials(cfg, trials); err != nil {
		return Outcome{}, err
	}

	result, found := evaluateExperiment(cfg, required, baseline, trials)
	if err := cfg.Store.PersistOracle(cfg.Candidate.ID, result); err != nil {
		return Outcome{}, fmt.Errorf("persist oracle result: %w", err)
	}
	outcome := Outcome{Oracle: result, Found: found}
	if !found {
		return outcome, nil
	}

	finding, err := finalizeFinding(ctx, cfg, baseURL, baseline, result, trials)
	if err != nil {
		return Outcome{}, err
	}
	outcome.Finding = &finding
	return outcome, nil
}

func evaluateExperiment(cfg RunConfig, required int, baseline domain.Baseline, trials []domain.ConcurrentTrial) (domain.OracleResult, bool) {
	corroboration := corroborationFromTrials(trials, cfg.Candidate.Invariant.Value)
	return oracle.Evaluate(oracle.EvaluateInput{
		CandidateID:              cfg.Candidate.ID,
		Invariant:                cfg.Candidate.Invariant,
		RequiredProofEffects:     required,
		ClassifierSeparation:     baseline.ClassifierSeparation,
		Trials:                   trials,
		CorroboratingObservables: corroboration,
	})
}

// finalizeFinding runs minimization and packages the Finding — split out of
// Run so the top-level pipeline stays a flat, readable sequence.
func finalizeFinding(ctx context.Context, cfg RunConfig, baseURL string, baseline domain.Baseline, result domain.OracleResult, trials []domain.ConcurrentTrial) (domain.Finding, error) {
	minimization, err := minimizeExperiment(ctx, cfg, baseURL, baseline, trials)
	if err != nil {
		return domain.Finding{}, fmt.Errorf("minimize: %w", err)
	}
	if err := cfg.Store.PersistMinimization(cfg.Candidate.ID, minimization); err != nil {
		return domain.Finding{}, fmt.Errorf("persist minimization: %w", err)
	}
	finding := buildFinding(cfg, baseline, result, trials, minimization)
	if err := cfg.Store.PersistFinding(finding); err != nil {
		return domain.Finding{}, fmt.Errorf("persist finding: %w", err)
	}
	return finding, nil
}

// minimizeExperiment reduces a Finding's trigger to its smallest
// reproducible form: the decreasing concurrency sweep, then greedy Setup-
// request dropping (Design/ARCHITECTURE.md §6). Only runs once a violation
// is already established (Run calls it after Evaluate returns found=true).
func minimizeExperiment(ctx context.Context, cfg RunConfig, baseURL string, baseline domain.Baseline, trials []domain.ConcurrentTrial) (domain.Minimization, error) {
	startN := 0
	for _, t := range trials {
		if t.N > startN {
			startN = t.N
		}
	}
	actSpec, ok := cfg.Candidate.Workflow.RequestByID(cfg.Candidate.Workflow.ActRequest)
	if !ok {
		return domain.Minimization{}, fmt.Errorf("act_request %q not found in workflow.requests", cfg.Candidate.Workflow.ActRequest)
	}
	success, reject := classifierFromBaseline(baseline)
	tc := trialContext{
		cfg: cfg, baseURL: baseURL, actSpec: actSpec,
		strategy:   sync.NewH1LastByte(cfg.Guard),
		success:    success,
		reject:     reject,
		stateIndep: stateIndependenceFor(cfg.Candidate),
	}

	concurrency, err := minimize.SweepConcurrency(ctx, func(ctx context.Context, n int) (bool, error) {
		trial, err := tc.runOne(ctx, 0, n)
		return trial.Violation, err
	}, startN, minimizeTrialsPerStep)
	if err != nil {
		return domain.Minimization{}, err
	}

	workflow, err := minimize.MinimizeWorkflow(ctx, func(ctx context.Context, setupRequests []string) (bool, error) {
		reduced := cfg
		reduced.Candidate.Workflow.SetupRequests = setupRequests
		rc := trialContext{
			cfg: reduced, baseURL: baseURL, actSpec: actSpec,
			strategy:   sync.NewH1LastByte(reduced.Guard),
			success:    success,
			reject:     reject,
			stateIndep: stateIndependenceFor(reduced.Candidate),
		}
		trial, err := rc.runOne(ctx, 0, concurrency.MinimalN)
		if err != nil {
			// A dropped Setup request can break a binding the Act request
			// still needs (e.g. no fresh code to redeem) — that failure
			// itself proves the request is required, not a fatal error.
			return false, nil
		}
		return trial.Violation, nil
	}, cfg.Candidate.Workflow.SetupRequests)
	if err != nil {
		return domain.Minimization{}, err
	}

	return domain.Minimization{
		Concurrency: concurrency,
		Workflow:    workflow,
		Method:      "decreasing-sweep + delta-debugging with re-verification",
	}, nil
}

func persistCandidateInputs(cfg RunConfig) error {
	if err := cfg.Store.PersistRequests(capturedRequests(cfg.Candidate)); err != nil {
		return fmt.Errorf("persist requests: %w", err)
	}
	if err := cfg.Store.PersistCandidate(cfg.Candidate); err != nil {
		return fmt.Errorf("persist candidate: %w", err)
	}
	return nil
}

func persistTrials(cfg RunConfig, trials []domain.ConcurrentTrial) error {
	for _, tr := range trials {
		if err := cfg.Store.PersistTrial(cfg.Candidate.ID, tr); err != nil {
			return fmt.Errorf("persist trial %d: %w", tr.Trial, err)
		}
	}
	return nil
}

func capturedRequests(c domain.Candidate) []domain.CapturedRequest {
	out := make([]domain.CapturedRequest, len(c.Workflow.Requests))
	for i, r := range c.Workflow.Requests {
		out[i] = domain.CapturedRequest{
			ID:       r.ID,
			Endpoint: domain.Endpoint{Method: r.Method, Path: r.URL},
			URL:      r.URL,
			Headers:  r.Headers,
			Body:     r.Body,
			Bindings: r.Bindings,
			Source:   "declared",
		}
	}
	return out
}

// runSetup executes every Setup Request once (Design/DOMAIN.md §Act
// Request), returning the map of prior responses later requests' bindings
// resolve against.
func runSetup(ctx context.Context, cfg RunConfig, baseURL string) (priorResponses, error) {
	prior := make(priorResponses)
	for _, id := range cfg.Candidate.Workflow.SetupRequests {
		spec, ok := cfg.Candidate.Workflow.RequestByID(id)
		if !ok {
			return nil, fmt.Errorf("setup request %q not found in workflow.requests", id)
		}
		resp, err := sendSpec(ctx, cfg, spec, baseURL, prior)
		if err != nil {
			return nil, fmt.Errorf("setup %q: %w", id, err)
		}
		if resp.StatusCode < 200 || resp.StatusCode >= 300 {
			return nil, fmt.Errorf("setup %q: unexpected status %d", id, resp.StatusCode)
		}
		prior[id] = resp.Body
	}
	return prior, nil
}

// sendSpec prepares, scope-checks, sends, and audits one RequestSpec.
func sendSpec(ctx context.Context, cfg RunConfig, spec domain.RequestSpec, baseURL string, prior priorResponses) (sync.Response, error) {
	prepared, err := prepareRequest(spec, baseURL, cfg.Session, prior)
	if err != nil {
		return sync.Response{}, err
	}
	if err := cfg.Guard.Check(prepared.Method, prepared.URL); err != nil {
		return sync.Response{}, err
	}
	resp, err := sync.SendOne(ctx, cfg.Guard, prepared)
	if err != nil {
		return sync.Response{}, err
	}
	if resp.Err != nil {
		return sync.Response{}, resp.Err
	}
	auditSend(cfg, prepared.Method, prepared.URL, resp.StatusCode)
	return resp, nil
}

func auditSend(cfg RunConfig, method, rawURL string, status int) {
	host, path := hostAndPath(rawURL)
	_ = cfg.Store.PersistAudit(domain.AuditEntry{
		Timestamp:    time.Now(),
		Method:       method,
		Host:         host,
		Path:         path,
		SessionGroup: cfg.Session.IsolationGroup,
		ExperimentID: cfg.Candidate.ID,
		Status:       status,
	})
}

func hostAndPath(rawURL string) (string, string) {
	u, err := url.Parse(rawURL)
	if err != nil {
		return "", rawURL
	}
	return u.Host, u.Path
}

func ruleFromMatcher(m *domain.Matcher) *oracle.Rule {
	if m == nil {
		return nil
	}
	return &oracle.Rule{Status: m.Status, BodyContains: m.BodyContains}
}

func matcherOrInferred(declared *domain.Matcher, observedStatuses []int) domain.Matcher {
	if declared != nil {
		return *declared
	}
	if len(observedStatuses) == 0 {
		return domain.Matcher{}
	}
	return domain.Matcher{Status: observedStatuses[0]}
}

func resetDescription(r *domain.ResetRecipe) string {
	if r == nil {
		return "unknown"
	}
	return string(r.Kind)
}

// stateIndependenceFor reports Independent only when the Candidate declares
// an executable Reset Recipe of a Phase-1-supported kind (Design/DOMAIN.md
// §State Independence: independence must be established, never assumed).
func stateIndependenceFor(c domain.Candidate) domain.StateIndependence {
	if c.ResetRecipe == nil {
		return domain.UnknownIndependence
	}
	switch c.ResetRecipe.Kind {
	case domain.ResetFreshCode, domain.ResetSetupRecipe:
		return domain.Independent
	default:
		return domain.UnknownIndependence
	}
}
