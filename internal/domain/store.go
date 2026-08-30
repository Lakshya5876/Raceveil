package domain

import "time"

// AuditEntry is one line of audit.jsonl: every outbound request, retained
// for accountability and post-hoc scope verification
// (Design/DATA_MODEL.md §10). It never contains bodies or secrets.
type AuditEntry struct {
	Timestamp    time.Time `json:"timestamp"`
	Method       string    `json:"method"`
	Host         string    `json:"host"`
	Path         string    `json:"path"`
	SessionGroup string    `json:"session_isolation_group"`
	ExperimentID string    `json:"experiment_id"`
	Status       int       `json:"status"`
	BytesSent    int       `json:"bytes_sent"`
	BytesRecv    int       `json:"bytes_recv"`
}

// RunStore is the persistence boundary the scheduler writes an Experiment's
// evidence through (Design/DATA_MODEL.md run directory layout). Implemented
// by internal/infrastructure/persist; test doubles for the scheduler mock
// this interface, never anything deeper (CLAUDE.md N3).
type RunStore interface {
	PersistRequests(requests []CapturedRequest) error
	PersistCandidate(candidate Candidate) error
	PersistBaseline(candidateID string, baseline Baseline) error
	PersistTrial(candidateID string, trial ConcurrentTrial) error
	PersistOracle(candidateID string, result OracleResult) error
	PersistFinding(finding Finding) error
	PersistAudit(entry AuditEntry) error
}

// FindingReader loads a self-contained .rv Reproduction bundle by path
// (Design/DOMAIN.md §Reproduction), used by the verify and replay commands.
type FindingReader interface {
	FetchFinding(path string) (Finding, error)
}
