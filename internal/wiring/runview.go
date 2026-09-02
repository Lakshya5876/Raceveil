package wiring

import (
	"github.com/Lakshya5876/Raceveil/internal/domain"
	"github.com/Lakshya5876/Raceveil/internal/infrastructure/persist"
)

// RunView is presentation's read-only view of a completed run directory,
// the input to `raceveil report` and `raceveil list`. Presentation never
// touches internal/infrastructure directly — this is the wiring-owned
// shape it sees instead.
type RunView struct {
	RunDir     string                `json:"run_dir"`
	Candidates []domain.Candidate    `json:"candidates"`
	Oracles    []domain.OracleResult `json:"experiments"`
	Findings   []domain.Finding      `json:"findings"`
}

// LoadRunView reads a run directory produced by a previous scan or verify.
func LoadRunView(runDir string) (RunView, error) {
	view, err := persist.LoadRun(runDir)
	if err != nil {
		return RunView{}, err
	}
	return RunView{
		RunDir:     view.RunDir,
		Candidates: view.Candidates,
		Oracles:    view.Oracles,
		Findings:   view.Findings,
	}, nil
}

// OracleFor returns the Oracle evaluation behind a Finding, so a report can
// print the "why" alongside it. Falls back to a zero value when the run
// directory is partial.
func (v RunView) OracleFor(f domain.Finding) domain.OracleResult {
	want := "cand_" + trimPrefix(f.FindingID, "find_")
	for _, o := range v.Oracles {
		if o.CandidateID == want {
			return o
		}
	}
	return domain.OracleResult{}
}

// HighestConfidence returns the strongest Confidence in the run, or "" when
// there are no findings — the value `report` and `list` map to an exit code.
func (v RunView) HighestConfidence() domain.Confidence {
	best := domain.Confidence("")
	for _, f := range v.Findings {
		switch f.Confidence {
		case domain.Confirmed:
			return domain.Confirmed
		case domain.Likely:
			best = domain.Likely
		case domain.Suspected:
			if best == "" {
				best = domain.Suspected
			}
		}
	}
	return best
}

// SuspectedCount reports how many experiments produced a SUSPECTED signal,
// which reports carry as a note rather than as an asserted finding.
func (v RunView) SuspectedCount() int {
	n := 0
	for _, o := range v.Oracles {
		if o.Confidence == domain.Suspected {
			n++
		}
	}
	return n
}

func trimPrefix(s, prefix string) string {
	if len(s) >= len(prefix) && s[:len(prefix)] == prefix {
		return s[len(prefix):]
	}
	return s
}
