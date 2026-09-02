package minimize

import (
	"context"
	"errors"
	"testing"
)

func TestSweepConcurrency_FindsSmallestReproducingN(t *testing.T) {
	// Reproduces at N>=3, never at N=2.
	run := func(_ context.Context, n int) (bool, error) {
		return n >= 3, nil
	}
	result, err := SweepConcurrency(context.Background(), run, 5, 3)
	if err != nil {
		t.Fatalf("SweepConcurrency: %v", err)
	}
	if result.StartN != 5 {
		t.Errorf("StartN = %d, want 5", result.StartN)
	}
	if result.MinimalN != 3 {
		t.Errorf("MinimalN = %d, want 3", result.MinimalN)
	}
	if result.MinRepro.RViolations != 3 || result.MinRepro.KIndependent != 3 {
		t.Errorf("MinRepro = %+v, want 3/3 (always reproduces at N=3)", result.MinRepro)
	}
}

func TestSweepConcurrency_NeverReproduces_KeepsStartN(t *testing.T) {
	run := func(_ context.Context, n int) (bool, error) { return false, nil }
	result, err := SweepConcurrency(context.Background(), run, 5, 2)
	if err != nil {
		t.Fatalf("SweepConcurrency: %v", err)
	}
	if result.MinimalN != 5 {
		t.Errorf("MinimalN = %d, want startN=5 when nothing reproduces", result.MinimalN)
	}
	if result.MinRepro.RViolations != 0 {
		t.Errorf("MinRepro.RViolations = %d, want 0", result.MinRepro.RViolations)
	}
}

func TestSweepConcurrency_RejectsStartNBelow2(t *testing.T) {
	run := func(_ context.Context, n int) (bool, error) { return true, nil }
	if _, err := SweepConcurrency(context.Background(), run, 1, 2); err == nil {
		t.Fatal("expected an error for startN < 2")
	}
}

func TestSweepConcurrency_PropagatesTrialError(t *testing.T) {
	wantErr := errors.New("boom")
	run := func(_ context.Context, n int) (bool, error) { return false, wantErr }
	if _, err := SweepConcurrency(context.Background(), run, 3, 2); !errors.Is(err, wantErr) {
		t.Fatalf("expected wrapped trial error, got %v", err)
	}
}
