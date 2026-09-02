package scope

import (
	"fmt"
	"net"
	"strings"

	"github.com/Lakshya5876/Raceveil/internal/domain"
)

// ErrPublicTargetRefused is returned when safe-mode refuses a target that
// resolves to a public IP without the explicit operator acknowledgement
// (Design/SECURITY.md §2). It wraps ErrOutOfScope so callers that already
// treat scope refusals as exit code 5 need no special case.
var ErrPublicTargetRefused = fmt.Errorf("%w: public target refused in safe-mode", ErrOutOfScope)

// CheckSafeMode enforces Design/SECURITY.md §2's public-target gate before
// any traffic is sent:
//
//   - safe-mode (the default) refuses any Scope host that resolves to a
//     public IP unless the operator passes --i-am-authorized;
//   - --no-safe-mode itself also requires --i-am-authorized.
//
// This is a friction/technical gate to prevent accidental scanning. It is
// explicitly NOT a determination that testing is permitted — the Scope's
// authorized_by string is provenance metadata the operator is responsible
// for, and RaceVeil cannot verify it.
func CheckSafeMode(s domain.Scope, safeMode, iAmAuthorized bool) error {
	if !safeMode && !iAmAuthorized {
		return fmt.Errorf("%w: --no-safe-mode requires --i-am-authorized", ErrOutOfScope)
	}

	var public []string
	for _, host := range s.Hosts {
		ip, err := pinIP(host)
		if err != nil {
			return err
		}
		if !isPrivateOrMetadata(ip) {
			public = append(public, fmt.Sprintf("%s (%s)", host.Host, ip))
		}
	}
	if len(public) == 0 {
		return nil
	}
	if !iAmAuthorized {
		return fmt.Errorf("%w: %s resolves to a public IP; pass --i-am-authorized to confirm you are authorized to test it",
			ErrPublicTargetRefused, strings.Join(public, ", "))
	}
	return nil
}

// Describe renders the effective Scope for the pre-scan banner
// (Design/UX.md: always show what will and won't be touched before touching
// it). It resolves nothing new — it reports what the Guard already pinned.
func (g *Guard) Describe() string {
	var hosts []string
	for name, entry := range g.hosts {
		ports := make([]string, 0, len(entry.ports))
		for p := range entry.ports {
			ports = append(ports, fmt.Sprint(p))
		}
		hosts = append(hosts, fmt.Sprintf("%s:%s(%s) paths:%s",
			name, strings.Join(ports, ","), entry.pinnedIP, strings.Join(entry.rule.PathPrefixes, ",")))
	}
	destructive := "blocked"
	if len(g.scope.Destructive.AllowVerbs) > 0 {
		destructive = "allowed:" + strings.Join(g.scope.Destructive.AllowVerbs, ",")
	}
	return fmt.Sprintf("Scope: %s  caps: N<=%d rate<=%d/s total<=%d wallclock<=%ds  destructive:%s  proof-cap:%d",
		strings.Join(hosts, " "),
		g.scope.Caps.MaxConcurrency, g.scope.Caps.MaxRatePerSec,
		g.scope.Caps.MaxRequestsTotal, g.scope.Caps.MaxWallclockSec,
		destructive, g.scope.Proof.MaxSuccessfulEffects)
}

// AuthorizedBy returns the Scope's operator-declared provenance string,
// printed alongside every scan so a reader always knows what authorization
// was claimed — and that RaceVeil did not verify it.
func (g *Guard) AuthorizedBy() string { return g.scope.AuthorizedBy }

// ResolvedIPs reports the pinned IP per authorized host, for `scope check`.
func (g *Guard) ResolvedIPs() map[string]net.IP {
	out := make(map[string]net.IP, len(g.hosts))
	for name, entry := range g.hosts {
		out[name] = entry.pinnedIP
	}
	return out
}
