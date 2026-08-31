package cli

import (
	"github.com/spf13/cobra"

	"github.com/Lakshya5876/Raceveil/internal/domain"
	"github.com/Lakshya5876/Raceveil/internal/wiring"
)

func newScanCmd() *cobra.Command {
	var candidatePath, scopePath, authPath, outDir, format string
	cmd := &cobra.Command{
		Use:   "scan",
		Short: "Run a supplied Candidate (Workflow + Invariant) against its Scope — the MVP path (Design/API.md)",
		RunE: func(cmd *cobra.Command, _ []string) error {
			result, err := wiring.RunScan(cmd.Context(), wiring.ScanOptions{
				CandidatePath: candidatePath,
				ScopePath:     scopePath,
				AuthPath:      authPath,
				OutDir:        outDir,
			})
			if err != nil {
				return classifyRunError(err)
			}
			printScanResult(cmd.OutOrStdout(), result, format)
			return exitForResult(result)
		},
	}
	cmd.Flags().StringVar(&candidatePath, "candidate", "", "path to candidate.yaml (required, MVP path)")
	cmd.Flags().StringVar(&scopePath, "scope", "", "path to scope.yaml (required)")
	cmd.Flags().StringVar(&authPath, "auth", "", "path to session.json (required)")
	cmd.Flags().StringVar(&outDir, "out", "", "run directory (default ./raceveil-run-<timestamp>)")
	cmd.Flags().StringVar(&format, "format", "human", "output format: human|json")
	_ = cmd.MarkFlagRequired("candidate")
	_ = cmd.MarkFlagRequired("scope")
	_ = cmd.MarkFlagRequired("auth")
	return cmd
}

// classifyRunError maps a scan/verify error to its Design/API.md exit code:
// 5 for a scope/authorization refusal, 4 for anything else (including
// unprovable-under-cap, which is a refusal to proceed, not a crash, but has
// no dedicated code of its own in the API).
func classifyRunError(err error) error {
	if wiring.IsScopeRefusal(err) {
		return &exitCodeErr{code: 5, err: err}
	}
	return &exitCodeErr{code: 4, err: err}
}

// exitForResult maps a scan/verify Confidence to its Design/API.md exit
// code: 0 for no finding, 3 for SUSPECTED-only, 2 for LIKELY+.
func exitForResult(r wiring.ScanResult) error {
	if !r.Found {
		return nil
	}
	if r.Oracle.Confidence == domain.Suspected {
		return &exitCodeErr{code: 3}
	}
	return &exitCodeErr{code: 2}
}
