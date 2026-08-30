// Package scheduler owns the Experiment lifecycle: acquire fresh Session/
// state, run the Baseline, run the Workflow's Setup phase once, hand N
// prepared Act instances to the concurrency engine, evaluate via the Oracle,
// verify, minimize, and record everything to the run directory (Design/
// ARCHITECTURE.md §2.5).
package scheduler

import (
	"bytes"
	"encoding/json"
	"fmt"
	"os"

	"github.com/Lakshya5876/Raceveil/internal/domain"
	"gopkg.in/yaml.v3"
)

// yamlWorkflow mirrors candidate.yaml's workflow block (Design/DATA_MODEL.md
// §MVP candidate input) — act_request and setup_requests are siblings of
// workflow at the top level of the file, not nested inside it.
type yamlWorkflow struct {
	Requests   []domain.RequestSpec `yaml:"requests"`
	ActRequest string               `yaml:"act_request"`
}

// yamlCandidate is the exact, strict shape of candidate.yaml
// (Design/DATA_MODEL.md §MVP candidate input, Phase-1 contract).
type yamlCandidate struct {
	Workflow       yamlWorkflow           `yaml:"workflow"`
	SetupRequests  []string               `yaml:"setup_requests"`
	SessionRef     string                 `yaml:"session_ref"`
	Invariant      domain.Invariant       `yaml:"invariant"`
	SuccessWhen    *domain.Matcher        `yaml:"success_when"`
	RejectWhen     *domain.Matcher        `yaml:"reject_when"`
	PostStateProbe *domain.PostStateProbe `yaml:"post_state_probe"`
	ResetRecipe    *domain.ResetRecipe    `yaml:"reset_recipe"`
}

// LoadCandidate reads and strictly validates a candidate.yaml against the
// Phase-1 contract (Design/DATA_MODEL.md): unknown fields, invariant types
// other than max_successes, and an act_request not present in requests are
// all rejected with a clear error rather than silently coerced.
func LoadCandidate(path string) (domain.Candidate, error) {
	data, err := os.ReadFile(path) // #nosec G304 -- operator-supplied CLI path, expected
	if err != nil {
		return domain.Candidate{}, fmt.Errorf("candidate.yaml: read %s: %w", path, err)
	}

	dec := yaml.NewDecoder(bytes.NewReader(data))
	dec.KnownFields(true)
	var yc yamlCandidate
	if err := dec.Decode(&yc); err != nil {
		return domain.Candidate{}, fmt.Errorf("candidate.yaml: %s: %w", path, err)
	}

	if err := validateCandidate(yc); err != nil {
		return domain.Candidate{}, fmt.Errorf("candidate.yaml: %s: %w", path, err)
	}

	return domain.Candidate{
		ID:     "cand_" + yc.Workflow.ActRequest,
		Source: "declared",
		Score:  nil,
		Workflow: domain.Workflow{
			ID:            "wf_" + yc.Workflow.ActRequest,
			Requests:      yc.Workflow.Requests,
			ActRequest:    yc.Workflow.ActRequest,
			SetupRequests: yc.SetupRequests,
		},
		SessionRef:     yc.SessionRef,
		Invariant:      withDeclaredSource(yc.Invariant),
		SuccessWhen:    yc.SuccessWhen,
		RejectWhen:     yc.RejectWhen,
		PostStateProbe: yc.PostStateProbe,
		ResetRecipe:    yc.ResetRecipe,
	}, nil
}

func withDeclaredSource(inv domain.Invariant) domain.Invariant {
	inv.Source = "declared"
	return inv
}

func validateCandidate(yc yamlCandidate) error {
	if err := validateWorkflow(yc); err != nil {
		return err
	}
	if err := validateInvariant(yc.Invariant); err != nil {
		return err
	}
	if yc.SessionRef == "" {
		return fmt.Errorf("session_ref is required")
	}
	return validateResetRecipe(yc.ResetRecipe)
}

func validateWorkflow(yc yamlCandidate) error {
	if len(yc.Workflow.Requests) == 0 {
		return fmt.Errorf("workflow.requests must have at least one entry")
	}
	if yc.Workflow.ActRequest == "" {
		return fmt.Errorf("workflow.act_request is required")
	}
	wf := domain.Workflow{Requests: yc.Workflow.Requests}
	if _, ok := wf.RequestByID(yc.Workflow.ActRequest); !ok {
		return fmt.Errorf("workflow.act_request %q is not present in workflow.requests", yc.Workflow.ActRequest)
	}
	for _, id := range yc.SetupRequests {
		if _, ok := wf.RequestByID(id); !ok {
			return fmt.Errorf("setup_requests references %q which is not present in workflow.requests", id)
		}
	}
	return nil
}

func validateInvariant(inv domain.Invariant) error {
	if inv.Type == "" {
		return fmt.Errorf("invariant.type is required")
	}
	if inv.Type != domain.InvariantMaxSuccesses {
		return fmt.Errorf("invariant.type %q is not supported in Phase 1 (only max_successes is implemented)", inv.Type)
	}
	if inv.Value < 0 {
		return fmt.Errorf("invariant.value must be >= 0, got %d", inv.Value)
	}
	return nil
}

func validateResetRecipe(r *domain.ResetRecipe) error {
	if r == nil {
		return nil
	}
	switch r.Kind {
	case domain.ResetFreshCode, domain.ResetSetupRecipe:
		return nil
	case domain.ResetEndpoint, domain.ResetFixtureCommand:
		return fmt.Errorf("reset_recipe.kind %q is not supported in Phase 1 (only fresh_code and setup_recipe are implemented)", r.Kind)
	default:
		return fmt.Errorf("reset_recipe.kind %q is not a recognized recipe kind", r.Kind)
	}
}

// LoadScope reads a scope.yaml into a domain.Scope.
func LoadScope(path string) (domain.Scope, error) {
	data, err := os.ReadFile(path) // #nosec G304 -- operator-supplied CLI path, expected
	if err != nil {
		return domain.Scope{}, fmt.Errorf("scope.yaml: read %s: %w", path, err)
	}
	var s domain.Scope
	if err := yaml.Unmarshal(data, &s); err != nil {
		return domain.Scope{}, fmt.Errorf("scope.yaml: %s: %w", path, err)
	}
	return s, nil
}

// LoadSession reads a session.json into a domain.Session.
func LoadSession(path string) (domain.Session, error) {
	data, err := os.ReadFile(path) // #nosec G304 -- operator-supplied CLI path, expected
	if err != nil {
		return domain.Session{}, fmt.Errorf("session.json: read %s: %w", path, err)
	}
	var s domain.Session
	if err := json.Unmarshal(data, &s); err != nil {
		return domain.Session{}, fmt.Errorf("session.json: %s: %w", path, err)
	}
	return s, nil
}
