package fixtureserver

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"sync"
	"testing"
	"time"
)

func startTestServer(t *testing.T) (*Server, string) {
	t.Helper()
	s := New("127.0.0.1:0")
	if err := s.Start(); err != nil {
		t.Fatalf("Start: %v", err)
	}
	t.Cleanup(func() {
		ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
		defer cancel()
		_ = s.Stop(ctx)
	})
	return s, "http://" + s.Addr()
}

func issueCode(t *testing.T, base, path string) string {
	t.Helper()
	resp, err := http.Post(base+path, "application/json", nil)
	if err != nil {
		t.Fatalf("issue %s: %v", path, err)
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("issue %s: status = %d, want 200", path, resp.StatusCode)
	}
	var body issueResponse
	if err := json.NewDecoder(resp.Body).Decode(&body); err != nil {
		t.Fatalf("issue %s: decode: %v", path, err)
	}
	if body.Code == "" {
		t.Fatalf("issue %s: empty code", path)
	}
	return body.Code
}

func redeem(base, path, code string) (int, error) {
	body, _ := json.Marshal(redeemRequest{Code: code})
	resp, err := http.Post(base+path, "application/json", bytes.NewReader(body))
	if err != nil {
		return 0, err
	}
	defer func() { _ = resp.Body.Close() }()
	return resp.StatusCode, nil
}

func TestIssueCode_ReturnsUniqueCodes(t *testing.T) {
	_, base := startTestServer(t)
	a := issueCode(t, base, "/issue-code")
	b := issueCode(t, base, "/issue-code")
	if a == b {
		t.Fatalf("expected distinct codes, got %q twice", a)
	}
}

func TestRedeemVulnerable_SequentialOnlyAllowsOnce(t *testing.T) {
	_, base := startTestServer(t)
	code := issueCode(t, base, "/issue-code")
	status1, err := redeem(base, "/redeem", code)
	if err != nil {
		t.Fatalf("redeem 1: %v", err)
	}
	status2, err := redeem(base, "/redeem", code)
	if err != nil {
		t.Fatalf("redeem 2: %v", err)
	}
	if status1 != http.StatusOK {
		t.Errorf("first sequential redeem = %d, want 200", status1)
	}
	if status2 != http.StatusConflict {
		t.Errorf("second sequential redeem = %d, want 409 (baseline must show the limit)", status2)
	}
}

func TestRedeemVulnerable_ConcurrentAllowsRace(t *testing.T) {
	_, base := startTestServer(t)
	code := issueCode(t, base, "/issue-code")

	const n = 5
	statuses := make([]int, n)
	var wg sync.WaitGroup
	for i := 0; i < n; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			status, err := redeem(base, "/redeem", code)
			if err != nil {
				t.Errorf("redeem instance %d: %v", i, err)
				return
			}
			statuses[i] = status
		}(i)
	}
	wg.Wait()

	successes := countStatus(statuses, http.StatusOK)
	if successes < 2 {
		t.Fatalf("expected the unlocked check-then-act window to let >=2 concurrent redemptions succeed, got %d of %d (statuses=%v)", successes, n, statuses)
	}
}

func TestRedeemSafe_ConcurrentNeverExceedsOne(t *testing.T) {
	_, base := startTestServer(t)
	code := issueCode(t, base, "/issue-code-safe")

	const n = 10
	statuses := make([]int, n)
	var wg sync.WaitGroup
	for i := 0; i < n; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			status, err := redeem(base, "/redeem-safe", code)
			if err != nil {
				t.Errorf("redeem-safe instance %d: %v", i, err)
				return
			}
			statuses[i] = status
		}(i)
	}
	wg.Wait()

	successes := countStatus(statuses, http.StatusOK)
	if successes != 1 {
		t.Fatalf("safe twin must allow exactly 1 success under concurrency, got %d of %d (statuses=%v)", successes, n, statuses)
	}
}

func countStatus(statuses []int, want int) int {
	n := 0
	for _, s := range statuses {
		if s == want {
			n++
		}
	}
	return n
}
