// Package cli defines RaceVeil's Cobra command tree: scope, auth, scan,
// verify, replay, report, list (Design/API.md). Owns CLI parsing, output
// serialisation (human/json/sarif), and input validation; it must not
// contain business logic — it calls internal/wiring, never
// internal/application or internal/infrastructure directly (Guide §2.2,
// docs/SYSTEM_DESIGN.md §2).
package cli

import (
	"errors"
	"fmt"
	"os"

	"github.com/spf13/cobra"
)

func newRootCmd() *cobra.Command {
	root := &cobra.Command{
		Use:   "raceveil",
		Short: "RaceVeil verifies concurrency-integrity invariants on running web apps",
		Long: "RaceVeil is a black/grey-box tool that tests running web applications " +
			"for externally observable integrity violations caused by concurrent " +
			"requests, and produces reproducible, minimized evidence for each one.",
		SilenceUsage:  true,
		SilenceErrors: true,
	}
	root.AddCommand(newScanCmd(), newVerifyCmd(), newReplayCmd())
	return root
}

// exitCodeErr carries one of Design/API.md's stable exit codes through
// Cobra's error return path. err is nil when the code alone (e.g. a clean
// LIKELY+ classification) needs no additional message beyond what was
// already printed to stdout.
type exitCodeErr struct {
	code int
	err  error
}

func (e *exitCodeErr) Error() string {
	if e.err == nil {
		return fmt.Sprintf("exit %d", e.code)
	}
	return e.err.Error()
}

func (e *exitCodeErr) Unwrap() error { return e.err }

// Execute runs the RaceVeil CLI and returns the process exit code
// (Design/API.md: 0 no findings, 2 LIKELY+, 3 SUSPECTED only, 4 scan error,
// 5 scope/authorization refusal). It is the sole entrypoint called by
// cmd/raceveil/main.go.
func Execute() int {
	cmd := newRootCmd()
	if err := cmd.Execute(); err != nil {
		var ece *exitCodeErr
		if errors.As(err, &ece) {
			if ece.err != nil {
				fmt.Fprintln(os.Stderr, "Error:", ece.err)
			}
			return ece.code
		}
		fmt.Fprintln(os.Stderr, "Error:", err)
		return 4
	}
	return 0
}
