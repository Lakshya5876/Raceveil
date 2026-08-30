// Package config is RaceVeil's single point of environment access (Guide
// §2.2.1 security invariant: config/env access only through this module —
// never raw os.Getenv in feature code). CORE_FILES.
package config

import "os"

// Lookup returns the environment variable named by key, or def if it is
// unset. This is the only place in the codebase permitted to call
// os.LookupEnv directly.
func Lookup(key, def string) string {
	if v, ok := os.LookupEnv(key); ok {
		return v
	}
	return def
}
