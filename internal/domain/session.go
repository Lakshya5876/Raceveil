package domain

// Cookie is one session cookie carried by a Session (Design/DATA_MODEL.md
// session.json).
type Cookie struct {
	Name   string `json:"name"`
	Value  string `json:"value"`
	Domain string `json:"domain"`
	Path   string `json:"path"`
}

// Session is the authentication/identity context a Workflow runs under
// (Design/DOMAIN.md §Session). Secrets carried here (cookie values, header
// tokens) live in memory only; every persisted artifact redacts them.
type Session struct {
	ID             string            `json:"id"`
	Cookies        []Cookie          `json:"cookies"`
	Headers        map[string]string `json:"headers"`
	IsolationGroup string            `json:"isolation_group"`
}
