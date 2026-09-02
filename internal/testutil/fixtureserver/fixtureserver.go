// Package fixtureserver is a local HTTP fixture used only by RaceVeil's own
// test suite and manual Phase-1 demo runs — test-only scaffolding, never
// reachable from the shipped CLI binary path (raceveil.exe never imports
// this package).
//
// It exposes two behaviourally-identical endpoint pairs on 127.0.0.1:
//   - Vulnerable: POST /issue-code, POST /redeem — a classic check-then-act
//     coupon redemption race (the check and the act each take the mutex
//     separately, leaving a window between them).
//   - Safe twin: POST /issue-code-safe, POST /redeem-safe — the same
//     behaviour with the check and act performed under one held lock, so
//     concurrent redemptions of the same code cannot both succeed.
package fixtureserver

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"net/http"
	"sync"
	"time"
)

// Server is the fixture's HTTP server plus its two independent code stores.
type Server struct {
	addr string
	srv  *http.Server
	ln   net.Listener

	vulnMu       sync.Mutex
	vulnCodes    map[string]bool // code -> unused
	vulnRedeemed int             // persisted count of successful vulnerable redemptions

	safeMu       sync.Mutex
	safeCodes    map[string]bool // code -> unused
	safeRedeemed int             // persisted count of successful safe redemptions
}

// New builds a Server bound to addr (e.g. "127.0.0.1:18743"). It does not
// start listening until Start is called.
func New(addr string) *Server {
	s := &Server{
		addr:      addr,
		vulnCodes: make(map[string]bool),
		safeCodes: make(map[string]bool),
	}
	mux := http.NewServeMux()
	mux.HandleFunc("/issue-code", s.handleIssue(&s.vulnMu, s.vulnCodes))
	mux.HandleFunc("/redeem", s.handleRedeemVulnerable)
	mux.HandleFunc("/redemptions", s.handleRedemptionsProbe(&s.vulnMu, &s.vulnRedeemed))
	mux.HandleFunc("/issue-code-safe", s.handleIssue(&s.safeMu, s.safeCodes))
	mux.HandleFunc("/redeem-safe", s.handleRedeemSafe)
	mux.HandleFunc("/redemptions-safe", s.handleRedemptionsProbe(&s.safeMu, &s.safeRedeemed))
	s.srv = &http.Server{Handler: mux, ReadHeaderTimeout: 5 * time.Second}
	return s
}

// Start binds the listener and begins serving in the background. If the
// port is still held by a just-stopped previous run (Windows TIME_WAIT), it
// retries briefly before failing with a clear error rather than the raw
// "address already in use".
func (s *Server) Start() error {
	var ln net.Listener
	var err error
	for attempt := 0; attempt < 5; attempt++ {
		ln, err = net.Listen("tcp", s.addr)
		if err == nil {
			break
		}
		time.Sleep(200 * time.Millisecond)
	}
	if err != nil {
		return fmt.Errorf("fixtureserver: %s still in use after retries — stop any prior run first: %w", s.addr, err)
	}
	s.ln = ln
	go func() {
		_ = s.srv.Serve(ln)
	}()
	return nil
}

// Stop shuts the server down cleanly, draining in-flight requests.
func (s *Server) Stop(ctx context.Context) error {
	if s.srv == nil {
		return nil
	}
	return s.srv.Shutdown(ctx)
}

// Addr returns the bound listener address (useful when addr:0 was passed).
func (s *Server) Addr() string {
	if s.ln != nil {
		return s.ln.Addr().String()
	}
	return s.addr
}

type issueResponse struct {
	Code string `json:"code"`
}

func (s *Server) handleIssue(mu *sync.Mutex, codes map[string]bool) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}
		code, err := randomCode()
		if err != nil {
			http.Error(w, "internal error", http.StatusInternalServerError)
			return
		}
		mu.Lock()
		codes[code] = true
		mu.Unlock()
		writeJSON(w, http.StatusOK, issueResponse{Code: code})
	}
}

type redeemRequest struct {
	Code string `json:"code"`
}

type redeemResponse struct {
	Status    string `json:"status"`
	ReceiptID string `json:"receipt_id"`
}

type redemptionsResponse struct {
	Count int `json:"count"`
}

func decodeRedeem(r *http.Request) (string, error) {
	var body redeemRequest
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		return "", errors.New("invalid request body")
	}
	return body.Code, nil
}

// handleRedeemVulnerable is the intentionally vulnerable check-then-act
// redemption: the validity check and the used-marking each take the lock
// separately, leaving a window in which two concurrent requests for the
// same code can both observe it as unused. The sleep between check and act
// widens that window to a size reliably larger than typical scheduler/timer
// jitter, so the race triggers consistently across repeated trials — it
// does not change what makes the endpoint vulnerable (the lock is still
// released between the two steps).
func (s *Server) handleRedeemVulnerable(w http.ResponseWriter, r *http.Request) {
	code, err := decodeRedeem(r)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	s.vulnMu.Lock()
	valid := s.vulnCodes[code]
	s.vulnMu.Unlock()
	if !valid {
		http.Error(w, "already used", http.StatusConflict)
		return
	}
	time.Sleep(3 * time.Millisecond)
	receipt, err := randomCode()
	if err != nil {
		http.Error(w, "internal error", http.StatusInternalServerError)
		return
	}
	s.vulnMu.Lock()
	s.vulnCodes[code] = false
	s.vulnRedeemed++
	s.vulnMu.Unlock()
	writeJSON(w, http.StatusOK, redeemResponse{Status: "redeemed", ReceiptID: receipt})
}

// handleRedeemSafe performs the identical check-then-act under one held
// lock, so it cannot be raced: only the first of any concurrent redemptions
// of the same code observes it as valid.
func (s *Server) handleRedeemSafe(w http.ResponseWriter, r *http.Request) {
	code, err := decodeRedeem(r)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	receipt, err := randomCode()
	if err != nil {
		http.Error(w, "internal error", http.StatusInternalServerError)
		return
	}
	s.safeMu.Lock()
	valid := s.safeCodes[code]
	if valid {
		s.safeCodes[code] = false
		s.safeRedeemed++
	}
	s.safeMu.Unlock()
	if !valid {
		http.Error(w, "already used", http.StatusConflict)
		return
	}
	writeJSON(w, http.StatusOK, redeemResponse{Status: "redeemed", ReceiptID: receipt})
}

// handleRedemptionsProbe is the Level 4 read-only post-state probe: it
// returns the persisted count of successful redemptions so far, letting the
// Oracle corroborate a Level-3 violation via persisted state instead of
// just response classification (Design/ARCHITECTURE.md §4).
func (s *Server) handleRedemptionsProbe(mu *sync.Mutex, count *int) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}
		mu.Lock()
		n := *count
		mu.Unlock()
		writeJSON(w, http.StatusOK, redemptionsResponse{Count: n})
	}
}

func randomCode() (string, error) {
	b := make([]byte, 8)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	return hex.EncodeToString(b), nil
}

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}
