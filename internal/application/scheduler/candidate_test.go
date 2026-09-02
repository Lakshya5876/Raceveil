package scheduler

import (
	"fmt"
	"os"
	"path/filepath"
	"testing"

	"github.com/Lakshya5876/Raceveil/internal/domain"
)

func writeTemp(t *testing.T, name, content string) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), name)
	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		t.Fatalf("write %s: %v", path, err)
	}
	return path
}

const validCandidateYAML = `
workflow:
  requests:
    - id: issue_code
      method: POST
      url: /issue-code
    - id: redeem
      method: POST
      url: /redeem
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

func TestLoadCandidate_ValidFixtureCandidate(t *testing.T) {
	path := writeTemp(t, "candidate.yaml", validCandidateYAML)
	c, err := LoadCandidate(path)
	if err != nil {
		t.Fatalf("LoadCandidate: %v", err)
	}
	if c.Source != "declared" || c.Score != nil {
		t.Errorf("expected declared candidate with nil score, got source=%q score=%v", c.Source, c.Score)
	}
	if c.Workflow.ActRequest != "redeem" {
		t.Errorf("ActRequest = %q, want redeem", c.Workflow.ActRequest)
	}
	if c.Invariant.Type != domain.InvariantMaxSuccesses || c.Invariant.Value != 1 {
		t.Errorf("Invariant = %+v, want max_successes=1", c.Invariant)
	}
}

func TestLoadCandidate_RejectsUnknownField(t *testing.T) {
	yaml := validCandidateYAML + "\nsome_unknown_field: true\n"
	path := writeTemp(t, "candidate.yaml", yaml)
	if _, err := LoadCandidate(path); err == nil {
		t.Fatal("expected LoadCandidate to reject an unknown top-level field")
	}
}

func TestLoadCandidate_RejectsUnknownInvariantType(t *testing.T) {
	yaml := `
workflow:
  requests:
    - {id: r1, method: POST, url: /x}
  act_request: r1
session_ref: session.json
invariant: {type: made_up_type, value: 1}
`
	path := writeTemp(t, "candidate.yaml", yaml)
	if _, err := LoadCandidate(path); err == nil {
		t.Fatal("expected LoadCandidate to reject an invariant.type outside the canonical four")
	}
}

func TestLoadCandidate_AcceptsAllFourCanonicalInvariantTypes(t *testing.T) {
	for _, invType := range []string{"max_successes", "uniqueness", "monotonic_limit", "single_transition"} {
		yaml := fmt.Sprintf(`
workflow:
  requests:
    - {id: r1, method: POST, url: /x}
  act_request: r1
session_ref: session.json
invariant: {type: %s, value: 1}
`, invType)
		path := writeTemp(t, "candidate-"+invType+".yaml", yaml)
		c, err := LoadCandidate(path)
		if err != nil {
			t.Errorf("LoadCandidate rejected canonical type %q: %v", invType, err)
			continue
		}
		if string(c.Invariant.Type) != invType {
			t.Errorf("Invariant.Type = %q, want %q", c.Invariant.Type, invType)
		}
	}
}

func TestLoadCandidate_RejectsActRequestNotInRequests(t *testing.T) {
	yaml := `
workflow:
  requests:
    - {id: r1, method: POST, url: /x}
  act_request: does_not_exist
session_ref: session.json
invariant: {type: max_successes, value: 1}
`
	path := writeTemp(t, "candidate.yaml", yaml)
	if _, err := LoadCandidate(path); err == nil {
		t.Fatal("expected LoadCandidate to reject an act_request absent from requests")
	}
}

func TestLoadCandidate_RejectsEmptyRequests(t *testing.T) {
	yaml := `
workflow:
  requests: []
  act_request: r1
session_ref: session.json
invariant: {type: max_successes, value: 1}
`
	path := writeTemp(t, "candidate.yaml", yaml)
	if _, err := LoadCandidate(path); err == nil {
		t.Fatal("expected LoadCandidate to reject workflow.requests with zero entries")
	}
}

func TestLoadCandidate_RejectsMissingSessionRef(t *testing.T) {
	yaml := `
workflow:
  requests:
    - {id: r1, method: POST, url: /x}
  act_request: r1
invariant: {type: max_successes, value: 1}
`
	path := writeTemp(t, "candidate.yaml", yaml)
	if _, err := LoadCandidate(path); err == nil {
		t.Fatal("expected LoadCandidate to reject a missing session_ref")
	}
}

func TestLoadCandidate_RejectsUnsupportedResetRecipeKind(t *testing.T) {
	yaml := `
workflow:
  requests:
    - {id: r1, method: POST, url: /x}
  act_request: r1
session_ref: session.json
invariant: {type: max_successes, value: 1}
reset_recipe: {kind: reset_endpoint, how: "hit /reset"}
`
	path := writeTemp(t, "candidate.yaml", yaml)
	if _, err := LoadCandidate(path); err == nil {
		t.Fatal("expected LoadCandidate to reject reset_endpoint (not implemented in Phase 1)")
	}
}

func TestLoadScope_ParsesFixtureScope(t *testing.T) {
	yaml := `
target: http://127.0.0.1:18743
authorized_by: test
hosts:
  - {host: 127.0.0.1, ports: [18743], pin_ip: 127.0.0.1, path_prefixes: ["/"]}
allow_private_target: true
proof: {max_successful_effects: 2}
`
	path := writeTemp(t, "scope.yaml", yaml)
	s, err := LoadScope(path)
	if err != nil {
		t.Fatalf("LoadScope: %v", err)
	}
	if s.Target != "http://127.0.0.1:18743" || s.Proof.MaxSuccessfulEffects != 2 {
		t.Errorf("unexpected scope: %+v", s)
	}
}

func TestLoadSession_ParsesFixtureSession(t *testing.T) {
	path := writeTemp(t, "session.json", `{"id":"sess_fixture","cookies":[],"headers":{},"isolation_group":"fixture"}`)
	s, err := LoadSession(path)
	if err != nil {
		t.Fatalf("LoadSession: %v", err)
	}
	if s.ID != "sess_fixture" || s.IsolationGroup != "fixture" {
		t.Errorf("unexpected session: %+v", s)
	}
}
