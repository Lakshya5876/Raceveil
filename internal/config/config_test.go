package config

import "testing"

func TestLookup_ReturnsDefaultWhenUnset(t *testing.T) {
	got := Lookup("RACEVEIL_DOES_NOT_EXIST_XYZ", "fallback")
	if got != "fallback" {
		t.Fatalf("Lookup() = %q, want %q", got, "fallback")
	}
}

func TestLookup_ReturnsSetValue(t *testing.T) {
	t.Setenv("RACEVEIL_TEST_VAR", "value")
	got := Lookup("RACEVEIL_TEST_VAR", "fallback")
	if got != "value" {
		t.Fatalf("Lookup() = %q, want %q", got, "value")
	}
}
