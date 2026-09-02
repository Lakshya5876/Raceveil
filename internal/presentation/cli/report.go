package cli

import (
	"encoding/json"
	"fmt"
	"io"

	"github.com/spf13/cobra"

	"github.com/Lakshya5876/Raceveil/internal/wiring"
)

// Version is the RaceVeil release identifier stamped into reports and
// SARIF output. Overridable at build time with
// -ldflags "-X .../internal/presentation/cli.Version=v1.2.3".
var Version = "1.0.0"

func newReportCmd() *cobra.Command {
	var format string
	cmd := &cobra.Command{
		Use:   "report <run-dir>",
		Short: "Render the findings of a completed run (human|json|sarif|md)",
		Long: "report is a derived, regenerable view over an existing run directory — it adds no\n" +
			"new data and sends no traffic. Only LIKELY+ findings are emitted as results;\n" +
			"SUSPECTED signals are reported separately and never asserted as vulnerabilities.",
		Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			view, err := wiring.LoadRunView(args[0])
			if err != nil {
				return &exitCodeErr{code: 4, err: err}
			}
			if err := renderReport(cmd.OutOrStdout(), view, format); err != nil {
				return &exitCodeErr{code: 4, err: err}
			}
			return exitForConfidence(view.HighestConfidence())
		},
	}
	cmd.Flags().StringVar(&format, "format", "human", "output format: human|json|sarif|md")
	return cmd
}

func renderReport(w io.Writer, view wiring.RunView, format string) error {
	switch format {
	case "json":
		enc := json.NewEncoder(w)
		enc.SetIndent("", "  ")
		return enc.Encode(view)
	case "sarif":
		return writeSARIF(w, view.Findings, view.SuspectedCount(), Version)
	case "md":
		return renderMarkdown(w, view)
	case "human", "":
		renderHuman(w, view)
		return nil
	default:
		return fmt.Errorf("unknown --format %q (want human|json|sarif|md)", format)
	}
}

func renderHuman(w io.Writer, view wiring.RunView) {
	_, _ = fmt.Fprintf(w, "Run: %s\n", view.RunDir)
	_, _ = fmt.Fprintf(w, "  %d candidate(s) · %d experiment(s) · %d finding(s)\n\n",
		len(view.Candidates), len(view.Oracles), len(view.Findings))
	if len(view.Findings) == 0 {
		_, _ = fmt.Fprintln(w, "No concurrency integrity violations found (LIKELY+).")
		return
	}
	for _, f := range view.Findings {
		printFinding(w, f, view.OracleFor(f))
		_, _ = fmt.Fprintln(w)
	}
}

// renderMarkdown produces a ticket/PR-ready report (Design/UX.md report
// artifacts: "the same content as Markdown, for tickets/PRs").
func renderMarkdown(w io.Writer, view wiring.RunView) error {
	_, _ = fmt.Fprintf(w, "# RaceVeil report\n\n")
	_, _ = fmt.Fprintf(w, "- **Run directory:** `%s`\n", view.RunDir)
	_, _ = fmt.Fprintf(w, "- **Candidates:** %d · **Experiments:** %d · **Findings:** %d\n",
		len(view.Candidates), len(view.Oracles), len(view.Findings))
	if len(view.Findings) > 0 {
		_, _ = fmt.Fprintf(w, "- **Authorized by (operator-declared, not verified by RaceVeil):** %s\n",
			view.Findings[0].ScopeRef.AuthorizedBy)
	}
	_, _ = fmt.Fprintln(w)

	if len(view.Findings) == 0 {
		_, _ = fmt.Fprintln(w, "No concurrency integrity violations found at LIKELY or above.")
		_, _ = fmt.Fprintln(w)
		_, _ = fmt.Fprintln(w, "> A clean result is a scoped claim: no LIKELY+ violation was found in the tested")
		_, _ = fmt.Fprintln(w, "> workflows. It is not a claim that no races exist.")
		return nil
	}

	_, _ = fmt.Fprintln(w, "| Endpoint | Invariant | Expected | Observed | N | Reproduction | Confidence | Severity |")
	_, _ = fmt.Fprintln(w, "|---|---|---|---|---|---|---|---|")
	for _, f := range view.Findings {
		rep := f.ObservedAtCreation.Reproduction
		_, _ = fmt.Fprintf(w, "| `%s` | %s=%d | %d | %d | %d | %d/%d (Wilson95 lo %.3f) | **%s** | %s |\n",
			wiring.ActRequestLine(f.Workflow), f.Invariant.Type, f.Invariant.Value,
			f.Invariant.Value, f.ObservedAtCreation.S, f.ConcurrencyN,
			rep.RViolations, rep.KIndependent, rep.Wilson95[0], f.Confidence, f.Severity.Label)
	}
	_, _ = fmt.Fprintln(w)

	for _, f := range view.Findings {
		_, _ = fmt.Fprintf(w, "## %s — %s\n\n", wiring.ActRequestLine(f.Workflow), f.Confidence)
		_, _ = fmt.Fprintf(w, "%s\n\n", view.OracleFor(f).Why)
		_, _ = fmt.Fprintf(w, "- **Invariant:** `%s = %d` (%s)\n", f.Invariant.Type, f.Invariant.Value, f.Invariant.Source)
		_, _ = fmt.Fprintf(w, "- **Expected:** %d successful effect(s); **observed:** %d under N=%d concurrent\n",
			f.Invariant.Value, f.ObservedAtCreation.S, f.ConcurrencyN)
		_, _ = fmt.Fprintf(w, "- **State independence:** %s\n", f.StateIndependence)
		_, _ = fmt.Fprintf(w, "- **Corroborating oracle levels:** %v (primary: %v)\n",
			f.Oracle.CorroboratingLevels, f.Oracle.PrimaryLevels)
		_, _ = fmt.Fprintf(w, "- **Confidence bands:** %s (provisional, pending corpus calibration)\n",
			f.ObservedAtCreation.ConfidenceBandsVersion)
		_, _ = fmt.Fprintf(w, "- **Severity:** %s — %s (advisory)\n\n", f.Severity.Label, f.Severity.Basis)
		_, _ = fmt.Fprintf(w, "```bash\nraceveil replay %s\nraceveil verify %s\n```\n\n", findingPath(f), findingPath(f))
	}
	return nil
}

func newListCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "list <run-dir>",
		Short: "List the candidates, experiments, and findings in a run directory",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			view, err := wiring.LoadRunView(args[0])
			if err != nil {
				return &exitCodeErr{code: 4, err: err}
			}
			w := cmd.OutOrStdout()
			_, _ = fmt.Fprintf(w, "Run: %s\n\nCANDIDATES (%d)\n", view.RunDir, len(view.Candidates))
			for _, c := range view.Candidates {
				score := "n/a (declared)"
				if c.Score != nil {
					score = fmt.Sprintf("%.2f", *c.Score)
				}
				_, _ = fmt.Fprintf(w, "  %-24s %-9s score=%-14s %s=%d\n",
					c.ID, c.Source, score, c.Invariant.Type, c.Invariant.Value)
			}
			_, _ = fmt.Fprintf(w, "\nEXPERIMENTS (%d)\n", len(view.Oracles))
			for _, o := range view.Oracles {
				confidence := string(o.Confidence)
				if confidence == "" {
					confidence = "no violation"
				}
				_, _ = fmt.Fprintf(w, "  %-24s %-10s r=%d/K=%d  wilson95lo=%.3f  %s\n",
					o.CandidateID, confidence, o.Reproduction.RViolations, o.Reproduction.KIndependent,
					o.Reproduction.Wilson95[0], o.StateIndependence)
			}
			_, _ = fmt.Fprintf(w, "\nFINDINGS (%d)\n", len(view.Findings))
			for _, f := range view.Findings {
				_, _ = fmt.Fprintf(w, "  %-24s %-10s %-8s %s\n",
					f.FindingID, f.Confidence, f.Severity.Label, wiring.ActRequestLine(f.Workflow))
			}
			return exitForConfidence(view.HighestConfidence())
		},
	}
	return cmd
}

func newVersionCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "version",
		Short: "Print the RaceVeil version",
		RunE: func(cmd *cobra.Command, _ []string) error {
			_, _ = fmt.Fprintf(cmd.OutOrStdout(), "raceveil %s\n", Version)
			return nil
		},
	}
}
