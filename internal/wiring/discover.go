package wiring

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"gopkg.in/yaml.v3"

	"github.com/Lakshya5876/Raceveil/internal/application/discovery"
	"github.com/Lakshya5876/Raceveil/internal/application/ranking"
	"github.com/Lakshya5876/Raceveil/internal/application/scheduler"
	"github.com/Lakshya5876/Raceveil/internal/domain"
	"github.com/Lakshya5876/Raceveil/internal/infrastructure/crawl"
	"github.com/Lakshya5876/Raceveil/internal/security/scope"
)

// DiscoverOptions are the resolved `raceveil scan <url>` flags — the v1
// discovery path (Design/API.md).
type DiscoverOptions struct {
	Target        string
	ScopePath     string
	AuthPath      string
	OutDir        string
	ImportPath    string // HAR or OpenAPI document
	InvariantsRef string // optional invariants.yaml (Oracle Level 5)
	Mode          string // crawl | import | both
	MaxCandidates int
	ProbeAttempts int
	SafeMode      bool
	IAmAuthorized bool
}

func (o DiscoverOptions) withDefaults() DiscoverOptions {
	if o.Mode == "" {
		o.Mode = "crawl"
	}
	if o.MaxCandidates <= 0 {
		o.MaxCandidates = 10
	}
	if o.ProbeAttempts <= 0 {
		o.ProbeAttempts = 3
	}
	return o
}

// DiscoverySummary reports what discovery saw, so the operator can watch
// the false-positive gate working rather than just the findings that
// survived it (Design/UX.md progress display).
type DiscoverySummary struct {
	Endpoints        int          `json:"endpoints"`
	Workflows        int          `json:"workflows"`
	Ranked           int          `json:"ranked"`
	Probed           int          `json:"probed"`
	CandidatesTested int          `json:"candidates_tested"`
	SkippedNoLimit   []string     `json:"skipped_no_invariant"`
	Findings         []ScanResult `json:"findings"`
	RunDir           string       `json:"run_dir"`
}

// HighestConfidence returns the strongest Confidence across the run's
// findings, or "" when nothing was found.
func (s DiscoverySummary) HighestConfidence() domain.Confidence {
	best := domain.Confidence("")
	for _, f := range s.Findings {
		switch f.Oracle.Confidence {
		case domain.Confirmed:
			return domain.Confirmed
		case domain.Likely:
			best = domain.Likely
		case domain.Suspected:
			if best == "" {
				best = domain.Suspected
			}
		}
	}
	return best
}

// RunDiscoverScan is the v1 `scan <url>` path: capture requests (crawl
// and/or import), extract Workflows, rank them, run the Phase B
// invariant-inference gate, and run a full Experiment on each surviving
// Candidate (Design/ARCHITECTURE.md §2.2-§2.5).
func RunDiscoverScan(ctx context.Context, opts DiscoverOptions) (DiscoverySummary, error) {
	opts = opts.withDefaults()
	summary := DiscoverySummary{}

	sc, session, declared, err := loadDiscoveryInputs(opts)
	if err != nil {
		return summary, err
	}
	guard, store, err := newGuardAndStore(sc, opts.OutDir, "raceveil-run", authorization{opts.SafeMode, opts.IAmAuthorized})
	if err != nil {
		return summary, err
	}
	summary.RunDir = store.RunDir()

	captured, err := captureRequests(ctx, opts, sc, session, guard)
	if err != nil {
		return summary, err
	}
	if err := store.PersistRequests(captured); err != nil {
		return summary, fmt.Errorf("persist requests: %w", err)
	}
	summary.Endpoints = len(captured)

	workflows := discovery.ExtractWorkflows(captured)
	summary.Workflows = len(workflows)

	ranked := ranking.RankWorkflows(workflows, captured)
	summary.Ranked = len(ranked)
	if len(ranked) > opts.MaxCandidates {
		ranked = ranked[:opts.MaxCandidates]
	}

	base := scheduler.RunConfig{
		Scope: sc, Session: session, Guard: guard, Store: store, RunDir: store.RunDir(),
	}

	for _, r := range ranked {
		summary.Probed++
		if err := runOneDiscoveredCandidate(ctx, base, store, r, captured, declared, opts.ProbeAttempts, &summary); err != nil {
			return summary, err
		}
	}
	return summary, nil
}

// runOneDiscoveredCandidate takes one ranked Workflow through the
// invariant gate and, if it survives, a full Experiment. It returns an
// error only for failures that should abort the whole scan; a Workflow
// that simply has no invariant, or that the proof cap cannot cover, is
// recorded as skipped and the scan continues.
func runOneDiscoveredCandidate(
	ctx context.Context,
	base scheduler.RunConfig,
	store candidateStore,
	r ranking.Ranked,
	captured []domain.CapturedRequest,
	declared []domain.UserInvariant,
	probeAttempts int,
	summary *DiscoverySummary,
) error {
	candidate, ok, err := establishCandidate(ctx, base, r, captured, declared, probeAttempts)
	if err != nil {
		// A probe that cannot run (unreachable endpoint, refused by the
		// Guard) disqualifies that Workflow, not the whole scan.
		summary.skip(r.Workflow, "probe failed: "+err.Error())
		return nil
	}
	if !ok {
		summary.skip(r.Workflow, "no invariant established")
		return nil
	}

	if err := store.PersistCandidate(candidate); err != nil {
		return fmt.Errorf("persist candidate: %w", err)
	}
	summary.CandidatesTested++

	cfg := base
	cfg.Candidate = candidate
	outcome, err := scheduler.Run(ctx, cfg)
	if err != nil {
		if scheduler.IsUnprovable(err) {
			summary.skip(r.Workflow, "unprovable under the scope's proof cap")
			return nil
		}
		return fmt.Errorf("experiment %s: %w", candidate.ID, err)
	}
	if outcome.Found {
		summary.Findings = append(summary.Findings, scanResultFrom(outcome))
	}
	return nil
}

// candidateStore is the narrow slice of the run store the discovery loop
// writes through.
type candidateStore interface {
	PersistCandidate(domain.Candidate) error
}

func (s *DiscoverySummary) skip(wf domain.Workflow, reason string) {
	s.SkippedNoLimit = append(s.SkippedNoLimit, describeWorkflow(wf)+" ("+reason+")")
}

// loadDiscoveryInputs reads the scope, session, and optional Level 5
// invariant declarations a discovery scan runs against.
func loadDiscoveryInputs(opts DiscoverOptions) (domain.Scope, domain.Session, []domain.UserInvariant, error) {
	sc, err := scheduler.LoadScope(opts.ScopePath)
	if err != nil {
		return domain.Scope{}, domain.Session{}, nil, err
	}
	if opts.Target != "" {
		sc.Target = strings.TrimSuffix(opts.Target, "/")
	}
	session, err := scheduler.LoadSession(opts.AuthPath)
	if err != nil {
		return domain.Scope{}, domain.Session{}, nil, err
	}
	declared, err := loadUserInvariants(opts.InvariantsRef)
	if err != nil {
		return domain.Scope{}, domain.Session{}, nil, err
	}
	return sc, session, declared, nil
}

// captureRequests runs the configured capture sources. Both may run; the
// results are merged and de-duplicated by endpoint.
func captureRequests(ctx context.Context, opts DiscoverOptions, sc domain.Scope, session domain.Session, guard *scope.Guard) ([]domain.CapturedRequest, error) {
	var captured []domain.CapturedRequest
	wantCrawl := opts.Mode == "crawl" || opts.Mode == "both"
	wantImport := opts.Mode == "import" || opts.Mode == "both"

	if opts.Mode == "import" && opts.ImportPath == "" {
		return nil, fmt.Errorf("--discovery import requires --import <har|openapi file>")
	}
	if wantCrawl {
		found, err := crawlTarget(ctx, sc, session, guard)
		if err != nil {
			return nil, err
		}
		captured = append(captured, found...)
	}
	if wantImport && opts.ImportPath != "" {
		imported, err := importRequests(opts.ImportPath, sc.Target)
		if err != nil {
			return nil, err
		}
		captured = append(captured, imported...)
	}
	return dedupeByEndpoint(captured), nil
}

// crawlTarget walks the target within Scope. A crawl that reaches nothing
// at all is a hard error (wrong target, wrong session, everything refused);
// a crawl that reached some pages before erroring is normal operation.
func crawlTarget(ctx context.Context, sc domain.Scope, session domain.Session, guard *scope.Guard) ([]domain.CapturedRequest, error) {
	found, err := crawl.New(guard, crawl.Options{}).Crawl(ctx, sc.Target, session)
	if err != nil && len(found) == 0 {
		return nil, fmt.Errorf("crawl %s: %w", sc.Target, err)
	}
	return found, nil
}

// importRequests picks the importer by file extension, falling back to
// trying HAR first then OpenAPI — the two shapes are unambiguous enough in
// practice that guessing wrong fails loudly rather than silently.
func importRequests(path, baseURL string) ([]domain.CapturedRequest, error) {
	ext := strings.ToLower(filepath.Ext(path))
	if ext == ".har" {
		return crawl.ImportHAR(path)
	}
	if imported, err := crawl.ImportOpenAPI(path, baseURL); err == nil {
		return imported, nil
	}
	imported, err := crawl.ImportHAR(path)
	if err != nil {
		return nil, fmt.Errorf("import %s: not a recognizable OpenAPI document or HAR file: %w", path, err)
	}
	return imported, nil
}

func dedupeByEndpoint(in []domain.CapturedRequest) []domain.CapturedRequest {
	seen := make(map[string]bool, len(in))
	out := make([]domain.CapturedRequest, 0, len(in))
	for _, r := range in {
		key := r.Endpoint.Method + " " + r.Endpoint.Path
		if seen[key] {
			continue
		}
		seen[key] = true
		out = append(out, r)
	}
	return out
}

// establishCandidate runs Phase B for one ranked Workflow: apply any
// user-declared Level 5 invariant, otherwise probe. When the probe reports
// a missing precondition, attach the best setup candidate and probe again —
// the setup-dependency heuristic from Design/ARCHITECTURE.md §2.3.
func establishCandidate(
	ctx context.Context,
	base scheduler.RunConfig,
	r ranking.Ranked,
	captured []domain.CapturedRequest,
	declared []domain.UserInvariant,
	attempts int,
) (domain.Candidate, bool, error) {
	act, ok := r.Workflow.RequestByID(r.Workflow.ActRequest)
	if !ok {
		return domain.Candidate{}, false, fmt.Errorf("workflow %s has no act request", r.Workflow.ID)
	}

	// Level 5 first: a declared invariant is authoritative and skips the
	// probe entirely (Design/ARCHITECTURE.md §4).
	if userInv, matched := ranking.ApplyUserInvariants(declared, act.Method, act.URL); matched {
		return candidateFrom(r, r.Workflow, userInv.Invariant, userInv.SuccessWhen, userInv.RejectWhen,
			userInv.PostStateProbe, userInv.BodyDiff, hasSetup(r.Workflow)), true, nil
	}

	send := func(ctx context.Context, wf domain.Workflow, attempts int) ([]ranking.ProbeResult, error) {
		observations, err := scheduler.ProbeWorkflow(ctx, base, wf, attempts)
		results := make([]ranking.ProbeResult, 0, len(observations))
		for _, o := range observations {
			results = append(results, ranking.ProbeResult{Status: o.Status, Body: o.Body})
		}
		return results, err
	}

	wf := r.Workflow
	inv, established, err := ranking.ProbeInvariant(ctx, send, wf, attempts)
	if err != nil {
		return domain.Candidate{}, false, err
	}

	if !established {
		// Maybe it just needed a Setup phase. Try only the best-ranked setup
		// candidate — one retry, not a search, to keep probe traffic minimal
		// (Design/ARCHITECTURE.md §2.4: Phase B is "cheap traffic").
		actCaptured := capturedByID(captured, r.Workflow.ActRequest)
		if setups := discovery.FindSetupCandidates(actCaptured, captured); len(setups) > 0 {
			withSetup := buildWorkflowWithSetup(ctx, base, wf, setups[0])
			inv, established, err = ranking.ProbeInvariant(ctx, send, withSetup, attempts)
			if err != nil {
				return domain.Candidate{}, false, err
			}
			if established {
				wf = withSetup
			}
		}
	}
	if !established {
		return domain.Candidate{}, false, nil
	}

	return candidateFrom(r, wf, inv, nil, nil, nil, nil, hasSetup(wf)), true, nil
}

// buildWorkflowWithSetup attaches a Setup phase to a Workflow and, when the
// Act Request's body shape is still unknown (the normal case for an
// endpoint scraped from inline JavaScript), learns it: run the setup once,
// look at what it actually returned, and bind that field into the Act
// Request. Inference from observed evidence, not from an endpoint name.
func buildWorkflowWithSetup(ctx context.Context, base scheduler.RunConfig, wf domain.Workflow, setup domain.CapturedRequest) domain.Workflow {
	withSetup := discovery.AttachSetup(wf, setup, guessBindField(setup))

	setupSpec, ok := withSetup.RequestByID(setup.ID)
	if !ok {
		return withSetup
	}
	status, body, err := scheduler.SendOneRequest(ctx, base, setupSpec)
	if err != nil || status < 200 || status >= 300 {
		return withSetup // the setup is not usable; the probe will say so
	}
	return discovery.SynthesizeActBinding(withSetup, setup.ID, body)
}

// candidateFrom normalizes a discovered Workflow + Invariant into the same
// Candidate representation a declared candidate.yaml produces
// (Design/DATA_MODEL.md §4: both origins normalize to one shape).
func candidateFrom(
	r ranking.Ranked,
	wf domain.Workflow,
	inv domain.Invariant,
	successWhen, rejectWhen *domain.Matcher,
	probe *domain.PostStateProbe,
	bodyDiff *domain.BodyDifferential,
	setupAttached bool,
) domain.Candidate {
	score := r.Score
	signals := r.Signals
	signals.LimitObservedInProbe = inv.Source == "inferred"

	c := domain.Candidate{
		ID:               candidateID(wf),
		Source:           "inferred",
		Score:            &score,
		Workflow:         wf,
		Invariant:        inv,
		SuccessWhen:      successWhen,
		RejectWhen:       rejectWhen,
		PostStateProbe:   probe,
		BodyDifferential: bodyDiff,
		ScoreSignals:     &signals,
	}
	if setupAttached {
		// Re-running the attached Setup phase per trial is exactly the
		// setup_recipe Reset Recipe, which is what lets these trials count as
		// independent (Design/DOMAIN.md §State Independence). Without a setup
		// there is no executable reset, so independence is left unclaimed and
		// Confidence caps at LIKELY.
		c.ResetRecipe = &domain.ResetRecipe{
			Kind:     domain.ResetSetupRecipe,
			SetupRef: wf.SetupRequests[0],
			How:      "re-run the discovered setup request before each trial",
		}
	}
	return c
}

func hasSetup(wf domain.Workflow) bool { return len(wf.SetupRequests) > 0 }

// candidateID names a discovered Candidate after the endpoint it tests, so
// a run directory and a .rv filename read like the finding they describe
// ("cand_post_coupon_redeem") rather than like a crawl artifact
// ("cand_js_7f674c57").
func candidateID(wf domain.Workflow) string {
	spec, ok := wf.RequestByID(wf.ActRequest)
	if !ok {
		return "cand_" + wf.ActRequest
	}
	slug := strings.ToLower(spec.Method + spec.URL)
	var b strings.Builder
	b.WriteString("cand")
	lastUnderscore := true
	for _, r := range slug {
		switch {
		case (r >= 'a' && r <= 'z') || (r >= '0' && r <= '9'):
			if lastUnderscore {
				b.WriteByte('_')
				lastUnderscore = false
			}
			b.WriteRune(r)
		default:
			lastUnderscore = true
		}
	}
	return b.String()
}

func capturedByID(captured []domain.CapturedRequest, id string) domain.CapturedRequest {
	for _, c := range captured {
		if c.ID == id {
			return c
		}
	}
	return domain.CapturedRequest{ID: id}
}

// guessBindField picks the response field a setup request most likely
// returns for the Act Request to consume. These are the conventional names;
// when none applies the binding is simply not wired and the Workflow runs
// without one.
func guessBindField(setup domain.CapturedRequest) string {
	path := strings.ToLower(setup.Endpoint.Path)
	switch {
	case strings.Contains(path, "code"), strings.Contains(path, "coupon"), strings.Contains(path, "voucher"):
		return "code"
	case strings.Contains(path, "cart"), strings.Contains(path, "order"), strings.Contains(path, "session"):
		return "id"
	case strings.Contains(path, "sku"), strings.Contains(path, "inventory"), strings.Contains(path, "stock"):
		return "sku"
	default:
		return "id"
	}
}

func describeWorkflow(wf domain.Workflow) string {
	if spec, ok := wf.RequestByID(wf.ActRequest); ok {
		return strings.ToUpper(spec.Method) + " " + spec.URL
	}
	return wf.ID
}

// loadUserInvariants reads an optional invariants.yaml (Oracle Level 5
// configuration, Design/DATA_MODEL.md §User invariants).
func loadUserInvariants(path string) ([]domain.UserInvariant, error) {
	if path == "" {
		return nil, nil
	}
	data, err := os.ReadFile(filepath.Clean(path)) // #nosec G304 -- operator-supplied CLI path, expected
	if err != nil {
		return nil, fmt.Errorf("invariants.yaml: read %s: %w", path, err)
	}
	var declared []domain.UserInvariant
	if err := yaml.Unmarshal(data, &declared); err != nil {
		return nil, fmt.Errorf("invariants.yaml: %s: %w", path, err)
	}
	return declared, nil
}
