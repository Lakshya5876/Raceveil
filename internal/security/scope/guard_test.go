package scope

import (
	"context"
	"errors"
	"net"
	"testing"
	"time"

	"github.com/Lakshya5876/Raceveil/internal/domain"
)

func fixtureScope() domain.Scope {
	return domain.Scope{
		Target:             "http://127.0.0.1:18743",
		AuthorizedBy:       "test",
		AllowPrivateTarget: true,
		Hosts: []domain.HostRule{
			{Host: "127.0.0.1", Ports: []int{18743}, PinIP: "127.0.0.1", PathPrefixes: []string{"/"}},
		},
		Caps: domain.Caps{MaxConcurrency: 10, MaxRequestsTotal: 2000, MaxRatePerSec: 100, MaxWallclockSec: 120},
	}
}

func TestNewGuard_RejectsPrivateTargetWithoutAllow(t *testing.T) {
	s := fixtureScope()
	s.AllowPrivateTarget = false
	if _, err := NewGuard(s); err == nil {
		t.Fatal("expected NewGuard to reject a private pin_ip without allow_private_target")
	}
}

func TestNewGuard_AllowsPrivateTargetWithAllow(t *testing.T) {
	if _, err := NewGuard(fixtureScope()); err != nil {
		t.Fatalf("NewGuard: %v", err)
	}
}

func TestCheck_AllowsInScopeRequest(t *testing.T) {
	g, err := NewGuard(fixtureScope())
	if err != nil {
		t.Fatalf("NewGuard: %v", err)
	}
	if err := g.Check("POST", "http://127.0.0.1:18743/issue-code"); err != nil {
		t.Fatalf("expected in-scope request to be allowed, got %v", err)
	}
}

func TestCheck_RejectsUnlistedHost(t *testing.T) {
	g, err := NewGuard(fixtureScope())
	if err != nil {
		t.Fatalf("NewGuard: %v", err)
	}
	err = g.Check("POST", "http://evil.example.com/issue-code")
	if !errors.Is(err, ErrOutOfScope) {
		t.Fatalf("expected ErrOutOfScope for unlisted host, got %v", err)
	}
}

func TestCheck_RejectsUnlistedPort(t *testing.T) {
	g, err := NewGuard(fixtureScope())
	if err != nil {
		t.Fatalf("NewGuard: %v", err)
	}
	err = g.Check("POST", "http://127.0.0.1:9999/issue-code")
	if !errors.Is(err, ErrOutOfScope) {
		t.Fatalf("expected ErrOutOfScope for unlisted port, got %v", err)
	}
}

func TestCheck_RejectsPathOutsidePrefix(t *testing.T) {
	s := fixtureScope()
	s.Hosts[0].PathPrefixes = []string{"/api/"}
	g, err := NewGuard(s)
	if err != nil {
		t.Fatalf("NewGuard: %v", err)
	}
	err = g.Check("POST", "http://127.0.0.1:18743/issue-code")
	if !errors.Is(err, ErrOutOfScope) {
		t.Fatalf("expected ErrOutOfScope for path outside prefix, got %v", err)
	}
}

func TestCheck_BlocksDeleteByDefault(t *testing.T) {
	g, err := NewGuard(fixtureScope())
	if err != nil {
		t.Fatalf("NewGuard: %v", err)
	}
	err = g.Check("DELETE", "http://127.0.0.1:18743/issue-code")
	if !errors.Is(err, ErrOutOfScope) {
		t.Fatalf("expected ErrOutOfScope for DELETE by default, got %v", err)
	}
}

func TestCheck_AllowsDeleteWhenDeclared(t *testing.T) {
	s := fixtureScope()
	s.Destructive.AllowVerbs = []string{"DELETE"}
	g, err := NewGuard(s)
	if err != nil {
		t.Fatalf("NewGuard: %v", err)
	}
	if err := g.Check("DELETE", "http://127.0.0.1:18743/issue-code"); err != nil {
		t.Fatalf("expected DELETE to be allowed once declared, got %v", err)
	}
}

func TestCheck_RejectsDenyPathGlob(t *testing.T) {
	s := fixtureScope()
	s.Destructive.DenyPaths = []string{"/admin/**"}
	g, err := NewGuard(s)
	if err != nil {
		t.Fatalf("NewGuard: %v", err)
	}
	err = g.Check("POST", "http://127.0.0.1:18743/admin/reset")
	if !errors.Is(err, ErrOutOfScope) {
		t.Fatalf("expected ErrOutOfScope for deny_paths match, got %v", err)
	}
}

func TestCheck_RejectsOverTotalRequestCap(t *testing.T) {
	s := fixtureScope()
	s.Caps.MaxRequestsTotal = 2
	g, err := NewGuard(s)
	if err != nil {
		t.Fatalf("NewGuard: %v", err)
	}
	for i := 0; i < 2; i++ {
		if err := g.Check("POST", "http://127.0.0.1:18743/issue-code"); err != nil {
			t.Fatalf("request %d: expected allow, got %v", i, err)
		}
	}
	if err := g.Check("POST", "http://127.0.0.1:18743/issue-code"); !errors.Is(err, ErrOutOfScope) {
		t.Fatalf("expected ErrOutOfScope once total request cap exhausted, got %v", err)
	}
}

func TestCheck_RejectsOverRateCap(t *testing.T) {
	s := fixtureScope()
	s.Caps.MaxRatePerSec = 1
	s.Caps.MaxRequestsTotal = 0
	g, err := NewGuard(s)
	if err != nil {
		t.Fatalf("NewGuard: %v", err)
	}
	if err := g.Check("POST", "http://127.0.0.1:18743/issue-code"); err != nil {
		t.Fatalf("expected first request to be allowed, got %v", err)
	}
	if err := g.Check("POST", "http://127.0.0.1:18743/issue-code"); !errors.Is(err, ErrOutOfScope) {
		t.Fatalf("expected ErrOutOfScope for exceeding rate cap within the same window, got %v", err)
	}
}

func TestCheck_RejectsAfterWallclockBudget(t *testing.T) {
	s := fixtureScope()
	s.Caps.MaxWallclockSec = 1
	g, err := NewGuard(s)
	if err != nil {
		t.Fatalf("NewGuard: %v", err)
	}
	g.started = time.Now().Add(-2 * time.Second)
	if err := g.Check("POST", "http://127.0.0.1:18743/issue-code"); !errors.Is(err, ErrOutOfScope) {
		t.Fatalf("expected ErrOutOfScope once wallclock budget exceeded, got %v", err)
	}
}

func TestAcquireSlot_RespectsMaxConcurrency(t *testing.T) {
	s := fixtureScope()
	s.Caps.MaxConcurrency = 1
	g, err := NewGuard(s)
	if err != nil {
		t.Fatalf("NewGuard: %v", err)
	}
	ctx := context.Background()
	release, err := g.AcquireSlot(ctx)
	if err != nil {
		t.Fatalf("first AcquireSlot: %v", err)
	}
	tightCtx, cancel := context.WithTimeout(ctx, 50*time.Millisecond)
	defer cancel()
	if _, err := g.AcquireSlot(tightCtx); err == nil {
		t.Fatal("expected second AcquireSlot to block until timeout while slot held")
	}
	release()
	release2, err := g.AcquireSlot(ctx)
	if err != nil {
		t.Fatalf("AcquireSlot after release: %v", err)
	}
	release2()
}

func TestIsPrivateOrMetadata(t *testing.T) {
	cases := map[string]bool{
		"127.0.0.1":       true,
		"10.0.0.5":        true,
		"172.16.0.5":      true,
		"192.168.1.1":     true,
		"169.254.169.254": true,
		"8.8.8.8":         false,
		"93.184.216.34":   false,
	}
	for ipStr, want := range cases {
		t.Run(ipStr, func(t *testing.T) {
			ip := net.ParseIP(ipStr)
			if ip == nil {
				t.Fatalf("invalid test IP %q", ipStr)
			}
			if got := isPrivateOrMetadata(ip); got != want {
				t.Errorf("isPrivateOrMetadata(%s) = %v, want %v", ipStr, got, want)
			}
		})
	}
}
