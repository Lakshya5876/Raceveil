package cli

import (
	"fmt"

	"github.com/spf13/cobra"

	"github.com/Lakshya5876/Raceveil/internal/wiring"
)

func newReplayCmd() *cobra.Command {
	var scopePath, authPath, outDir string
	var iAmAuthorized, noSafeMode bool
	cmd := &cobra.Command{
		Use:   "replay <finding.rv>",
		Short: "Re-run a Finding's exact recorded burst once, for demonstration (Design/API.md replay)",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			trial, finding, err := wiring.RunReplay(cmd.Context(), wiring.ReplayOptions{
				FindingPath:   args[0],
				ScopePath:     scopePath,
				AuthPath:      authPath,
				OutDir:        outDir,
				SafeMode:      !noSafeMode,
				IAmAuthorized: iAmAuthorized,
			})
			if err != nil {
				return classifyRunError(err)
			}
			w := cmd.OutOrStdout()
			_, _ = fmt.Fprintf(w, "%s\n\n", wiring.ActRequestLine(finding.Workflow))
			_, _ = fmt.Fprintf(w, "  Expected      max_successes = %d\n", finding.Invariant.Value)
			_, _ = fmt.Fprintf(w, "  Observed      %d successful effects under N=%d (%s)\n", trial.SuccessCountS, trial.N, trial.SyncStrategy)
			if trial.Violation {
				_, _ = fmt.Fprintln(w, "  Result        replayed — the invariant was broken again")
			} else {
				_, _ = fmt.Fprintln(w, "  Result        replayed — the invariant held this time (replay does not recompute Confidence)")
			}
			_, _ = fmt.Fprintln(w, "Exit code: 0")
			return nil
		},
	}
	cmd.Flags().StringVar(&scopePath, "scope", "", "path to scope.yaml (required)")
	cmd.Flags().StringVar(&authPath, "auth", "", "path to session.json (required)")
	cmd.Flags().StringVar(&outDir, "out", "", "run directory (default ./raceveil-replay-<timestamp>)")
	cmd.Flags().BoolVar(&iAmAuthorized, "i-am-authorized", false, "required to test a target resolving to a public IP")
	cmd.Flags().BoolVar(&noSafeMode, "no-safe-mode", false, "relax safe-mode; still requires --i-am-authorized")
	_ = cmd.MarkFlagRequired("scope")
	_ = cmd.MarkFlagRequired("auth")
	return cmd
}
