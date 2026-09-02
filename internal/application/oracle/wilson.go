package oracle

import "math"

// Z95 is the standard normal critical value for a 95% confidence interval,
// used for every Wilson score interval RaceVeil computes
// (Design/ARCHITECTURE.md §5).
const Z95 = 1.96

// WilsonInterval computes the Wilson score interval for r successes in n
// Bernoulli trials at critical value z (Design/ARCHITECTURE.md §5, ADR-013).
// Chosen over the normal approximation because it behaves well for small n
// and p near 0/1 — exactly RaceVeil's regime. Returns (0, 0) for n == 0
// (undefined — the caller must not treat that as "no violation").
func WilsonInterval(r, n int, z float64) (lo, hi float64) {
	if n <= 0 {
		return 0, 0
	}
	nf := float64(n)
	p := float64(r) / nf
	z2 := z * z
	denom := 1 + z2/nf
	center := p + z2/(2*nf)
	adj := z * math.Sqrt(p*(1-p)/nf+z2/(4*nf*nf))
	lo = clamp01((center - adj) / denom)
	hi = clamp01((center + adj) / denom)
	return lo, hi
}

func clamp01(v float64) float64 {
	if v < 0 {
		return 0
	}
	if v > 1 {
		return 1
	}
	return v
}
