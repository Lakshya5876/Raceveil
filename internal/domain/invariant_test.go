package domain

import "testing"

func TestRequiredProofEffects(t *testing.T) {
	cases := []struct {
		name string
		inv  Invariant
		want int
	}{
		{"max_successes L=1", Invariant{Type: InvariantMaxSuccesses, Value: 1}, 2},
		{"max_successes L=5", Invariant{Type: InvariantMaxSuccesses, Value: 5}, 6},
		{"uniqueness", Invariant{Type: InvariantUniqueness}, 2},
		{"monotonic_limit", Invariant{Type: InvariantMonotonicLimit}, 1},
		{"single_transition", Invariant{Type: InvariantSingleTransition}, 2},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if got := RequiredProofEffects(c.inv); got != c.want {
				t.Errorf("RequiredProofEffects(%+v) = %d, want %d", c.inv, got, c.want)
			}
		})
	}
}
