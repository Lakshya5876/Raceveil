// Package cli defines RaceVeil's Cobra command tree: scope, auth, scan,
// verify, replay, report, list (Design/API.md). Owns CLI parsing, output
// serialisation (human/json/sarif), and input validation; it must not
// contain business logic or call internal/infrastructure directly — it
// calls internal/application via internal/wiring (Guide §2.2, docs/
// SYSTEM_DESIGN.md §2).
//
// Subcommands are added phase-by-phase per Context/ROADMAP.md; this file
// wires only the root command ahead of that work.
package cli

import "github.com/spf13/cobra"

func newRootCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "raceveil",
		Short: "RaceVeil verifies concurrency-integrity invariants on running web apps",
		Long: "RaceVeil is a black/grey-box tool that tests running web applications " +
			"for externally observable integrity violations caused by concurrent " +
			"requests, and produces reproducible, minimized evidence for each one.",
	}
}

// Execute runs the RaceVeil CLI. It is the sole entrypoint called by
// cmd/raceveil/main.go.
func Execute() error {
	return newRootCmd().Execute()
}
