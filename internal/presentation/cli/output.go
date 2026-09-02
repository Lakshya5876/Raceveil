package cli

import (
	"encoding/json"
	"fmt"
	"io"
	"path/filepath"

	"github.com/Lakshya5876/Raceveil/internal/domain"
	"github.com/Lakshya5876/Raceveil/internal/wiring"
)

// printScanResult renders a scan/verify ScanResult per Design/UX.md: a
// finding (Expected vs Observed, reproduction stats, evidence, in that
// order), or the clean "no violations found" result. Certainty is never
// overstated — SUSPECTED is never printed as "vulnerable".
func printScanResult(w io.Writer, r wiring.ScanResult, format string) {
	if format == "json" {
		_ = json.NewEncoder(w).Encode(r)
		return
	}
	if !r.Found || r.Finding == nil {
		printClean(w)
		return
	}
	printFinding(w, *r.Finding, r.Oracle)
}

// printDiscoverySummary renders the v1 `scan <url>` result per
// Design/UX.md: discovery counts first, then each finding, then the
// skipped-with-no-invariant list — because showing the false-positive gate
// working is precisely why a reader can trust what did get reported.
func printDiscoverySummary(w io.Writer, s wiring.DiscoverySummary, format string) {
	if format == "json" {
		_ = json.NewEncoder(w).Encode(s)
		return
	}
	if format == "sarif" {
		findings := make([]domain.Finding, 0, len(s.Findings))
		for _, f := range s.Findings {
			if f.Finding != nil {
				findings = append(findings, *f.Finding)
			}
		}
		_ = writeSARIF(w, findings, countSuspected(s), Version)
		return
	}

	_, _ = fmt.Fprintf(w, "Discovery  endpoints:%d  workflows:%d  ranked:%d\n", s.Endpoints, s.Workflows, s.Ranked)
	_, _ = fmt.Fprintf(w, "Ranking    probed:%d  candidates with an established invariant:%d\n\n", s.Probed, s.CandidatesTested)

	for _, f := range s.Findings {
		if f.Finding != nil {
			printFinding(w, *f.Finding, f.Oracle)
			_, _ = fmt.Fprintln(w)
		}
	}

	if len(s.Findings) == 0 {
		_, _ = fmt.Fprintln(w, "No concurrency integrity violations found (LIKELY+).")
	}
	_, _ = fmt.Fprintf(w, "  %d experiment(s) run · %d finding(s) · %d endpoint(s) skipped (no invariant established)\n",
		s.CandidatesTested, len(s.Findings), len(s.SkippedNoLimit))
	for _, skipped := range s.SkippedNoLimit {
		_, _ = fmt.Fprintf(w, "    skipped  %s\n", skipped)
	}
	_, _ = fmt.Fprintf(w, "  run directory: %s\n", s.RunDir)
}

func countSuspected(s wiring.DiscoverySummary) int {
	n := 0
	for _, f := range s.Findings {
		if f.Oracle.Confidence == domain.Suspected {
			n++
		}
	}
	return n
}

func printClean(w io.Writer) {
	_, _ = fmt.Fprintln(w, "No concurrency integrity violations found (LIKELY+).")
	_, _ = fmt.Fprintln(w, "  1 experiment run · 0 violations")
	_, _ = fmt.Fprintln(w, "Exit code: 0")
}

// printFinding follows Design/UX.md's finding layout exactly: headline
// (severity + confidence), Invariant/Expected/Observed, Reproduction,
// Evidence, Confidence (with its rationale), Severity, then the two
// commands that let a reader falsify the result themselves.
func printFinding(w io.Writer, f domain.Finding, o domain.OracleResult) {
	label := "POSSIBLE CONCURRENCY INTEGRITY VIOLATION"
	if f.Confidence == domain.Confirmed {
		label = "CONCURRENCY INTEGRITY VIOLATION"
	}
	_, _ = fmt.Fprintf(w, "%-8s%-51s[%s]\n", f.Severity.Label, label, f.Confidence)
	_, _ = fmt.Fprintf(w, "%s\n\n", wiring.ActRequestLine(f.Workflow))

	_, _ = fmt.Fprintf(w, "  Invariant     %s = %d   (%s)\n", f.Invariant.Type, f.Invariant.Value, f.Invariant.Source)
	_, _ = fmt.Fprintf(w, "  Expected      %d successful effect(s)\n", f.Invariant.Value)
	_, _ = fmt.Fprintf(w, "  Observed      %d successful effects   under N=%d concurrent   (minimum proof for %s: %d effect(s), reached)\n\n",
		f.ObservedAtCreation.S, f.ConcurrencyN, f.Invariant.Type, f.RequiredProofEffects)

	rep := f.ObservedAtCreation.Reproduction
	_, _ = fmt.Fprintf(w, "  Reproduction  %d/%d fresh-state (%s) trials   Wilson95 lower bound %.3f\n",
		rep.RViolations, rep.KIndependent, f.StateIndependence, rep.Wilson95[0])
	_, _ = fmt.Fprintf(w, "  Evidence      primary  Level-3 cross-request: %d distinct successful effects observed\n", f.ObservedAtCreation.S)
	if len(o.CorroboratingObservables) == 0 {
		_, _ = fmt.Fprintln(w, "                (no Level-2/4 corroboration available for this candidate)")
	}
	for _, level := range f.Oracle.CorroboratingLevels {
		switch level {
		case 2:
			_, _ = fmt.Fprintln(w, "                corrob.  Level-2 body-differential: distinct effect signatures exceed the invariant")
		case 4:
			_, _ = fmt.Fprintln(w, "                corrob.  Level-4 post-state: persisted count exceeds the invariant")
		}
	}
	_, _ = fmt.Fprintf(w, "  Confidence    %s  (%s;\n                confidence bands %s — provisional, pending corpus calibration)\n",
		f.Confidence, o.Why, o.ConfidenceBandsVersion)
	_, _ = fmt.Fprintf(w, "  Severity      %s (advisory: %s)\n\n", f.Severity.Label, f.Severity.Basis)

	_, _ = fmt.Fprintf(w, "  Reproduce     raceveil replay %s\n", findingPath(f))
	_, _ = fmt.Fprintf(w, "  Re-verify     raceveil verify  %s\n", findingPath(f))
	_, _ = fmt.Fprintln(w, "Exit code: 2")
}

func findingPath(f domain.Finding) string {
	return filepath.Join(f.EvidenceRefs.RunDir, "findings", f.FindingID+".rv")
}
