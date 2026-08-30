package oracle

import "testing"

func approxEqual(a, b, eps float64) bool {
	d := a - b
	if d < 0 {
		d = -d
	}
	return d <= eps
}

func TestWilsonInterval_KnownValues(t *testing.T) {
	// Independently derived from the standard Wilson score interval formula
	// (not copied from Design/DATA_MODEL.md's illustrative numbers) as a
	// sanity check against a well-known worked case: r=5, n=6, z=1.96.
	lo, hi := WilsonInterval(5, 6, Z95)
	if !approxEqual(lo, 0.4365, 0.001) {
		t.Errorf("lo = %.4f, want ~0.4365", lo)
	}
	if !approxEqual(hi, 0.9700, 0.001) {
		t.Errorf("hi = %.4f, want ~0.9700", hi)
	}
}

func TestWilsonInterval_ZeroTrials(t *testing.T) {
	lo, hi := WilsonInterval(0, 0, Z95)
	if lo != 0 || hi != 0 {
		t.Errorf("WilsonInterval(0,0) = (%v,%v), want (0,0)", lo, hi)
	}
}

func TestWilsonInterval_AllSuccess(t *testing.T) {
	lo, hi := WilsonInterval(6, 6, Z95)
	if lo <= 0 || lo >= 1 {
		t.Errorf("lo = %v, want strictly between 0 and 1 even at p=1", lo)
	}
	if hi != 1 {
		t.Errorf("hi = %v, want 1 at p=1", hi)
	}
}

func TestWilsonInterval_AllFailure(t *testing.T) {
	lo, hi := WilsonInterval(0, 6, Z95)
	if lo != 0 {
		t.Errorf("lo = %v, want 0 at p=0", lo)
	}
	if hi <= 0 || hi >= 1 {
		t.Errorf("hi = %v, want strictly between 0 and 1 even at p=0", hi)
	}
}

func TestWilsonInterval_WidensWithSmallerN(t *testing.T) {
	// Same p_hat, smaller n must give a wider (less certain) interval.
	loSmall, hiSmall := WilsonInterval(1, 2, Z95)
	loBig, hiBig := WilsonInterval(5, 10, Z95)
	widthSmall := hiSmall - loSmall
	widthBig := hiBig - loBig
	if widthSmall <= widthBig {
		t.Errorf("expected n=2 interval (%v) wider than n=10 interval (%v) at the same p_hat", widthSmall, widthBig)
	}
}
