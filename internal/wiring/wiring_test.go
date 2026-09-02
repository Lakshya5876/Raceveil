package wiring

import (
	"context"
	"fmt"
	"net"
	"os"
	"path/filepath"
	"strconv"
	"testing"
	"time"

	"github.com/Lakshya5876/Raceveil/internal/domain"
	"github.com/Lakshya5876/Raceveil/internal/testutil/fixtureserver"
)

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

func writeFile(t *testing.T, dir, name, content string) string {
	t.Helper()
	path := filepath.Join(dir, name)
	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		t.Fatalf("write %s: %v", path, err)
	}
	return path
}

const candidateTemplate = `
workflow:
  requests:
    - id: issue_code
      method: POST
      url: %s
    - id: redeem
      method: POST
      url: %s
      headers: {content-type: application/json}
      body: '{"code":"${CODE}"}'
      bindings: [{name: CODE, in: body, from: issue_code.$.code}]
  act_request: redeem
setup_requests: [issue_code]
session_ref: session.json
invariant: {type: max_successes, value: 1}
success_when: {status: 200}
reject_when: {status: 409}
reset_recipe: {kind: fresh_code, setup_ref: issue_code, how: "re-run issue_code before each trial"}
`

const scopeTemplate = `
target: http://%s
authorized_by: "wiring integration test"
hosts:
  - {host: 127.0.0.1, ports: [%d], pin_ip: 127.0.0.1, path_prefixes: ["/"]}
allow_private_target: true
caps: {max_concurrency: 10, max_requests_total: 2000, max_rate_per_sec: 100, max_wallclock_sec: 60}
proof: {max_successful_effects: 2}
`

const sessionJSON = `{"id":"sess_fixture","cookies":[],"headers":{},"isolation_group":"fixture"}`

func setupFixtureFiles(t *testing.T, issuePath, redeemPath, addr string, port int) (candidatePath, scopePath, sessionPath string) {
	t.Helper()
	dir := t.TempDir()
	candidatePath = writeFile(t, dir, "candidate.yaml", fmt.Sprintf(candidateTemplate, issuePath, redeemPath))
	scopePath = writeFile(t, dir, "scope.yaml", fmt.Sprintf(scopeTemplate, addr, port))
	sessionPath = writeFile(t, dir, "session.json", sessionJSON)
	return candidatePath, scopePath, sessionPath
}

func mustSplitPort(t *testing.T, addr string) int {
	t.Helper()
	_, portStr, err := net.SplitHostPort(addr)
	if err != nil {
		t.Fatalf("split host:port %q: %v", addr, err)
	}
	port, err := strconv.Atoi(portStr)
	if err != nil {
		t.Fatalf("parse port %q: %v", portStr, err)
	}
	return port
}

func TestRunScan_VulnerableFixture_ProducesLikelyFinding(t *testing.T) {
	srv := startFixture(t)
	port := mustSplitPort(t, srv.Addr())
	candidatePath, scopePath, sessionPath := setupFixtureFiles(t, "/issue-code", "/redeem", srv.Addr(), port)

	result, err := RunScan(context.Background(), ScanOptions{
		CandidatePath: candidatePath,
		ScopePath:     scopePath,
		AuthPath:      sessionPath,
		SafeMode:      true,
		OutDir:        filepath.Join(t.TempDir(), "run"),
	})
	if err != nil {
		t.Fatalf("RunScan: %v", err)
	}
	if !result.Found {
		t.Fatal("expected a Finding through the full wiring path (real Guard, real Writer)")
	}
	if result.Oracle.Confidence != domain.Likely {
		t.Errorf("Confidence = %v, want LIKELY", result.Oracle.Confidence)
	}

	// verify re-confirms on fresh trials.
	verifyResult, _, err := RunVerify(context.Background(), VerifyOptions{
		FindingPath: findingFilePath(t, result),
		ScopePath:   scopePath,
		AuthPath:    sessionPath,
		SafeMode:    true,
		OutDir:      filepath.Join(t.TempDir(), "verify"),
	})
	if err != nil {
		t.Fatalf("RunVerify: %v", err)
	}
	if !verifyResult.Found {
		t.Error("expected verify to re-confirm the finding on fresh trials")
	}

	// replay runs once and reports observed vs expected without erroring.
	trial, _, err := RunReplay(context.Background(), ReplayOptions{
		FindingPath: findingFilePath(t, result),
		ScopePath:   scopePath,
		AuthPath:    sessionPath,
		SafeMode:    true,
		OutDir:      filepath.Join(t.TempDir(), "replay"),
	})
	if err != nil {
		t.Fatalf("RunReplay: %v", err)
	}
	if trial.N == 0 {
		t.Error("expected replay to report a non-zero N")
	}
}

func TestRunScan_SafeTwinFixture_NoFinding(t *testing.T) {
	srv := startFixture(t)
	port := mustSplitPort(t, srv.Addr())
	candidatePath, scopePath, sessionPath := setupFixtureFiles(t, "/issue-code-safe", "/redeem-safe", srv.Addr(), port)

	result, err := RunScan(context.Background(), ScanOptions{
		CandidatePath: candidatePath,
		ScopePath:     scopePath,
		AuthPath:      sessionPath,
		SafeMode:      true,
		OutDir:        filepath.Join(t.TempDir(), "run"),
	})
	if err != nil {
		t.Fatalf("RunScan: %v", err)
	}
	if result.Found {
		t.Fatalf("expected no Finding against the synchronized-safe twin, got Confidence=%v", result.Oracle.Confidence)
	}
}

func TestRunVerify_RefusesScopeTargetMismatch(t *testing.T) {
	srv := startFixture(t)
	port := mustSplitPort(t, srv.Addr())
	candidatePath, scopePath, sessionPath := setupFixtureFiles(t, "/issue-code", "/redeem", srv.Addr(), port)

	result, err := RunScan(context.Background(), ScanOptions{
		CandidatePath: candidatePath, ScopePath: scopePath, AuthPath: sessionPath, SafeMode: true,
		OutDir: filepath.Join(t.TempDir(), "run"),
	})
	if err != nil || !result.Found {
		t.Fatalf("setup scan failed: found=%v err=%v", result.Found, err)
	}

	mismatchedScope := writeFile(t, t.TempDir(), "scope.yaml", fmt.Sprintf(scopeTemplate, "127.0.0.1:9", 9))
	_, _, err = RunVerify(context.Background(), VerifyOptions{
		FindingPath: findingFilePath(t, result),
		ScopePath:   mismatchedScope,
		AuthPath:    sessionPath,
		SafeMode:    true,
	})
	if !IsScopeRefusal(err) {
		t.Fatalf("expected a scope-refusal error for a mismatched target, got %v", err)
	}
}

func findingFilePath(t *testing.T, r ScanResult) string {
	t.Helper()
	if r.Finding == nil {
		t.Fatal("ScanResult has no Finding")
	}
	return filepath.Join(r.Finding.EvidenceRefs.RunDir, "findings", r.Finding.FindingID+".rv")
}
