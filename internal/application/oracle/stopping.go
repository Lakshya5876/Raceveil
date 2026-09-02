package oracle

import "github.com/Lakshya5876/Raceveil/internal/domain"

// BandFor maps a hypothetical (r, k) reproduction outcome to the Confidence
// band it would produce, holding the other inputs fixed. It exists so the
// scheduler can ask "could any remaining trial still change the verdict?"
// without duplicating the band rules.
func BandFor(r, k int, classifierSeparation string, corroboration []string) domain.Confidence {
	if k == 0 && r == 0 {
		return "" // nothing observed yet
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
