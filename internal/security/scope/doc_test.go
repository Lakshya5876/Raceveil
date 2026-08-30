package scope

import "testing"

// TestGuardZeroValue proves the package compiles and is importable ahead of
// Phase 0's real enforcement logic (Design/SECURITY.md §3).
func TestGuardZeroValue(t *testing.T) {
	var g Guard
	_ = g
}
