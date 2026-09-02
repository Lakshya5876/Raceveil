package fixtureserver

import (
	"os"
	"testing"
)

// TestServeStandalone starts the fixture and blocks forever, so a human (or
// an overnight demo script) can point a real raceveil.exe at it. It is a
// no-op under a normal `go test ./...` run — only RACEVEIL_SERVE_FIXTURE=1
// activates it, and it is meant to be killed externally
// (`RACEVEIL_SERVE_FIXTURE=1 go test -run TestServeStandalone -timeout 0 -v
// ./internal/testutil/fixtureserver &`, then kill the PID once done).
func TestServeStandalone(t *testing.T) {
	if os.Getenv("RACEVEIL_SERVE_FIXTURE") != "1" {
		t.Skip("set RACEVEIL_SERVE_FIXTURE=1 to run the fixture standalone for manual demo runs")
	}
	addr := os.Getenv("RACEVEIL_FIXTURE_ADDR")
	if addr == "" {
		addr = "127.0.0.1:18743"
	}
	srv := New(addr)
	if err := srv.Start(); err != nil {
		t.Fatalf("Start: %v", err)
	}
	t.Logf("fixture serving on %s (vulnerable: /issue-code,/redeem; safe: /issue-code-safe,/redeem-safe)", srv.Addr())
	select {}
}
