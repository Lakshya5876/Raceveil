package domain

// MinReproStats freezes the reproduction rate observed at one concurrency
// step during minimization (Design/DATA_MODEL.md §8 minimization.json
// concurrency.min_repro).
type MinReproStats struct {
	RViolations  int     `json:"r_violations"`
	KIndependent int     `json:"K_independent"`
	WilsonLo     float64 `json:"wilson_lo"`
}

// ConcurrencyMinimization is the result of the decreasing-sweep search for
// the smallest N that still reliably reproduces a violation
// (Design/ARCHITECTURE.md §6, ADR-014).
type ConcurrencyMinimization struct {
	StartN   int           `json:"start_N"`
	MinimalN int           `json:"minimal_N"`
	MinRepro MinReproStats `json:"min_repro"`
}

// WorkflowMinimization is the result of greedily dropping Setup requests
// not required to still trigger the violation (Design/ARCHITECTURE.md §6).
type WorkflowMinimization struct {
	StartRequests   []string `json:"start_requests"`
	MinimalRequests []string `json:"minimal_requests"`
}

// Minimization is the smallest reproducible concurrent interaction that
// still violates the Invariant (Design/DATA_MODEL.md §8
// experiments/<id>/minimization.json).
type Minimization struct {
	Concurrency ConcurrencyMinimization `json:"concurrency"`
	Workflow    WorkflowMinimization    `json:"workflow"`
	Method      string                  `json:"method"`
}
