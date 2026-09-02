package wiring

import (
	"context"
	"encoding/json"
	"fmt"
	"net"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"time"

	"gopkg.in/yaml.v3"

	"github.com/Lakshya5876/Raceveil/internal/application/scheduler"
	"github.com/Lakshya5876/Raceveil/internal/config"
	"github.com/Lakshya5876/Raceveil/internal/domain"
	"github.com/Lakshya5876/Raceveil/internal/security/scope"
)

// ScopeInitResult reports what `scope init` resolved and wrote.
type ScopeInitResult struct {
	Path     string
	Resolved map[string]string
}

// InitScope resolves a target, pins its IP, and writes a conservative
// default-deny scope.yaml (Design/API.md scope init). The generated caps
// are deliberately modest: an operator should have to raise them
// deliberately, never discover them raised by default.
func InitScope(target, out, authorizedBy string) (ScopeInitResult, error) {
	result := ScopeInitResult{Path: out, Resolved: map[string]string{}}
	if out == "" {
		result.Path = "scope.yaml"
	}
	u, err := url.Parse(target)
	if err != nil || u.Host == "" {
		return result, fmt.Errorf("scope init: %q is not a valid base URL (want e.g. https://host[:port])", target)
	}

	host := u.Hostname()
	ips, err := net.LookupIP(host)
	if err != nil || len(ips) == 0 {
		return result, fmt.Errorf("scope init: cannot resolve %q to pin an IP: %w", host, err)
	}
	pinned := ips[0]
	result.Resolved[host] = pinned.String()

	sc := defaultScope(target, host, authorizedBy, portOf(u), pinned)

	data, err := yaml.Marshal(sc)
	if err != nil {
		return result, fmt.Errorf("scope init: marshal: %w", err)
	}
	header := "# RaceVeil Scope — the default-deny authorization boundary.\n" +
		"# RaceVeil enforces this technically; it does NOT verify that you are authorized.\n" +
		"# authorized_by is provenance metadata you are responsible for.\n"
	if err := os.WriteFile(result.Path, append([]byte(header), data...), 0o600); err != nil {
		return result, fmt.Errorf("scope init: write %s: %w", result.Path, err)
	}
	return result, nil
}

// portOf derives the target port from the URL, defaulting by scheme.
func portOf(u *url.URL) int {
	if p := u.Port(); p != "" {
		if parsed, err := strconv.Atoi(p); err == nil {
			return parsed
		}
	}
	if u.Scheme == "http" {
		return 80
	}
	return 443
}

// defaultScope builds the conservative default-deny Scope `scope init`
// writes. Caps are deliberately modest: an operator should have to raise
// them deliberately, never discover them already raised.
func defaultScope(target, host, authorizedBy string, port int, pinned net.IP) domain.Scope {
	if authorizedBy == "" {
		authorizedBy = "UNSET — replace with your own authorization provenance (ticket, owner, date)"
	}
	return domain.Scope{
		Target:             strings.TrimSuffix(target, "/"),
		AuthorizedBy:       authorizedBy,
		AllowPrivateTarget: isPrivateIP(pinned),
		Hosts: []domain.HostRule{{
			Host: host, Ports: []int{port}, PinIP: pinned.String(), PathPrefixes: []string{"/"},
		}},
		Caps: domain.Caps{
			MaxConcurrency: 10, MaxRequestsTotal: 2000, MaxRatePerSec: 50, MaxWallclockSec: 300,
		},
		Destructive: domain.Destructive{AllowVerbs: []string{}, DenyPaths: []string{"/admin/**"}},
		Proof:       domain.Proof{MaxSuccessfulEffects: 2},
	}
}

func isPrivateIP(ip net.IP) bool {
	return ip.IsLoopback() || ip.IsPrivate() || ip.IsLinkLocalUnicast() || ip.IsUnspecified()
}

// ScopeCheckReport is what `scope check` prints: the effective scope, and
// an explicit statement of what is and is not reachable.
type ScopeCheckReport struct {
	Effective    string
	AuthorizedBy string
	Allowed      []string
	Denied       []string
}

// CheckScope validates a scope file, applies the safe-mode gate, and
// reports exactly what RaceVeil would and would not touch (Design/API.md
// scope check: "Run this before anything").
func CheckScope(scopePath string, safeMode, iAmAuthorized bool) (ScopeCheckReport, error) {
	sc, err := scheduler.LoadScope(scopePath)
	if err != nil {
		return ScopeCheckReport{}, err
	}
	if err := scope.CheckSafeMode(sc, safeMode, iAmAuthorized); err != nil {
		return ScopeCheckReport{}, err
	}
	guard, err := scope.NewGuard(sc)
	if err != nil {
		return ScopeCheckReport{}, fmt.Errorf("scope: %w", err)
	}

	report := ScopeCheckReport{
		Effective:    guard.Describe(),
		AuthorizedBy: guard.AuthorizedBy(),
	}
	for host, ip := range guard.ResolvedIPs() {
		for _, rule := range sc.Hosts {
			if !strings.EqualFold(rule.Host, host) {
				continue
			}
			for _, prefix := range rule.PathPrefixes {
				report.Allowed = append(report.Allowed,
					fmt.Sprintf("%s (pinned %s) ports %v paths %s*", host, ip, rule.Ports, prefix))
			}
		}
	}
	report.Denied = append(report.Denied,
		"any host not listed above (default-deny; DNS rebinding cannot redirect traffic — IPs are pinned for the run)",
		"private/loopback/link-local/cloud-metadata ranges unless allow_private_target is set",
		"DELETE and any verb not in destructive.allow_verbs",
	)
	for _, p := range sc.Destructive.DenyPaths {
		report.Denied = append(report.Denied, "paths matching "+p)
	}
	report.Denied = append(report.Denied,
		fmt.Sprintf("more than %d successful state-changing effects while proving a finding (proof cap)",
			sc.Proof.MaxSuccessfulEffects))
	return report, nil
}

// loginRecipe describes how to obtain a Session (Design/API.md auth login).
// Credentials are referenced by environment-variable name, never inlined:
// a recipe file is meant to be committable, a session.json is not.
type loginRecipe struct {
	ID             string            `yaml:"id"`
	IsolationGroup string            `yaml:"isolation_group"`
	Method         string            `yaml:"method"`
	URL            string            `yaml:"url"`
	Headers        map[string]string `yaml:"headers"`
	// Body may contain ${ENV_VAR} placeholders, resolved from the process
	// environment through internal/config at send time.
	Body string `yaml:"body"`
	// KeepCookies names the response cookies that constitute the session.
	// Empty means keep every cookie the login response sets.
	KeepCookies []string `yaml:"keep_cookies"`
	// TokenFrom extracts a bearer token from the JSON response body, e.g.
	// "$.access_token", and sets it as an Authorization header.
	TokenFrom string `yaml:"token_from"`
}

// RunAuthLogin executes a login recipe and writes the captured Session.
func RunAuthLogin(ctx context.Context, recipePath, out string) (string, domain.Session, error) {
	if out == "" {
		out = "session.json"
	}
	recipe, err := loadLoginRecipe(recipePath)
	if err != nil {
		return "", domain.Session{}, err
	}
	resp, err := executeLogin(ctx, recipe)
	if err != nil {
		return "", domain.Session{}, err
	}
	defer func() { _ = resp.Body.Close() }()

	session, err := sessionFromResponse(recipe, resp)
	if err != nil {
		return "", domain.Session{}, err
	}
	encoded, err := json.MarshalIndent(session, "", "  ")
	if err != nil {
		return "", domain.Session{}, fmt.Errorf("auth login: marshal session: %w", err)
	}
	// 0600: a session.json holds live credentials (Design/SECURITY.md §9).
	if err := os.WriteFile(out, encoded, 0o600); err != nil {
		return "", domain.Session{}, fmt.Errorf("auth login: write %s: %w", out, err)
	}
	return out, session, nil
}

func loadLoginRecipe(path string) (loginRecipe, error) {
	var recipe loginRecipe
	data, err := os.ReadFile(filepath.Clean(path)) // #nosec G304 -- operator-supplied CLI path, expected
	if err != nil {
		return recipe, fmt.Errorf("auth login: read %s: %w", path, err)
	}
	if err := yaml.Unmarshal(data, &recipe); err != nil {
		return recipe, fmt.Errorf("auth login: parse %s: %w", path, err)
	}
	if recipe.URL == "" {
		return recipe, fmt.Errorf("auth login: recipe %s has no url", path)
	}
	if recipe.Method == "" {
		recipe.Method = http.MethodPost
	}
	return recipe, nil
}

func executeLogin(ctx context.Context, recipe loginRecipe) (*http.Response, error) {
	body := expandEnv(recipe.Body)
	req, err := http.NewRequestWithContext(ctx, strings.ToUpper(recipe.Method), recipe.URL, strings.NewReader(body))
	if err != nil {
		return nil, fmt.Errorf("auth login: build request: %w", err)
	}
	for k, v := range recipe.Headers {
		req.Header.Set(k, expandEnv(v))
	}
	if req.Header.Get("Content-Type") == "" && body != "" {
		req.Header.Set("Content-Type", "application/json")
	}

	client := &http.Client{Timeout: 20 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("auth login: %w", err)
	}
	if resp.StatusCode >= 400 {
		_ = resp.Body.Close()
		return nil, fmt.Errorf("auth login: %s returned %d", recipe.URL, resp.StatusCode)
	}
	return resp, nil
}

func sessionFromResponse(recipe loginRecipe, resp *http.Response) (domain.Session, error) {
	session := domain.Session{
		ID:             orDefault(recipe.ID, "sess_default"),
		IsolationGroup: orDefault(recipe.IsolationGroup, "default"),
		Headers:        map[string]string{},
	}
	keep := make(map[string]bool, len(recipe.KeepCookies))
	for _, name := range recipe.KeepCookies {
		keep[name] = true
	}
	for _, ck := range resp.Cookies() {
		if len(keep) > 0 && !keep[ck.Name] {
			continue
		}
		session.Cookies = append(session.Cookies, domain.Cookie{
			Name: ck.Name, Value: ck.Value, Domain: ck.Domain, Path: ck.Path,
		})
	}
	if recipe.TokenFrom != "" {
		token, err := extractToken(resp, recipe.TokenFrom)
		if err != nil {
			return domain.Session{}, err
		}
		session.Headers["Authorization"] = "Bearer " + token
	}
	if len(session.Cookies) == 0 && len(session.Headers) == 0 {
		return domain.Session{}, fmt.Errorf(
			"auth login: %s succeeded but produced no cookies or token — check keep_cookies/token_from", recipe.URL)
	}
	return session, nil
}

func extractToken(resp *http.Response, path string) (string, error) {
	var payload map[string]any
	if err := json.NewDecoder(resp.Body).Decode(&payload); err != nil {
		return "", fmt.Errorf("auth login: token_from %q: response is not JSON: %w", path, err)
	}
	field := strings.TrimPrefix(path, "$.")
	value, ok := payload[field]
	if !ok {
		return "", fmt.Errorf("auth login: token_from %q: field not present in the login response", path)
	}
	token, ok := value.(string)
	if !ok {
		return "", fmt.Errorf("auth login: token_from %q: field is not a string", path)
	}
	return token, nil
}

// envPlaceholder matches ${VAR} references in a login recipe.
var envPlaceholder = regexp.MustCompile(`\$\{([A-Za-z_][A-Za-z0-9_]*)\}`)

// expandEnv resolves ${VAR} placeholders through internal/config, the only
// module permitted to read the environment (CLAUDE.md §2.1).
func expandEnv(s string) string {
	return envPlaceholder.ReplaceAllStringFunc(s, func(match string) string {
		name := strings.TrimSuffix(strings.TrimPrefix(match, "${"), "}")
		return config.Lookup(name, "")
	})
}

func orDefault(v, fallback string) string {
	if v == "" {
		return fallback
	}
	return v
}

// AuthStatus is what `auth check` reports.
type AuthStatus struct {
	SessionID      string
	IsolationGroup string
	ProbeURL       string
	Status         int
	Authenticated  bool
}

// CheckAuth confirms a Session is live by issuing one read-only, in-scope
// GET through the Guard and checking it is neither refused nor bounced to
// a login page (Design/API.md auth check).
func CheckAuth(ctx context.Context, scopePath, authPath, probePath string) (AuthStatus, error) {
	sc, err := scheduler.LoadScope(scopePath)
	if err != nil {
		return AuthStatus{}, err
	}
	session, err := scheduler.LoadSession(authPath)
	if err != nil {
		return AuthStatus{}, err
	}
	guard, err := scope.NewGuard(sc)
	if err != nil {
		return AuthStatus{}, fmt.Errorf("scope: %w", err)
	}
	if probePath == "" {
		probePath = "/"
	}

	status := AuthStatus{
		SessionID:      session.ID,
		IsolationGroup: session.IsolationGroup,
		ProbeURL:       strings.TrimSuffix(sc.Target, "/") + probePath,
	}
	code, body, err := scheduler.ProbeEndpoint(ctx, guard, session, status.ProbeURL)
	if err != nil {
		return status, fmt.Errorf("auth check: %w", err)
	}
	status.Status = code
	status.Authenticated = code < 400 && !looksLikeLoginPage(body)
	return status, nil
}

// looksLikeLoginPage catches the common "200 OK, but it's the login form"
// case that a bare status check would call authenticated.
func looksLikeLoginPage(body string) bool {
	lower := strings.ToLower(body)
	for _, marker := range []string{"sign in", "log in", "login", "password"} {
		if strings.Contains(lower, marker) {
			return true
		}
	}
	return false
}
