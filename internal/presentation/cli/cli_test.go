package cli

import "testing"

func TestNewRootCmd_HasExpectedUse(t *testing.T) {
	cmd := newRootCmd()
	if cmd.Use != "raceveil" {
		t.Fatalf("Use = %q, want %q", cmd.Use, "raceveil")
	}
}
