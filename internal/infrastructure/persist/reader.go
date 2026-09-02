package persist

import (
	"bufio"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/Lakshya5876/Raceveil/internal/domain"
)

// RunView is a read-only projection of a completed run directory, the input
// to `raceveil report` and `raceveil list` (Design/DATA_MODEL.md §11:
// reports are a derived, regenerable view over findings — no new data).
type RunView struct {
	RunDir     string
	Candidates []domain.Candidate
	Oracles    []domain.OracleResult
	Findings   []domain.Finding
}

// LoadRun reads a run directory produced by a previous scan/verify.
func LoadRun(runDir string) (RunView, error) {
	view := RunView{RunDir: runDir}
	info, err := os.Stat(runDir)
	if err != nil {
		return view, fmt.Errorf("persist: read run dir %s: %w", runDir, err)
	}
	if !info.IsDir() {
		return view, fmt.Errorf("persist: %s is not a run directory", runDir)
	}

	if view.Candidates, err = readCandidates(filepath.Join(runDir, "candidates.jsonl")); err != nil {
		return view, err
	}
	if view.Findings, err = readFindings(filepath.Join(runDir, "findings")); err != nil {
		return view, err
	}
	if view.Oracles, err = readOracles(filepath.Join(runDir, "experiments")); err != nil {
		return view, err
	}
	return view, nil
}

func readCandidates(path string) ([]domain.Candidate, error) {
	f, err := os.Open(filepath.Clean(path)) // #nosec G304 -- path derived from the operator-supplied run dir
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil // a run that never got as far as writing candidates is still readable
		}
		return nil, fmt.Errorf("persist: read candidates.jsonl: %w", err)
	}
	defer func() { _ = f.Close() }()

	var out []domain.Candidate
	sc := bufio.NewScanner(f)
	sc.Buffer(make([]byte, 0, 64*1024), 4*1024*1024)
	for sc.Scan() {
		line := strings.TrimSpace(sc.Text())
		if line == "" {
			continue
		}
		var c domain.Candidate
		if err := json.Unmarshal([]byte(line), &c); err != nil {
			return nil, fmt.Errorf("persist: malformed candidates.jsonl line: %w", err)
		}
		out = append(out, c)
	}
	return out, sc.Err()
}

func readFindings(dir string) ([]domain.Finding, error) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, fmt.Errorf("persist: read findings dir: %w", err)
	}
	var out []domain.Finding
	for _, e := range entries {
		if e.IsDir() || !strings.HasSuffix(e.Name(), ".rv") {
			continue
		}
		f, err := (FindingLoader{}).FetchFinding(filepath.Join(dir, e.Name()))
		if err != nil {
			return nil, err
		}
		out = append(out, f)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].FindingID < out[j].FindingID })
	return out, nil
}

func readOracles(experimentsDir string) ([]domain.OracleResult, error) {
	entries, err := os.ReadDir(experimentsDir)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, fmt.Errorf("persist: read experiments dir: %w", err)
	}
	var out []domain.OracleResult
	for _, e := range entries {
		if !e.IsDir() {
			continue
		}
		path := filepath.Join(experimentsDir, e.Name(), "oracle.json")
		data, err := os.ReadFile(filepath.Clean(path)) // #nosec G304 -- derived from the operator-supplied run dir
		if err != nil {
			if os.IsNotExist(err) {
				continue // an experiment that never reached the Oracle is not an error
			}
			return nil, fmt.Errorf("persist: read %s: %w", path, err)
		}
		var result domain.OracleResult
		if err := json.Unmarshal(data, &result); err != nil {
			return nil, fmt.Errorf("persist: parse %s: %w", path, err)
		}
		out = append(out, result)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].CandidateID < out[j].CandidateID })
	return out, nil
}
