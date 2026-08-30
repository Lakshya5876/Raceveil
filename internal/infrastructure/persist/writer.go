// Package persist writes RaceVeil's run directory to the filesystem:
// config.yaml, audit.jsonl, requests.jsonl, candidates.jsonl, per-experiment
// baseline/trials/oracle/minimization files, and findings/*.rv (Design/
// DATA_MODEL.md, Design/ARCHITECTURE.md §2.10). No database — filesystem
// only (Context/DECISIONS.md ADR-011).
package persist

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sync"

	"github.com/Lakshya5876/Raceveil/internal/domain"
	"gopkg.in/yaml.v3"
)

const (
	dirPerm  = 0o700
	filePerm = 0o600
)

// Writer implements domain.RunStore against a single run directory
// (Design/DATA_MODEL.md §1 on-disk layout). Safe for concurrent use — the
// scheduler runs the concurrency ladder and trial loop through it.
type Writer struct {
	runDir string
	mu     sync.Mutex
}

// NewWriter creates runDir (and its experiments/, findings/ subdirectories)
// with restrictive permissions (Design/SECURITY.md §9) and returns a Writer
// bound to it.
func NewWriter(runDir string) (*Writer, error) {
	for _, dir := range []string{runDir, filepath.Join(runDir, "experiments"), filepath.Join(runDir, "findings")} {
		if err := os.MkdirAll(dir, dirPerm); err != nil {
			return nil, fmt.Errorf("persist: create %s: %w", dir, err)
		}
	}
	return &Writer{runDir: runDir}, nil
}

// RunDir returns the run directory this Writer writes into.
func (w *Writer) RunDir() string { return w.runDir }

// WriteConfig writes the resolved, secret-free Scope to config.yaml
// (Design/DATA_MODEL.md §1). Scope carries no credentials, so no redaction
// is needed beyond never accepting a Session here.
func (w *Writer) WriteConfig(scope domain.Scope) error {
	w.mu.Lock()
	defer w.mu.Unlock()
	data, err := yaml.Marshal(scope)
	if err != nil {
		return fmt.Errorf("persist: marshal config.yaml: %w", err)
	}
	return os.WriteFile(filepath.Join(w.runDir, "config.yaml"), data, filePerm)
}

// PersistAudit appends one line to audit.jsonl (Design/DATA_MODEL.md §10).
func (w *Writer) PersistAudit(entry domain.AuditEntry) error {
	return w.appendJSONLine("audit.jsonl", entry)
}

// PersistRequests appends each CapturedRequest as one line of
// requests.jsonl (Design/DATA_MODEL.md §3).
func (w *Writer) PersistRequests(requests []domain.CapturedRequest) error {
	for _, r := range requests {
		if err := w.appendJSONLine("requests.jsonl", r); err != nil {
			return err
		}
	}
	return nil
}

// PersistCandidate appends one line to candidates.jsonl
// (Design/DATA_MODEL.md §4). Declared and inferred Candidates normalize to
// the same representation, so this is the only write path for either.
func (w *Writer) PersistCandidate(candidate domain.Candidate) error {
	return w.appendJSONLine("candidates.jsonl", candidate)
}

// PersistBaseline writes experiments/<id>/baseline.json
// (Design/DATA_MODEL.md §5).
func (w *Writer) PersistBaseline(candidateID string, baseline domain.Baseline) error {
	return w.writeExperimentJSON(candidateID, "baseline.json", baseline)
}

// PersistTrial appends one line to experiments/<id>/trials.jsonl
// (Design/DATA_MODEL.md §6).
func (w *Writer) PersistTrial(candidateID string, trial domain.ConcurrentTrial) error {
	id, err := sanitizeID(candidateID)
	if err != nil {
		return fmt.Errorf("persist: candidate id: %w", err)
	}
	dir := filepath.Join(w.runDir, "experiments", id)
	w.mu.Lock()
	defer w.mu.Unlock()
	if err := os.MkdirAll(dir, dirPerm); err != nil {
		return fmt.Errorf("persist: create %s: %w", dir, err)
	}
	return appendJSONLineLocked(filepath.Join(dir, "trials.jsonl"), trial)
}

// PersistOracle writes experiments/<id>/oracle.json (Design/DATA_MODEL.md §7).
func (w *Writer) PersistOracle(candidateID string, result domain.OracleResult) error {
	return w.writeExperimentJSON(candidateID, "oracle.json", result)
}

// PersistFinding writes findings/<id>.rv (Design/DATA_MODEL.md §9).
func (w *Writer) PersistFinding(finding domain.Finding) error {
	id, err := sanitizeID(finding.FindingID)
	if err != nil {
		return fmt.Errorf("persist: finding id: %w", err)
	}
	w.mu.Lock()
	defer w.mu.Unlock()
	data, err := json.MarshalIndent(finding, "", "  ")
	if err != nil {
		return fmt.Errorf("persist: marshal finding %s: %w", finding.FindingID, err)
	}
	path := filepath.Join(w.runDir, "findings", id+".rv")
	return os.WriteFile(path, data, filePerm)
}

// sanitizeID rejects a candidate/finding ID that could escape the run
// directory when joined into a path (defense in depth: these IDs are
// generated internally, but a path-traversal-shaped ID must never silently
// write outside runDir).
func sanitizeID(id string) (string, error) {
	if id == "" || id != filepath.Base(id) || id == "." || id == ".." {
		return "", fmt.Errorf("invalid id %q", id)
	}
	return id, nil
}

func (w *Writer) writeExperimentJSON(candidateID, filename string, v any) error {
	id, err := sanitizeID(candidateID)
	if err != nil {
		return fmt.Errorf("persist: candidate id: %w", err)
	}
	dir := filepath.Join(w.runDir, "experiments", id)
	w.mu.Lock()
	defer w.mu.Unlock()
	if err := os.MkdirAll(dir, dirPerm); err != nil {
		return fmt.Errorf("persist: create %s: %w", dir, err)
	}
	data, err := json.MarshalIndent(v, "", "  ")
	if err != nil {
		return fmt.Errorf("persist: marshal %s/%s: %w", candidateID, filename, err)
	}
	return os.WriteFile(filepath.Join(dir, filename), data, filePerm)
}

func (w *Writer) appendJSONLine(filename string, v any) error {
	w.mu.Lock()
	defer w.mu.Unlock()
	return appendJSONLineLocked(filepath.Join(w.runDir, filename), v)
}

// appendJSONLineLocked appends v as one JSON line. Callers must already
// hold whatever lock guards concurrent writes to path. path is always
// built by this package from w.runDir plus either a fixed filename
// constant or a sanitizeID-checked candidate ID — never raw external input.
func appendJSONLineLocked(path string, v any) error {
	data, err := json.Marshal(v)
	if err != nil {
		return fmt.Errorf("persist: marshal line for %s: %w", path, err)
	}
	f, err := os.OpenFile(path, os.O_APPEND|os.O_CREATE|os.O_WRONLY, filePerm) // #nosec G304 -- path built from runDir + fixed/sanitized components, not external input
	if err != nil {
		return fmt.Errorf("persist: open %s: %w", path, err)
	}
	defer func() { _ = f.Close() }()
	if _, err := f.Write(append(data, '\n')); err != nil {
		return fmt.Errorf("persist: append to %s: %w", path, err)
	}
	return nil
}

// FindingLoader implements domain.FindingReader by reading a self-contained
// .rv bundle directly from disk (Design/DOMAIN.md §Reproduction), used by
// the verify and replay commands.
type FindingLoader struct{}

// FetchFinding reads and parses the .rv JSON bundle at path. path is an
// operator-supplied CLI argument (raceveil verify/replay <finding.rv>) —
// reading an arbitrary local file the operator names is the command's
// intended behavior, the same trust boundary as `cat <path>`.
func (FindingLoader) FetchFinding(path string) (domain.Finding, error) {
	data, err := os.ReadFile(filepath.Clean(path)) // #nosec G304 -- operator-supplied CLI path, expected
	if err != nil {
		return domain.Finding{}, fmt.Errorf("persist: read %s: %w", path, err)
	}
	var f domain.Finding
	if err := json.Unmarshal(data, &f); err != nil {
		return domain.Finding{}, fmt.Errorf("persist: parse %s: %w", path, err)
	}
	return f, nil
}
