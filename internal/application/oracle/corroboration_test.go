package oracle

import "testing"

func TestDistinctEffects(t *testing.T) {
	cases := []struct {
		name   string
		values []string
		want   int
	}{
		{"all distinct", []string{"r1", "r2", "r3"}, 3},
		{"duplicates collapse", []string{"r1", "r1", "r1"}, 1},
		{"empty values ignored", []string{"r1", "", "r2", ""}, 2},
		{"none", nil, 0},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if got := DistinctEffects(c.values); got != c.want {
				t.Errorf("DistinctEffects(%v) = %d, want %d", c.values, got, c.want)
			}
		})
	}
}

func TestLevel2Corroborates(t *testing.T) {
	if !Level2Corroborates(2, 1) {
		t.Error("2 distinct effects should corroborate a max_successes(1) violation")
	}
	if Level2Corroborates(1, 1) {
		t.Error("1 distinct effect must not corroborate max_successes(1) — that's the expected single success")
	}
}

func TestLevel4Corroborates(t *testing.T) {
	if !Level4Corroborates(2, 1) {
		t.Error("persisted count of 2 should corroborate a max_successes(1) violation")
	}
	if Level4Corroborates(1, 1) {
		t.Error("persisted count of 1 must not corroborate max_successes(1)")
	}
}
