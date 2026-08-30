package persist

import (
	"bufio"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/Lakshya5876/Raceveil/internal/domain"
)

var (
	_ domain.RunStore      = (*Writer)(nil)
	_ domain.FindingReader = FindingLoader{}
)

func newTestWriter(t *testing.T) *Writer {
	t.Helper()
	w, err := NewWriter(filepath.Join(t.TempDir(), "run"))
	if err != nil {
		t.Fatalf("NewWriter: %v", err)
	}
	return w
}

func readLines(t *testing.T, path string) []string {
	t.Helper()
	f, err := os.Open(path)
	if err != nil {
		t.Fatalf("open %s: %v", path, err)
	}
	defer func() { _ = f.Close() }()
	var lines []string
	sc := bufio.NewScanner(f)
	for sc.Scan() {
		lines = append(lines, sc.Text())
	}
	return lines
}

func TestNewWriter_CreatesDirectoryLayout(t *testing.T) {
	w := newTestWriter(t)
	for _, dir := range []string{w.RunDir(), filepath.Join(w.RunDir(), "experiments"), filepath.Join(w.RunDir(), "findings")} {
		if info, err := os.Stat(dir); err != nil || !info.IsDir() {
			t.Errorf("expected directory %s to exist, err=%v", dir, err)
		}
	}
}

func TestWriteConfig_WritesYAML(t *testing.T) {
	w := newTestWriter(t)
	scope := domain.Scope{Target: "http://127.0.0.1:18743", AuthorizedBy: "test"}
	if err := w.WriteConfig(scope); err != nil {
		t.Fatalf("WriteConfig: %v", err)
	}
	data, err := os.ReadFile(filepath.Join(w.RunDir(), "config.yaml"))
	if err != nil {
		t.Fatalf("read config.yaml: %v", err)
	}
	if len(data) == 0 {
		t.Fatal("config.yaml is empty")
	}
}

func TestPersistRequests_AppendsJSONL(t *testing.T) {
	w := newTestWriter(t)
	reqs := []domain.CapturedRequest{
		{ID: "req_1", Endpoint: domain.Endpoint{Method: "POST", Path: "/issue-code"}},
		{ID: "req_2", Endpoint: domain.Endpoint{Method: "POST", Path: "/redeem"}},
	}
	if err := w.PersistRequests(reqs); err != nil {
		t.Fatalf("PersistRequests: %v", err)
	}
	lines := readLines(t, filepath.Join(w.RunDir(), "requests.jsonl"))
	if len(lines) != 2 {
		t.Fatalf("got %d lines, want 2", len(lines))
	}
	var got domain.CapturedRequest
	if err := json.Unmarshal([]byte(lines[0]), &got); err != nil {
		t.Fatalf("unmarshal line 0: %v", err)
	}
	if got.ID != "req_1" {
		t.Errorf("line 0 ID = %q, want req_1", got.ID)
	}
}

func TestPersistCandidate_AppendsJSONL(t *testing.T) {
	w := newTestWriter(t)
	c := domain.Candidate{ID: "cand_redeem", Source: "declared", Score: nil,
		Invariant: domain.Invariant{Type: domain.InvariantMaxSuccesses, Value: 1}}
	if err := w.PersistCandidate(c); err != nil {
		t.Fatalf("PersistCandidate: %v", err)
	}
	lines := readLines(t, filepath.Join(w.RunDir(), "candidates.jsonl"))
	if len(lines) != 1 {
		t.Fatalf("got %d lines, want 1", len(lines))
	}
	if want := `"source":"declared"`; !strings.Contains(lines[0], want) {
		t.Errorf("candidates.jsonl line missing %q: %s", want, lines[0])
	}
	if want := `"score":null`; !strings.Contains(lines[0], want) {
		t.Errorf("declared candidate should serialize score as null: %s", lines[0])
	}
}

func TestPersistBaselineAndOracle_WriteExperimentJSON(t *testing.T) {
	w := newTestWriter(t)
	baseline := domain.Baseline{CandidateID: "cand_redeem", Runs: 3, ExpectedSuccessCount: 1}
	if err := w.PersistBaseline("cand_redeem", baseline); err != nil {
		t.Fatalf("PersistBaseline: %v", err)
	}
	oracle := domain.OracleResult{CandidateID: "cand_redeem", Confidence: domain.Likely}
	if err := w.PersistOracle("cand_redeem", oracle); err != nil {
		t.Fatalf("PersistOracle: %v", err)
	}
	dir := filepath.Join(w.RunDir(), "experiments", "cand_redeem")
	for _, f := range []string{"baseline.json", "oracle.json"} {
		if _, err := os.Stat(filepath.Join(dir, f)); err != nil {
			t.Errorf("expected %s to exist: %v", f, err)
		}
	}
}

func TestPersistTrial_AppendsPerCandidateJSONL(t *testing.T) {
	w := newTestWriter(t)
	for i := 1; i <= 3; i++ {
		trial := domain.ConcurrentTrial{Trial: i, N: 2, StateIndependence: domain.Independent}
		if err := w.PersistTrial("cand_redeem", trial); err != nil {
			t.Fatalf("PersistTrial %d: %v", i, err)
		}
	}
	lines := readLines(t, filepath.Join(w.RunDir(), "experiments", "cand_redeem", "trials.jsonl"))
	if len(lines) != 3 {
		t.Fatalf("got %d trial lines, want 3", len(lines))
	}
}

func TestPersistFinding_WritesRVFile(t *testing.T) {
	w := newTestWriter(t)
	finding := domain.Finding{RVVersion: "1", FindingID: "find_redeem_01", Confidence: domain.Likely}
	if err := w.PersistFinding(finding); err != nil {
		t.Fatalf("PersistFinding: %v", err)
	}
	path := filepath.Join(w.RunDir(), "findings", "find_redeem_01.rv")
	if _, err := os.Stat(path); err != nil {
		t.Fatalf("expected .rv file to exist: %v", err)
	}
	loaded, err := (FindingLoader{}).FetchFinding(path)
	if err != nil {
		t.Fatalf("FetchFinding: %v", err)
	}
	if loaded.FindingID != finding.FindingID {
		t.Errorf("FetchFinding round-trip FindingID = %q, want %q", loaded.FindingID, finding.FindingID)
	}
	if loaded.Confidence != domain.Likely {
		t.Errorf("FetchFinding round-trip Confidence = %q, want LIKELY", loaded.Confidence)
	}
}

func TestPersistAudit_NeverContainsSecretFields(t *testing.T) {
	w := newTestWriter(t)
	entry := domain.AuditEntry{Method: "POST", Host: "127.0.0.1", Path: "/redeem", Status: 200}
	if err := w.PersistAudit(entry); err != nil {
		t.Fatalf("PersistAudit: %v", err)
	}
	lines := readLines(t, filepath.Join(w.RunDir(), "audit.jsonl"))
	if len(lines) != 1 {
		t.Fatalf("got %d lines, want 1", len(lines))
	}
}
