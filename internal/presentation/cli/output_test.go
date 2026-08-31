package cli

import (
	"bytes"
	"strings"
	"testing"

	"github.com/Lakshya5876/Raceveil/internal/domain"
	"github.com/Lakshya5876/Raceveil/internal/wiring"
)

func TestPrintClean(t *testing.T) {
	var buf bytes.Buffer
	printClean(&buf)
	out := buf.String()
	if !strings.Contains(out, "No concurrency integrity violations found") {
		t.Errorf("clean output missing expected headline: %q", out)
	}
	if !strings.Contains(out, "Exit code: 0") {
		t.Errorf("clean output missing exit code line: %q", out)
	}
}

func sampleFinding() domain.Finding {
	return domain.Finding{
		FindingID: "find_redeem",
		Workflow: domain.Workflow{
			ActRequest: "redeem",
			Requests:   []domain.RequestSpec{{ID: "redeem", Method: "POST", URL: "/redeem"}},
		},
		Invariant:            domain.Invariant{Value: 1, Source: "declared"},
		RequiredProofEffects: 2,
		ConcurrencyN:         2,
		StateIndependence:    domain.Independent,
		ObservedAtCreation: domain.ObservedAtCreation{
			S: 2,
			Reproduction: domain.ReproductionStats{
				KIndependent: 6, RViolations: 5, Wilson95: [2]float64{0.436, 0.970},
			},
			ConfidenceBandsVersion: "provisional-v0",
		},
		Confidence:   domain.Likely,
		Severity:     domain.Severity{Label: "HIGH", Basis: "duplicate-effect integrity violation"},
		EvidenceRefs: domain.EvidenceRefs{RunDir: "raceveil-run-test"},
	}
}

func TestPrintFinding_ContainsUXRequiredFields(t *testing.T) {
	var buf bytes.Buffer
	f := sampleFinding()
	o := domain.OracleResult{Why: "S>1 in 5/6 independent trials", ConfidenceBandsVersion: "provisional-v0"}
	printFinding(&buf, f, o)
	out := buf.String()

	for _, want := range []string{
		"POST /redeem",
		"[LIKELY]",
		"Invariant     max_successes = 1",
		"Observed      2 successful effects",
		"Reproduction  5/6 fresh-state (independent) trials",
		"Confidence    LIKELY",
		"Severity      HIGH",
		"raceveil replay raceveil-run-test",
		"raceveil verify",
		"Exit code: 2",
	} {
		if !strings.Contains(out, want) {
			t.Errorf("finding output missing %q\nfull output:\n%s", want, out)
		}
	}
	if strings.Contains(out, "CONFIRMED") {
		t.Error("LIKELY finding must never print CONFIRMED anywhere")
	}
}

func TestExitForResult(t *testing.T) {
	cases := []struct {
		name string
		r    wiring.ScanResult
		want int
	}{
		{"no finding", wiring.ScanResult{Found: false}, 0},
		{"suspected", wiring.ScanResult{Found: true, Oracle: domain.OracleResult{Confidence: domain.Suspected}}, 3},
		{"likely", wiring.ScanResult{Found: true, Oracle: domain.OracleResult{Confidence: domain.Likely}}, 2},
		{"confirmed", wiring.ScanResult{Found: true, Oracle: domain.OracleResult{Confidence: domain.Confirmed}}, 2},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			err := exitForResult(c.r)
			if c.want == 0 {
				if err != nil {
					t.Errorf("expected nil error (exit 0), got %v", err)
				}
				return
			}
			var ece *exitCodeErr
			if err == nil {
				t.Fatal("expected a non-nil exitCodeErr")
			}
			ece, ok := err.(*exitCodeErr)
			if !ok {
				t.Fatalf("expected *exitCodeErr, got %T", err)
			}
			if ece.code != c.want {
				t.Errorf("exit code = %d, want %d", ece.code, c.want)
			}
		})
	}
}
