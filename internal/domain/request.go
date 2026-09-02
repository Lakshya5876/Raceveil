package domain

// Binding resolves a variable at send time — a session value, a literal, or
// a value extracted from a prior response (Design/DATA_MODEL.md
// candidate.yaml bindings, requests.jsonl bindings).
type Binding struct {
	Name string `yaml:"name" json:"name"`
	In   string `yaml:"in" json:"in"` // "header" | "body"
	From string `yaml:"from" json:"from"`
}

// RequestSpec is one Request in a Workflow: method, URL, headers, body, and
// its variable Bindings (Design/DOMAIN.md §Request).
type RequestSpec struct {
	ID       string            `yaml:"id" json:"id"`
	Method   string            `yaml:"method" json:"method"`
	URL      string            `yaml:"url" json:"url"`
	Headers  map[string]string `yaml:"headers" json:"headers"`
	Body     string            `yaml:"body" json:"body"`
	Bindings []Binding         `yaml:"bindings" json:"bindings"`
}

// CapturedRequest is one line of requests.jsonl (Design/DATA_MODEL.md §3):
// a Request as recorded in the run directory, independent of which
// Candidate/Workflow references it.
type CapturedRequest struct {
	ID              string            `json:"id"`
	Endpoint        Endpoint          `json:"endpoint"`
	URL             string            `json:"url"`
	Headers         map[string]string `json:"headers"`
	Body            string            `json:"body"`
	SessionID       string            `json:"session_id"`
	Bindings        []Binding         `json:"bindings"`
	HTTPVersionSeen string            `json:"http_version_seen"`
	Source          string            `json:"source"` // "import" | "crawl" | "declared"
}

// Endpoint is a single addressable operation: method + path template
// (Design/DOMAIN.md §Endpoint).
type Endpoint struct {
	Method string `json:"method"`
	Path   string `json:"path"`
}

// MethodDelete is the one HTTP verb RaceVeil treats as destructive and
// blocks by default (Design/SECURITY.md §6).
const MethodDelete = "DELETE"

// mutatingMethods are the HTTP verbs that can produce a state-changing
// effect. Which verbs can change state is domain vocabulary — it is the
// same question as "can this Endpoint carry an integrity violation at all"
// — so it lives here rather than in a discovery heuristic.
var mutatingMethods = map[string]bool{
	"POST": true, "PUT": true, "PATCH": true, MethodDelete: true,
}

// IsMutating reports whether a method can change server state. Only
// mutating Endpoints can violate an integrity Invariant: a read-only
// endpoint has no effect to duplicate.
func IsMutating(method string) bool {
	return mutatingMethods[upper(method)]
}

// IsDestructiveVerb reports whether a method is destructive enough to be
// blocked by default (Design/SECURITY.md §6).
func IsDestructiveVerb(method string) bool {
	return upper(method) == MethodDelete
}

func upper(s string) string {
	out := []byte(s)
	for i, c := range out {
		if c >= 'a' && c <= 'z' {
			out[i] = c - ('a' - 'A')
		}
	}
	return string(out)
}
