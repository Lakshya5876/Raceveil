package domain

// StateIndependence records whether a trial (or Experiment) can be treated
// as statistically independent for the Wilson reproduction calculation
// (Design/DOMAIN.md §State Independence). Only "independent" trials enter
// K_independent/r_violations.
type StateIndependence string

const (
	Independent         StateIndependence = "independent"
	Dependent           StateIndependence = "dependent"
	UnknownIndependence StateIndependence = "unknown"
)

// ResponsesSummary tallies classified responses within one ConcurrentTrial.
type ResponsesSummary struct {
	Success int `json:"success"`
	Reject  int `json:"reject"`
	Error   int `json:"error"`
}

// ConcurrentTrial is one synchronized burst of N Act-Request instances plus
// the collected Observables (Design/DOMAIN.md §ConcurrentTrial,
// Design/DATA_MODEL.md trials.jsonl).
type ConcurrentTrial struct {
	Trial                    int               `json:"trial"`
	N                        int               `json:"N"`
	SyncStrategy             string            `json:"sync_strategy"`
	InterarrivalDispersionMs float64           `json:"interarrival_dispersion_ms"`
	StateIndependence        StateIndependence `json:"state_independence"`
	ResponsesSummary         ResponsesSummary  `json:"responses_summary"`
	SuccessCountS            int               `json:"success_count_S"`
	Violation                bool              `json:"violation"`
	Observables              map[string]any    `json:"observables"`
}
