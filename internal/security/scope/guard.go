// Package scope implements RaceVeil's Scope/Authorization guard: the single
// chokepoint every outbound request must pass through before RaceVeil will
// send it (Design/SECURITY.md §3). It enforces, in order, the host/port/path
// allowlist (default-deny), IP pinning against DNS rebinding, private/
// loopback/metadata-IP blocking, per-scope caps, and destructive-verb rules.
//
// This is CORE_FILES (docs/ARCHITECTURE_DECISIONS.md GOV-004): a regression
// here is an SSRF/DNS-rebinding/scope-escape class bug, not an ordinary
// feature bug, so it always triggers a full-suite run on change.
package scope

import (
	"context"
	"errors"
	"fmt"
	"net"
	"net/url"
	"path"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/Lakshya5876/Raceveil/internal/domain"
)

// ErrOutOfScope wraps every rejection the Guard produces, so callers can
// distinguish a scope refusal from a network/protocol error.
var ErrOutOfScope = errors.New("out of scope")

type hostEntry struct {
	rule     domain.HostRule
	pinnedIP net.IP
	ports    map[int]bool
}

// Guard is the enforcement chokepoint every outbound request passes through
// (Design/SECURITY.md §3). It cannot be bypassed by discovery, redirects, or
// replay: every caller sends through Check + Dial.
type Guard struct {
	scope domain.Scope
	hosts map[string]*hostEntry
	sem   chan struct{}

	mu           sync.Mutex
	started      time.Time
	requestsSent int
	windowStart  time.Time
	windowCount  int
}

// NewGuard builds a Guard from a Scope, resolving and pinning each
// authorized host's IP once (Design/SECURITY.md §4: anti-DNS-rebinding). A
// HostRule with an explicit pin_ip uses it directly instead of resolving.
// Construction fails if a host resolves to a private/metadata address and
// the Scope does not declare allow_private_target.
func NewGuard(s domain.Scope) (*Guard, error) {
	g := &Guard{
		scope:   s,
		hosts:   make(map[string]*hostEntry, len(s.Hosts)),
		started: time.Now(),
	}
	if s.Caps.MaxConcurrency > 0 {
		g.sem = make(chan struct{}, s.Caps.MaxConcurrency)
	}
	for _, hr := range s.Hosts {
		ip, err := pinIP(hr)
		if err != nil {
			return nil, err
		}
		if !s.AllowPrivateTarget && isPrivateOrMetadata(ip) {
			return nil, fmt.Errorf("scope: host %q resolves to private/metadata IP %s but allow_private_target is false", hr.Host, ip)
		}
		ports := make(map[int]bool, len(hr.Ports))
		for _, p := range hr.Ports {
			ports[p] = true
		}
		g.hosts[strings.ToLower(hr.Host)] = &hostEntry{rule: hr, pinnedIP: ip, ports: ports}
	}
	return g, nil
}

func pinIP(hr domain.HostRule) (net.IP, error) {
	if hr.PinIP != "" {
		ip := net.ParseIP(hr.PinIP)
		if ip == nil {
			return nil, fmt.Errorf("scope: host %q has invalid pin_ip %q", hr.Host, hr.PinIP)
		}
		return ip, nil
	}
	addrs, err := net.LookupIP(hr.Host)
	if err != nil || len(addrs) == 0 {
		return nil, fmt.Errorf("scope: cannot resolve host %q to pin an IP: %w", hr.Host, err)
	}
	return addrs[0], nil
}

// Check validates one outbound request against every Guard rule, in the
// fixed order from Design/SECURITY.md §3 (allowlist -> pinned-IP private
// check -> caps -> destructive-verb rules), and atomically records it
// against the caps counters. Callers must call Check immediately before
// sending; a request refused here must not be sent.
func (g *Guard) Check(method, rawURL string) error {
	u, err := url.Parse(rawURL)
	if err != nil {
		return fmt.Errorf("%w: unparseable URL %q: %v", ErrOutOfScope, rawURL, err)
	}
	if err := g.checkAllowlist(method, u); err != nil {
		return err
	}
	return g.checkAndRecordCaps()
}

// checkAllowlist enforces Design/SECURITY.md §3 steps 1, 3, and 5: the
// host/port/path allowlist, the pinned-IP private/metadata block, and the
// destructive-verb/deny_paths rules. Step 2 (IP pinning) already happened
// once at NewGuard time; step 4 (caps) is checkAndRecordCaps.
func (g *Guard) checkAllowlist(method string, u *url.URL) error {
	host := strings.ToLower(u.Hostname())
	entry, ok := g.hosts[host]
	if !ok {
		return fmt.Errorf("%w: host %q not in scope allowlist", ErrOutOfScope, host)
	}
	// An empty path is semantically "/" (RFC 3986 §6.2.3), so
	// "https://host" must be treated exactly like "https://host/" — that is
	// the most natural way to name a target on the command line.
	path := u.Path
	if path == "" {
		path = "/"
	}
	if err := g.checkHostReachable(entry, host, portOf(u), path); err != nil {
		return err
	}
	return g.checkDestructiveRules(method, path)
}

// checkHostReachable covers Design/SECURITY.md §3 steps 1 and 3: the
// host/port/path allowlist and the pinned-IP private/metadata block.
func (g *Guard) checkHostReachable(entry *hostEntry, host string, port int, path string) error {
	if len(entry.ports) > 0 && !entry.ports[port] {
		return fmt.Errorf("%w: port %d not authorized for host %q", ErrOutOfScope, port, host)
	}
	if !pathAllowed(path, entry.rule.PathPrefixes) {
		return fmt.Errorf("%w: path %q not under an authorized prefix for host %q", ErrOutOfScope, path, host)
	}
	if !g.scope.AllowPrivateTarget && isPrivateOrMetadata(entry.pinnedIP) {
		return fmt.Errorf("%w: pinned IP %s for host %q is private/metadata and allow_private_target is false",
			ErrOutOfScope, entry.pinnedIP, host)
	}
	return nil
}

// checkDestructiveRules covers Design/SECURITY.md §3 step 5 and §6.
func (g *Guard) checkDestructiveRules(method, path string) error {
	if isDestructiveVerb(method) && !g.destructiveAllowed(method) {
		return fmt.Errorf("%w: destructive verb %q not allowed by scope.destructive.allow_verbs", ErrOutOfScope, method)
	}
	if pathDenied(path, g.scope.Destructive.DenyPaths) {
		return fmt.Errorf("%w: path %q matches a deny_paths rule", ErrOutOfScope, path)
	}
	return nil
}

// checkAndRecordCaps enforces the Scope's per-run budgets
// (Design/SECURITY.md §8) and distinguishes the two kinds:
//
//   - max_requests_total and max_wallclock_sec are *budgets*. Once spent
//     they never come back, so exceeding one is terminal and refuses.
//   - max_rate_per_sec is a *throttle*. Its window clears every second, so
//     the correct response to hitting it is to wait for the next window,
//     not to abort a scan mid-experiment. Throttling is the whole point of
//     a rate cap: it shapes load, it does not cancel work.
//
// waitFor is returned rather than slept on here so the caller can wait
// without holding the mutex.
func (g *Guard) checkAndRecordCaps() error {
	for {
		waitFor, err := g.reserveSlot()
		if err != nil {
			return err
		}
		if waitFor <= 0 {
			return nil
		}
		time.Sleep(waitFor)
	}
}

// reserveSlot accounts for one request against the caps. It returns a
// non-zero duration when the caller must wait for the rate window to roll
// over before the request may be sent.
func (g *Guard) reserveSlot() (time.Duration, error) {
	g.mu.Lock()
	defer g.mu.Unlock()

	now := time.Now()
	if g.scope.Caps.MaxWallclockSec > 0 && now.Sub(g.started) > time.Duration(g.scope.Caps.MaxWallclockSec)*time.Second {
		return 0, fmt.Errorf("%w: wallclock budget of %ds exceeded", ErrOutOfScope, g.scope.Caps.MaxWallclockSec)
	}
	if g.scope.Caps.MaxRequestsTotal > 0 && g.requestsSent >= g.scope.Caps.MaxRequestsTotal {
		return 0, fmt.Errorf("%w: total request budget of %d exhausted", ErrOutOfScope, g.scope.Caps.MaxRequestsTotal)
	}
	if g.scope.Caps.MaxRatePerSec > 0 {
		if now.Sub(g.windowStart) >= time.Second {
			g.windowStart = now
			g.windowCount = 0
		}
		if g.windowCount >= g.scope.Caps.MaxRatePerSec {
			return time.Second - now.Sub(g.windowStart), nil
		}
		g.windowCount++
	}
	g.requestsSent++
	return 0, nil
}

// AcquireSlot blocks until a concurrency slot is available under the
// Scope's max_concurrency cap (a no-op if the Scope declares no cap),
// returning a func that releases the slot. Callers must call the returned
// func exactly once, typically deferred.
func (g *Guard) AcquireSlot(ctx context.Context) (func(), error) {
	if g.sem == nil {
		return func() {}, nil
	}
	select {
	case g.sem <- struct{}{}:
		return func() { <-g.sem }, nil
	case <-ctx.Done():
		return nil, ctx.Err()
	}
}

// Dial dials the pinned IP for addr's host (never a fresh DNS resolution),
// preventing DNS-rebinding mid-run (Design/SECURITY.md §4). addr is
// "host:port" as passed by net/http's Transport.DialContext / DialTLSContext.
// The hostname is preserved by the caller for TLS SNI; only the connection
// target is substituted.
func (g *Guard) Dial(ctx context.Context, network, addr string) (net.Conn, error) {
	host, port, err := net.SplitHostPort(addr)
	if err != nil {
		return nil, fmt.Errorf("scope: invalid dial address %q: %w", addr, err)
	}
	entry, ok := g.hosts[strings.ToLower(host)]
	if !ok {
		return nil, fmt.Errorf("%w: dial to host %q not in scope allowlist", ErrOutOfScope, host)
	}
	var d net.Dialer
	return d.DialContext(ctx, network, net.JoinHostPort(entry.pinnedIP.String(), port))
}

func portOf(u *url.URL) int {
	if p := u.Port(); p != "" {
		n, err := strconv.Atoi(p)
		if err == nil {
			return n
		}
	}
	if u.Scheme == "https" {
		return 443
	}
	return 80
}

func pathAllowed(p string, prefixes []string) bool {
	for _, prefix := range prefixes {
		if strings.HasPrefix(p, prefix) {
			return true
		}
	}
	return false
}

func pathDenied(p string, patterns []string) bool {
	for _, pat := range patterns {
		if matchGlob(pat, p) {
			return true
		}
	}
	return false
}

func matchGlob(pattern, p string) bool {
	if strings.HasSuffix(pattern, "/**") {
		prefix := strings.TrimSuffix(pattern, "/**")
		return p == prefix || strings.HasPrefix(p, prefix+"/")
	}
	ok, err := path.Match(pattern, p)
	return err == nil && ok
}

func isDestructiveVerb(method string) bool {
	return domain.IsDestructiveVerb(method)
}

func (g *Guard) destructiveAllowed(method string) bool {
	for _, v := range g.scope.Destructive.AllowVerbs {
		if strings.EqualFold(v, method) {
			return true
		}
	}
	return false
}

var privateBlocks []*net.IPNet

func init() {
	for _, cidr := range []string{
		"127.0.0.0/8", "10.0.0.0/8", "172.16.0.0/12", "192.168.0.0/16",
		"169.254.0.0/16", "::1/128", "fd00::/8", "fe80::/10",
	} {
		if _, block, err := net.ParseCIDR(cidr); err == nil {
			privateBlocks = append(privateBlocks, block)
		}
	}
}

func isPrivateOrMetadata(ip net.IP) bool {
	if ip == nil {
		return false
	}
	for _, block := range privateBlocks {
		if block.Contains(ip) {
			return true
		}
	}
	return false
}
