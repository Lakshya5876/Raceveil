package scheduler

import (
	"math/rand"
	"time"

	"testing"

	"github.com/Lakshya5876/Raceveil/internal/application/oracle"
	"github.com/Lakshya5876/Raceveil/internal/infrastructure/sync"
)

// TEST_PLAN.md Layer 4: "inject noisy responses, changing response order,
// latency jitter, occasional 5xx; assert the oracle's baseline-variability
// calibration absorbs it without firing." summarizeResponses is where a
// concurrent burst's raw responses become the S count Evaluate reasons
// about, so this is where that robustness has to actually hold.

var (
	successRule = &oracle.Rule{Status: 200}
	rejectRule  = &oracle.Rule{Status: 409}
)

// TestSummarizeResponses_OrderIndependent asserts the summary a burst
// produces does not depend on the order responses arrived in — a race
// detector cannot afford to be sensitive to network reordering, since that
// would make S (and therefore the violation verdict) a function of jitter
// rather than of what the server actually did.
func TestSummarizeResponses_OrderIndependent(t *testing.T) {
	base := []sync.Response{
		{StatusCode: 200}, {StatusCode: 200}, {StatusCode: 409},
		{StatusCode: 500}, {StatusCode: 200}, {Err: errTimeout},
	}
	_, wantS := summarizeResponses(base, successRule, rejectRule)

	rng := rand.New(rand.NewSource(42))
	for trial := 0; trial < 20; trial++ {
		shuffled := append([]sync.Response(nil), base...)
		rng.Shuffle(len(shuffled), func(i, j int) { shuffled[i], shuffled[j] = shuffled[j], shuffled[i] })
		_, s := summarizeResponses(shuffled, successRule, rejectRule)
		if s != wantS {
			t.Fatalf("shuffled order changed S: got %d, want %d (order must not affect the violation count)", s, wantS)
		}
	}
}

// TestSummarizeResponses_UnexpectedStatusesNeverCountAsSuccess is the
// "noisy responses / occasional 5xx" half of Layer 4: a 5xx, a 3xx redirect,
// or any status outside the declared/learned success signature must never
// be miscounted toward S — only an exact match against the calibrated
// success rule may increase it. An oracle that let noise inflate S would
// report violations that never happened.
func TestSummarizeResponses_UnexpectedStatusesNeverCountAsSuccess(t *testing.T) {
	noisy := []sync.Response{
		{StatusCode: 200}, // the one genuine success
		{StatusCode: 503}, // noise: transient server error
		{StatusCode: 502}, // noise: bad gateway
		{StatusCode: 301}, // noise: unrelated redirect
		{StatusCode: 418}, // noise: nonsense status
		{Err: errTimeout}, // noise: network-level failure, no status at all
	}
	summary, s := summarizeResponses(noisy, successRule, rejectRule)
	if s != 1 {
		t.Fatalf("S = %d, want 1 (only the genuine 200 should count; noise must not inflate S)", s)
	}
	if summary.Success != 1 {
		t.Errorf("summary.Success = %d, want 1", summary.Success)
	}
	if summary.Error != 5 {
		t.Errorf("summary.Error = %d, want 5 (every non-matching status and the timeout fall to Error, none silently become Success or Reject)", summary.Error)
	}
	if summary.Reject != 0 {
		t.Errorf("summary.Reject = %d, want 0 (none of the noisy statuses match the 409 reject rule)", summary.Reject)
	}
}

// TestSummarizeResponses_LatencyJitterDoesNotAffectClassification proves
// that per-response timing (ArrivedAt / interarrival dispersion, which the
// scheduler records separately as InterarrivalDispersionMs) plays no part
// in how a response is classified — only StatusCode/Body do. Latency jitter
// is real network behavior RaceVeil must tolerate, not evidence a violation
// occurred.
func TestSummarizeResponses_LatencyJitterDoesNotAffectClassification(t *testing.T) {
	base := time.Now()
	jittered := []sync.Response{
		{StatusCode: 200, ArrivedAt: base},
		{StatusCode: 200, ArrivedAt: base.Add(3 * time.Millisecond)},
		{StatusCode: 409, ArrivedAt: base.Add(47 * time.Millisecond)}, // a straggler
	}
	tight := []sync.Response{
		{StatusCode: 200, ArrivedAt: base},
		{StatusCode: 200, ArrivedAt: base.Add(10 * time.Microsecond)},
		{StatusCode: 409, ArrivedAt: base.Add(20 * time.Microsecond)},
	}
	_, sJittered := summarizeResponses(jittered, successRule, rejectRule)
	_, sTight := summarizeResponses(tight, successRule, rejectRule)
	if sJittered != sTight {
		t.Fatalf("S differed by arrival timing alone: jittered=%d tight=%d, want equal", sJittered, sTight)
	}
}

var errTimeout = &timeoutErr{}

type timeoutErr struct{}

func (*timeoutErr) Error() string { return "simulated network timeout" }
