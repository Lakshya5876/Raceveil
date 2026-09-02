package minimize

import (
	"context"
	"errors"
	"reflect"
	"testing"
)

func TestMinimizeWorkflow_DropsUnnecessaryRequests(t *testing.T) {
	// Only "add_coupon" is actually required; "add_to_cart" and "login" can
	// both be dropped and the violation still reproduces.
	check := func(_ context.Context, requests []string) (bool, error) {
		for _, r := range requests {
			if r == "add_coupon" {
				return true, nil
			}
		}
		return false, nil
	}
	result, err := MinimizeWorkflow(context.Background(), check, []string{"login", "add_to_cart", "add_coupon"})
	if err != nil {
		t.Fatalf("MinimizeWorkflow: %v", err)
	}
	want := []string{"add_coupon"}
	if !reflect.DeepEqual(result.MinimalRequests, want) {
		t.Errorf("MinimalRequests = %v, want %v", result.MinimalRequests, want)
	}
	if !reflect.DeepEqual(result.StartRequests, []string{"login", "add_to_cart", "add_coupon"}) {
		t.Errorf("StartRequests should preserve the original list, got %v", result.StartRequests)
	}
}

func TestMinimizeWorkflow_KeepsAllWhenAllRequired(t *testing.T) {
	check := func(_ context.Context, requests []string) (bool, error) {
		return len(requests) == 2, nil // both requests must be present
	}
	result, err := MinimizeWorkflow(context.Background(), check, []string{"issue_code", "redeem"})
	if err != nil {
		t.Fatalf("MinimizeWorkflow: %v", err)
	}
	want := []string{"issue_code", "redeem"}
	if !reflect.DeepEqual(result.MinimalRequests, want) {
		t.Errorf("MinimalRequests = %v, want %v (nothing droppable)", result.MinimalRequests, want)
	}
}

func TestMinimizeWorkflow_PropagatesCheckError(t *testing.T) {
	wantErr := errors.New("boom")
	check := func(_ context.Context, requests []string) (bool, error) { return false, wantErr }
	if _, err := MinimizeWorkflow(context.Background(), check, []string{"a", "b"}); !errors.Is(err, wantErr) {
		t.Fatalf("expected wrapped check error, got %v", err)
	}
}
