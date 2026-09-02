package cli

import (
	"fmt"

	"github.com/spf13/cobra"

	"github.com/Lakshya5876/Raceveil/internal/domain"
	"github.com/Lakshya5876/Raceveil/internal/wiring"
)

func newScanCmd() *cobra.Command {
	var candidatePath, scopePath, authPath, outDir, format string
	var importPath, invariantsPath, discoveryMode string
	var maxCandidates int
	var iAmAuthorized, noSafeMode bool

	cmd := &cobra.Command{
		Use:   "scan [url]",
		Short: "Test a target for concurrency-integrity violations (Design/API.md)",
		Long: "scan has two mutually exclusive input modes:\n" +
			"  --candidate candidate.yaml   the MVP path: run one operator-supplied Candidate, no discovery\n" +
			"  <url>                        the v1 path: crawl/import, rank, infer invariants, then experiment\n\n" +
			"Everything after the input stage (baseline -> concurrent trials -> oracle -> verify ->\n" +
			"minimize -> .rv) is identical between the two.",
		Args: cobra.MaximumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			target := ""
			if len(args) == 1 {
				target = args[0]
			}
			if (target == "") == (candidatePath == "") {
				return &exitCodeErr{code: 4, err: fmt.Errorf(
					"provide exactly one input: either a <url> to discover, or --candidate candidate.yaml")}
			}

			safeMode := !noSafeMode
			if candidatePath != "" {
				return runCandidateScan(cmd, wiring.ScanOptions{
					CandidatePath: candidatePath, ScopePath: scopePath, AuthPath: authPath,
					OutDir: outDir, SafeMode: safeMode, IAmAuthorized: iAmAuthorized,
				}, format)
			}
			return runDiscoveryScan(cmd, wiring.DiscoverOptions{
				Target: target, ScopePath: scopePath, AuthPath: authPath, OutDir: outDir,
				ImportPath: importPath, InvariantsRef: invariantsPath, Mode: discoveryMode,
				MaxCandidates: maxCandidates, SafeMode: safeMode, IAmAuthorized: iAmAuthorized,
			}, format)
		},
	}

	cmd.Flags().StringVar(&candidatePath, "candidate", "", "path to candidate.yaml (MVP path; mutually exclusive with <url>)")
	cmd.Flags().StringVar(&scopePath, "scope", "", "path to scope.yaml (required)")
	cmd.Flags().StringVar(&authPath, "auth", "", "path to session.json (required)")
	cmd.Flags().StringVar(&outDir, "out", "", "run directory (default ./raceveil-run-<timestamp>)")
	cmd.Flags().StringVar(&format, "format", "human", "output format: human|json|sarif")
	cmd.Flags().StringVar(&importPath, "import", "", "HAR or OpenAPI document to import instead of/with crawling")
	cmd.Flags().StringVar(&invariantsPath, "invariants", "", "invariants.yaml declaring Oracle Level 5 invariants (discovery path only)")
	cmd.Flags().StringVar(&discoveryMode, "discovery", "crawl", "discovery source: crawl|import|both")
	cmd.Flags().IntVar(&maxCandidates, "max-candidates", 10, "maximum ranked candidates to probe and test")
	cmd.Flags().BoolVar(&iAmAuthorized, "i-am-authorized", false, "required to test a target resolving to a public IP")
	cmd.Flags().BoolVar(&noSafeMode, "no-safe-mode", false, "relax safe-mode; still requires --i-am-authorized")
	_ = cmd.MarkFlagRequired("scope")
	_ = cmd.MarkFlagRequired("auth")
	return cmd
}

func runCandidateScan(cmd *cobra.Command, opts wiring.ScanOptions, format string) error {
	result, err := wiring.RunScan(cmd.Context(), opts)
	if err != nil {
		return classifyRunError(err)
	}
	printScanResult(cmd.OutOrStdout(), result, format)
	return exitForResult(result)
}

func runDiscoveryScan(cmd *cobra.Command, opts wiring.DiscoverOptions, format string) error {
	summary, err := wiring.RunDiscoverScan(cmd.Context(), opts)
	if err != nil {
		return classifyRunError(err)
	}
	printDiscoverySummary(cmd.OutOrStdout(), summary, format)
	return exitForConfidence(summary.HighestConfidence())
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

// exitForResult maps a candidate-scan result to its Design/API.md exit code.
func exitForResult(r wiring.ScanResult) error {
	if !r.Found {
		return nil
	}
	return exitForConfidence(r.Oracle.Confidence)
}

// exitForConfidence maps a Confidence to its Design/API.md exit code:
// 0 no findings, 3 SUSPECTED only, 2 LIKELY+.
func exitForConfidence(c domain.Confidence) error {
	switch c {
	case "":
		return nil
	case domain.Suspected:
		return &exitCodeErr{code: 3}
	default:
		return &exitCodeErr{code: 2}
	}
}
