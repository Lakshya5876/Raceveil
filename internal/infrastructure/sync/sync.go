// Package sync implements RaceVeil's synchronized-concurrency engine: the
// H2SinglePacket and H1LastByte strategies that release N prepared Act-
// request instances' final bytes/frames as close to simultaneously as
// possible (Design/ARCHITECTURE.md §2.6). Prior-art infrastructure,
// reimplemented under Apache-2.0 to avoid a GPL-3 dependency (Context/
// DECISIONS.md ADR-009). Implementation lands in Phase 1 (Context/ROADMAP.md).
//
// Named "sync" to match Design/ARCHITECTURE.md's own naming; callers that
// also need the stdlib sync package import it under an alias.
package sync

import (
	"context"
	"fmt"
	"net"
	"time"
)

// Dialer opens a network connection for a sync strategy. security/scope.Guard
// satisfies this interface structurally (it exposes an identically-shaped
// Dial method), so in production every strategy dials through the pinned-IP
// Guard chokepoint; tests may substitute a plain net.Dialer wrapper against
// a local fixture.
type Dialer interface {
	Dial(ctx context.Context, network, addr string) (net.Conn, error)
}

// PreparedRequest is one Act-Request instance ready to send: the Setup
// phase has already run and its output is bound into Headers/Body
// (Design/DOMAIN.md §Act Request — only the Act Request is duplicated).
type PreparedRequest struct {
	Method  string
	URL     string
	Headers map[string]string
	Body    []byte
}

// Response is one instance's observed response, timestamped at first-byte
// arrival so the caller can measure inter-arrival dispersion
// (Design/DATA_MODEL.md trials.jsonl interarrival_dispersion_ms). Err is set
// instead of a valid StatusCode if the instance's request failed outright
// (dial/write/read error, stream reset).
type Response struct {
	StatusCode int
	Headers    map[string]string
	Body       []byte
	ArrivedAt  time.Time
	Err        error
}

// SyncStrategy releases N prepared Act-Request instances' final bytes/frames
// as close to simultaneously as possible and returns their per-instance
// responses (Design/ARCHITECTURE.md §2.6).
type SyncStrategy interface {
	Burst(ctx context.Context, reqs []PreparedRequest) ([]Response, error)
	Name() string
}

// SendOne sends a single request with nothing to synchronize against — used
// for the Setup phase, sequential Baseline runs, and post-state probes,
// none of which are part of a concurrent burst. It reuses H1LastByte with a
// single instance rather than duplicating request-building/response-reading
// logic.
func SendOne(ctx context.Context, d Dialer, req PreparedRequest) (Response, error) {
	responses, err := (&H1LastByte{Dialer: d}).Burst(ctx, []PreparedRequest{req})
	if err != nil {
		return Response{}, err
	}
	if len(responses) == 0 {
		return Response{}, fmt.Errorf("sync: SendOne got no response")
	}
	return responses[0], nil
}

// Dispersion returns the spread (max-min) of ArrivedAt across responses that
// arrived without error, in milliseconds — the burst-tightness measurement
// recorded per trial (Design/TEST_PLAN.md Layer 2). Returns 0 if fewer than
// two responses arrived successfully.
func Dispersion(responses []Response) float64 {
	var earliest, latest time.Time
	seen := false
	for _, r := range responses {
		if r.Err != nil || r.ArrivedAt.IsZero() {
			continue
		}
		if !seen {
			earliest, latest = r.ArrivedAt, r.ArrivedAt
			seen = true
			continue
		}
		if r.ArrivedAt.Before(earliest) {
			earliest = r.ArrivedAt
		}
		if r.ArrivedAt.After(latest) {
			latest = r.ArrivedAt
		}
	}
	if !seen {
		return 0
	}
	return float64(latest.Sub(earliest).Microseconds()) / 1000.0
}
