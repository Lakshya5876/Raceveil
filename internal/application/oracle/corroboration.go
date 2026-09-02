package oracle

// DistinctEffects is the Level 2 (response-body differential) primitive
// (Design/ARCHITECTURE.md §4): the count of distinct non-empty values
// extracted from successful concurrent responses. More distinct values than
// the Invariant permits corroborates that a Level-3 S>L count reflects
// genuinely separate effects (e.g. distinct receipt/order ids, or a balance
// that dropped more than once), not a classifier miscount.
func DistinctEffects(values []string) int {
	seen := make(map[string]struct{}, len(values))
	for _, v := range values {
		if v != "" {
			seen[v] = struct{}{}
		}
	}
	return len(seen)
}

// Level2Corroborates reports whether the count of distinct successful
// effect signatures alone exceeds the Invariant's permitted count.
func Level2Corroborates(distinctEffects, limit int) bool {
	return distinctEffects > limit
}

// Level4Corroborates reports whether a read-only post-state probe's
// persisted count exceeds the Invariant's permitted count
// (Design/ARCHITECTURE.md §4 Level 4).
func Level4Corroborates(persistedCount, limit int) bool {
	return persistedCount > limit
}
