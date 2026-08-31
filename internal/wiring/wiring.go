// Package wiring assembles RaceVeil's CLI commands with their concrete
// dependencies: the Experiment Scheduler, Oracle, Scope guard, and the
// filesystem run-directory writer (docs/SYSTEM_DESIGN.md §2). This is the
// single place that constructs concrete types and hands them to
// internal/presentation/cli as interfaces — CORE_FILES (docs/
// ARCHITECTURE_DECISIONS.md GOV-004), since editing it can silently change
// what every command actually runs against.
package wiring

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/Lakshya5876/Raceveil/internal/application/scheduler"
	"github.com/Lakshya5876/Raceveil/internal/domain"
	"github.com/Lakshya5876/Raceveil/internal/infrastructure/persist"
	"github.com/Lakshya5876/Raceveil/internal/security/scope"
)

// ErrScopeTargetMismatch is returned by RunVerify/RunReplay when the
// supplied Scope's target does not match the Finding's recorded target
// (Design/API.md verify: "Refuses if the current Scope doesn't cover the
// finding's target").
var ErrScopeTargetMismatch = errors.New("scope target does not cover finding target")

// ScanOptions are the resolved `raceveil scan --candidate` flags.
type ScanOptions struct {
	CandidatePath string
	ScopePath     string
	AuthPath      string
	OutDir        string
}

// ScanResult is presentation's view of a scan/verify outcome — it never
// sees scheduler.Outcome directly, only this wiring-owned shape.
type ScanResult struct {
	Found   bool
	Oracle  domain.OracleResult
	Finding *domain.Finding
}

func scanResultFrom(o scheduler.Outcome) ScanResult {
	return ScanResult{Found: o.Found, Oracle: o.Oracle, Finding: o.Finding}
}

func defaultRunDir(prefix string) string {
	return fmt.Sprintf("%s-%s", prefix, time.Now().UTC().Format("20060102T150405Z"))
}

func newGuardAndStore(sc domain.Scope, outDir, dirPrefix string) (*scope.Guard, *persist.Writer, error) {
	guard, err := scope.NewGuard(sc)
	if err != nil {
		return nil, nil, fmt.Errorf("scope: %w", err)
	}
	if outDir == "" {
		outDir = defaultRunDir(dirPrefix)
	}
	store, err := persist.NewWriter(outDir)
	if err != nil {
		return nil, nil, err
	}
	if err := store.WriteConfig(sc); err != nil {
		return nil, nil, err
	}
	return guard, store, nil
}

// RunScan wires the concrete Guard and run-directory Writer and executes the
// MVP scan pipeline for one supplied Candidate (Design/API.md scan --candidate).
func RunScan(ctx context.Context, opts ScanOptions) (ScanResult, error) {
	candidate, err := scheduler.LoadCandidate(opts.CandidatePath)
	if err != nil {
		return ScanResult{}, err
	}
	sc, err := scheduler.LoadScope(opts.ScopePath)
	if err != nil {
		return ScanResult{}, err
	}
	session, err := scheduler.LoadSession(opts.AuthPath)
	if err != nil {
		return ScanResult{}, err
	}
	guard, store, err := newGuardAndStore(sc, opts.OutDir, "raceveil-run")
	if err != nil {
		return ScanResult{}, err
	}
	outcome, err := scheduler.Run(ctx, scheduler.RunConfig{
		Candidate: candidate,
		Scope:     sc,
		Session:   session,
		Guard:     guard,
		Store:     store,
		RunDir:    store.RunDir(),
	})
	if err != nil {
		return ScanResult{}, err
	}
	return scanResultFrom(outcome), nil
}

// VerifyOptions are the resolved `raceveil verify` flags.
type VerifyOptions struct {
	FindingPath string
	ScopePath   string
	AuthPath    string
	OutDir      string
}

// RunVerify independently re-confirms a Finding on fresh trials, recomputing
// Confidence from scratch (Design/API.md verify).
func RunVerify(ctx context.Context, opts VerifyOptions) (ScanResult, domain.Finding, error) {
	finding, err := (persist.FindingLoader{}).FetchFinding(opts.FindingPath)
	if err != nil {
		return ScanResult{}, domain.Finding{}, err
	}
	sc, err := scheduler.LoadScope(opts.ScopePath)
	if err != nil {
		return ScanResult{}, finding, err
	}
	if sc.Target != finding.Target {
		return ScanResult{}, finding, fmt.Errorf("%w: scope target %q, finding target %q", ErrScopeTargetMismatch, sc.Target, finding.Target)
	}
	session, err := scheduler.LoadSession(opts.AuthPath)
	if err != nil {
		return ScanResult{}, finding, err
	}
	guard, store, err := newGuardAndStore(sc, opts.OutDir, "raceveil-verify")
	if err != nil {
		return ScanResult{}, finding, err
	}
	candidate := scheduler.CandidateFromFinding(finding)
	outcome, err := scheduler.Run(ctx, scheduler.RunConfig{
		Candidate: candidate,
		Scope:     sc,
		Session:   session,
		Guard:     guard,
		Store:     store,
		RunDir:    store.RunDir(),
	})
	if err != nil {
		return ScanResult{}, finding, err
	}
	return scanResultFrom(outcome), finding, nil
}

// ReplayOptions are the resolved `raceveil replay` flags.
type ReplayOptions struct {
	FindingPath string
	ScopePath   string
	AuthPath    string
	OutDir      string
}

// RunReplay re-runs a Finding's exact recorded burst once for demonstration
// (Design/API.md replay) — it does not recompute Confidence.
func RunReplay(ctx context.Context, opts ReplayOptions) (domain.ConcurrentTrial, domain.Finding, error) {
	finding, err := (persist.FindingLoader{}).FetchFinding(opts.FindingPath)
	if err != nil {
		return domain.ConcurrentTrial{}, domain.Finding{}, err
	}
	sc, err := scheduler.LoadScope(opts.ScopePath)
	if err != nil {
		return domain.ConcurrentTrial{}, finding, err
	}
	if sc.Target != finding.Target {
		return domain.ConcurrentTrial{}, finding, fmt.Errorf("%w: scope target %q, finding target %q", ErrScopeTargetMismatch, sc.Target, finding.Target)
	}
	session, err := scheduler.LoadSession(opts.AuthPath)
	if err != nil {
		return domain.ConcurrentTrial{}, finding, err
	}
	guard, store, err := newGuardAndStore(sc, opts.OutDir, "raceveil-replay")
	if err != nil {
		return domain.ConcurrentTrial{}, finding, err
	}
	candidate := scheduler.CandidateFromFinding(finding)
	n := finding.ConcurrencyN
	if n <= 0 {
		n = 2
	}
	trial, err := scheduler.ReplayOnce(ctx, scheduler.RunConfig{
		Candidate: candidate,
		Scope:     sc,
		Session:   session,
		Guard:     guard,
		Store:     store,
		RunDir:    store.RunDir(),
	}, n)
	return trial, finding, err
}

// IsUnprovableUnderCap reports whether err is (or wraps)
// scheduler.ErrUnprovableUnderCap — presentation checks this without
// importing internal/application directly.
func IsUnprovableUnderCap(err error) bool {
	return errors.Is(err, scheduler.ErrUnprovableUnderCap)
}

// IsScopeRefusal reports whether err is (or wraps) a Scope/Guard refusal —
// either security/scope.ErrOutOfScope or a verify/replay target mismatch —
// presentation checks this without importing internal/security/scope
// directly.
func IsScopeRefusal(err error) bool {
	return errors.Is(err, scope.ErrOutOfScope) || errors.Is(err, ErrScopeTargetMismatch)
}

// ActRequestLine renders "METHOD /path" for a Finding/Candidate's Act
// Request — a small formatting helper shared by cli's output functions.
func ActRequestLine(w domain.Workflow) string {
	spec, ok := w.RequestByID(w.ActRequest)
	if !ok {
		return w.ActRequest
	}
	return strings.ToUpper(spec.Method) + " " + spec.URL
}
