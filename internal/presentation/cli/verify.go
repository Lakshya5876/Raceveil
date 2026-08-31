package cli

import (
	"fmt"

	"github.com/spf13/cobra"

	"github.com/Lakshya5876/Raceveil/internal/wiring"
)

func newVerifyCmd() *cobra.Command {
	var scopePath, authPath, outDir, format string
	cmd := &cobra.Command{
		Use:   "verify <finding.rv>",
		Short: "Independently re-confirm a Finding on fresh trials (Design/API.md verify)",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			result, _, err := wiring.RunVerify(cmd.Context(), wiring.VerifyOptions{
				FindingPath: args[0],
				ScopePath:   scopePath,
				AuthPath:    authPath,
				OutDir:      outDir,
			})
			if err != nil {
				return classifyRunError(err)
			}
			if !result.Found {
				_, _ = fmt.Fprintln(cmd.OutOrStdout(), "verify: the finding did NOT re-reproduce on fresh trials.")
			}
			printScanResult(cmd.OutOrStdout(), result, format)
			return exitForResult(result)
		},
	}
	cmd.Flags().StringVar(&scopePath, "scope", "", "path to scope.yaml (required)")
	cmd.Flags().StringVar(&authPath, "auth", "", "path to session.json (required)")
	cmd.Flags().StringVar(&outDir, "out", "", "run directory (default ./raceveil-verify-<timestamp>)")
	cmd.Flags().StringVar(&format, "format", "human", "output format: human|json")
	_ = cmd.MarkFlagRequired("scope")
	_ = cmd.MarkFlagRequired("auth")
	return cmd
}
