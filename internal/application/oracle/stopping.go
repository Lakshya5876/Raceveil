package oracle

import "github.com/Lakshya5876/Raceveil/internal/domain"

// BandFor maps a hypothetical (r, k) reproduction outcome to the Confidence
// band it would produce, holding the other inputs fixed. It exists so the
// scheduler can ask "could any remaining trial still change the verdict?"
// without duplicating the band rules.
func BandFor(r, k int, classifierSeparation string, corroboration []string) domain.Confidence {
	if r == 0 {
		// Mirrors Evaluate's anyViolation gate: zero observed violations is
		// not a (weak) Confidence band, it is no Finding at all. Without
		// this, a hypothetical r=0 would fall through to classifyConfidence
		// and come back "LIKELY" (kIndependent>0, rViolations<2), which
		// would make BandCanStillChange think the verdict is already
		// settled at "no violation yet" and stop the trial loop before a
		// single violation has even been observed.
		return ""
	}
	lo, _ := WilsonInterval(r, k, Z95)
	return classifyConfidence(classifyInput{
		rViolations:          r,
		kIndependent:         k,
		wilsonLo:             lo,
		classifierSeparation: classifierSeparation,
		corroboration:        corroboration,
	})
}

// BandCanStillChange reports whether any assignment of the remaining trials
// could still move the Confidence band (Design/ARCHITECTURE.md §5 stopping
// criteria: "stop early when the band can no longer change").
//
// It answers the question exactly rather than by heuristic: compare the band
// if every remaining trial violated against the band if none did. If both
// extremes agree, every outcome in between agrees too, and running the rest
// of the trials cannot teach us anything — so stop, and stop having caused
// fewer effects on the target than the budget allowed.
func BandCanStillChange(r, k, remaining int, classifierSeparation string, corroboration []string) bool {
	if remaining <= 0 {
		return false
	}
	allViolate := BandFor(r+remaining, k+remaining, classifierSeparation, corroboration)
	noneViolate := BandFor(r, k+remaining, classifierSeparation, corroboration)
	return allViolate != noneViolate
}
