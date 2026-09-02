package discovery

import (
	"testing"

	"github.com/Lakshya5876/Raceveil/internal/domain"
)

func captured(id, method, path, body string) domain.CapturedRequest {
	return domain.CapturedRequest{
		ID:       id,
		Endpoint: domain.Endpoint{Method: method, Path: path},
		URL:      path,
		Body:     body,
	}
}

func TestExtractWorkflows_OnlyMutatingRequests(t *testing.T) {
	requests := []domain.CapturedRequest{
		captured("r1", "POST", "/coupon/redeem", ""),
		captured("r2", "GET", "/account", ""),
		captured("r3", "PUT", "/order/1", ""),
		captured("r4", "HEAD", "/health", ""),
	}
	got := ExtractWorkflows(requests)
	if len(got) != 2 {
		t.Fatalf("expected 2 workflows (POST, PUT only), got %d: %+v", len(got), got)
	}
	for _, wf := range got {
		if wf.ActRequest == "r2" || wf.ActRequest == "r4" {
			t.Errorf("read-only request %q must never become an Act Request", wf.ActRequest)
		}
	}
}

func TestIsPreconditionFailure_VsLimitSignal(t *testing.T) {
	cases := []struct {
		name        string
		status      int
		body        string
		wantPrecond bool
		wantLimit   bool
	}{
		{"409 already redeemed is a limit", 409, "already redeemed", false, true},
		{"429 rate limited is a limit", 429, "too many requests", false, true},
		{"404 missing cart is a precondition", 404, "no cart found", true, false},
		{"422 unprocessable is a precondition", 422, "field required", true, false},
		{"200 is neither", 200, "ok", false, false},
		{"500 is neither", 500, "boom", false, false},
		{"400 with limit wording is a limit, not a precondition", 400, "coupon already used", false, true},
		{"403 out of stock is a limit", 403, "out of stock", false, true},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if got := IsPreconditionFailure(c.status, c.body); got != c.wantPrecond {
				t.Errorf("IsPreconditionFailure = %v, want %v", got, c.wantPrecond)
			}
			if got := IsLimitSignal(c.status, c.body); got != c.wantLimit {
				t.Errorf("IsLimitSignal = %v, want %v", got, c.wantLimit)
			}
		})
	}
}

func TestFindSetupCandidates_PrefersSiblingCreationEndpoint(t *testing.T) {
	act := captured("act", "POST", "/coupon/redeem", `{"code":"${CODE}"}`)
	all := []domain.CapturedRequest{
		act,
		captured("issue", "POST", "/coupon/issue", ""),
		captured("unrelated", "POST", "/newsletter/subscribe", ""),
		captured("readonly", "GET", "/coupon/list", ""),
	}
	got := FindSetupCandidates(act, all)
	if len(got) == 0 {
		t.Fatal("expected at least one setup candidate")
	}
	if got[0].ID != "issue" {
		t.Errorf("best setup candidate = %q, want the sibling creation endpoint %q", got[0].ID, "issue")
	}
	for _, c := range got {
		if c.ID == "readonly" {
			t.Error("a read-only request can never be a setup candidate")
		}
		if c.ID == "act" {
			t.Error("the Act Request must not be its own setup")
		}
	}
}

func TestAttachSetup_WiresBindingIntoActRequest(t *testing.T) {
	act := captured("act", "POST", "/coupon/redeem", `{"code":"${CODE}"}`)
	wf := ExtractWorkflows([]domain.CapturedRequest{act})[0]
	setup := captured("issue", "POST", "/coupon/issue", "")

	got := AttachSetup(wf, setup, "code")

	if len(got.SetupRequests) != 1 || got.SetupRequests[0] != "issue" {
		t.Fatalf("SetupRequests = %v, want [issue]", got.SetupRequests)
	}
	if len(got.Requests) != 2 || got.Requests[0].ID != "issue" {
		t.Fatalf("setup must be prepended to Requests, got %+v", got.Requests)
	}
	actSpec, ok := got.RequestByID("act")
	if !ok {
		t.Fatal("act request missing after AttachSetup")
	}
	if len(actSpec.Bindings) != 1 {
		t.Fatalf("expected one binding wired into the Act Request, got %+v", actSpec.Bindings)
	}
	b := actSpec.Bindings[0]
	if b.Name != "CODE" || b.From != "issue.$.code" || b.In != "body" {
		t.Errorf("binding = %+v, want CODE from issue.$.code in body", b)
	}
}

func TestAttachSetup_NoPlaceholderMeansNoBinding(t *testing.T) {
	act := captured("act", "POST", "/cart/checkout", `{"confirm":true}`)
	wf := ExtractWorkflows([]domain.CapturedRequest{act})[0]
	setup := captured("create", "POST", "/cart/create", "")

	got := AttachSetup(wf, setup, "cartId")

	actSpec, _ := got.RequestByID("act")
	if len(actSpec.Bindings) != 0 {
		t.Errorf("no ${PLACEHOLDER} in the body means no binding should be invented, got %+v", actSpec.Bindings)
	}
	if len(got.SetupRequests) != 1 {
		t.Error("setup should still be attached even without a data binding")
	}
}

// An API that exposes parallel variants of one resource must never have an
// Act Request paired with the *other* variant's setup: that mints state the
// Act Request cannot use, and the endpoint then looks — wrongly — like it
// has no invariant. Found against the real corpus, where /coupon/redeem-safe
// was being fed a code minted by the vulnerable /coupon/issue.
func TestFindSetupCandidates_RespectsResourceVariants(t *testing.T) {
	all := []domain.CapturedRequest{
		captured("issue", "POST", "/coupon/issue", ""),
		captured("issue_safe", "POST", "/coupon/issue-safe", ""),
		captured("redeem", "POST", "/coupon/redeem", `{"code":"x"}`),
		captured("redeem_safe", "POST", "/coupon/redeem-safe", `{"code":"x"}`),
	}

	safe := FindSetupCandidates(all[3], all)
	if len(safe) == 0 || safe[0].ID != "issue_safe" {
		t.Errorf("the safe Act Request must use the safe setup, got %+v", safe)
	}
	for _, c := range safe {
		if c.ID == "issue" {
			t.Error("a safe Act Request must never be paired with the vulnerable variant's setup")
		}
	}

	vuln := FindSetupCandidates(all[2], all)
	if len(vuln) == 0 || vuln[0].ID != "issue" {
		t.Errorf("the base Act Request must use the base setup, got %+v", vuln)
	}
	for _, c := range vuln {
		if c.ID == "issue_safe" {
			t.Error("the base Act Request must never be paired with a -safe variant setup")
		}
	}
}

// A hyphen is only a variant marker when the API actually uses it as one:
// /issue-code is a name, not a "code variant", because no /issue exists
// beside it.
func TestVariantMarkers_OnlyCountsRealVariants(t *testing.T) {
	realVariants := variantMarkers([]domain.CapturedRequest{
		captured("a", "POST", "/coupon/redeem", ""),
		captured("b", "POST", "/coupon/redeem-safe", ""),
	})
	if !realVariants["safe"] {
		t.Error(`"safe" should be recognised as a variant marker when both /redeem and /redeem-safe exist`)
	}

	notVariants := variantMarkers([]domain.CapturedRequest{
		captured("a", "POST", "/issue-code", ""),
		captured("b", "POST", "/redeem", ""),
	})
	if notVariants["code"] {
		t.Error(`"code" must not be treated as a variant marker: no /issue exists beside /issue-code`)
	}
}

// Variants belong to one resource. Comparing bare last segments would let
// /signup/issue-username pair off against an unrelated /coupon/issue and
// invent a "username" variant, which then filtered away the real setup and
// silently lost a finding. Found against the live corpus.
func TestVariantMarkers_AreScopedToTheSameResource(t *testing.T) {
	markers := variantMarkers([]domain.CapturedRequest{
		captured("a", "POST", "/coupon/issue", ""),
		captured("b", "POST", "/coupon/issue-safe", ""),
		captured("c", "POST", "/signup/issue-username", ""),
		captured("d", "POST", "/signup", ""),
		captured("e", "POST", "/signup-safe", ""),
	})
	if !markers["safe"] {
		t.Error(`"safe" is a real variant marker here (/coupon/issue and /coupon/issue-safe both exist)`)
	}
	if markers["username"] {
		t.Error(`"username" must not be a variant marker: there is no /signup/issue beside /signup/issue-username`)
	}
}

func TestFindSetupCandidates_KeepsSetupWithUnrelatedHyphenatedName(t *testing.T) {
	all := []domain.CapturedRequest{
		captured("issue_username", "POST", "/signup/issue-username", ""),
		captured("signup", "POST", "/signup", `{"username":"x"}`),
		captured("signup_safe", "POST", "/signup-safe", `{"username":"x"}`),
		captured("coupon_issue", "POST", "/coupon/issue", ""),
	}
	got := FindSetupCandidates(all[1], all)
	if len(got) == 0 || got[0].ID != "issue_username" {
		t.Errorf("expected /signup/issue-username to remain the setup for /signup, got %+v", got)
	}
}
