package domain

// Workflow is an ordered sequence of Requests that together perform one
// logical state-changing operation, split into a Setup phase and the single
// Act Request that gets replicated concurrently (Design/DOMAIN.md
// §Workflow, §Act Request).
type Workflow struct {
	ID            string        `json:"id,omitempty"`
	Requests      []RequestSpec `yaml:"requests" json:"requests"`
	ActRequest    string        `yaml:"act_request" json:"act_request"`
	SetupRequests []string      `yaml:"setup_requests" json:"setup_requests"`
}

// RequestByID returns the Request with the given id, or false if none
// matches.
func (w Workflow) RequestByID(id string) (RequestSpec, bool) {
	for _, r := range w.Requests {
		if r.ID == id {
			return r, true
		}
	}
	return RequestSpec{}, false
}
