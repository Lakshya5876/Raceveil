package scheduler

import (
	"testing"

	"github.com/Lakshya5876/Raceveil/internal/domain"
)

func TestResolveBinding_Literal(t *testing.T) {
	v, err := resolveBinding(domain.Binding{Name: "X", In: "body", From: "literal:SAVE10"}, nil)
	if err != nil {
		t.Fatalf("resolveBinding: %v", err)
	}
	if v != "SAVE10" {
		t.Errorf("got %q, want SAVE10", v)
	}
}

func TestResolveBinding_FromPriorResponse(t *testing.T) {
	prior := priorResponses{"issue_code": []byte(`{"code":"abc123"}`)}
	v, err := resolveBinding(domain.Binding{Name: "CODE", In: "body", From: "issue_code.$.code"}, prior)
	if err != nil {
		t.Fatalf("resolveBinding: %v", err)
	}
	if v != "abc123" {
		t.Errorf("got %q, want abc123", v)
	}
}

func TestResolveBinding_MissingPriorResponse(t *testing.T) {
	if _, err := resolveBinding(domain.Binding{Name: "CODE", From: "issue_code.$.code"}, priorResponses{}); err == nil {
		t.Fatal("expected an error when the referenced request has no prior response")
	}
}

func TestResolveBinding_UnsupportedSource(t *testing.T) {
	if _, err := resolveBinding(domain.Binding{Name: "X", From: "session.csrf"}, nil); err == nil {
		t.Fatal("expected session.* bindings to be rejected in Phase 1")
	}
}

func TestExtractJSONPath_TopLevelField(t *testing.T) {
	v, err := extractJSONPath([]byte(`{"code":"xyz"}`), "$.code")
	if err != nil {
		t.Fatalf("extractJSONPath: %v", err)
	}
	if v != "xyz" {
		t.Errorf("got %q, want xyz", v)
	}
}

func TestExtractJSONPath_MissingField(t *testing.T) {
	if _, err := extractJSONPath([]byte(`{"other":"xyz"}`), "$.code"); err == nil {
		t.Fatal("expected an error for a missing field")
	}
}

func TestPrepareRequest_ResolvesBindingAndCookies(t *testing.T) {
	spec := domain.RequestSpec{
		ID: "redeem", Method: "POST", URL: "/redeem",
		Headers: map[string]string{"content-type": "application/json"},
		Body:    `{"code":"${CODE}"}`,
		Bindings: []domain.Binding{
			{Name: "CODE", In: "body", From: "issue_code.$.code"},
		},
	}
	sess := domain.Session{Cookies: []domain.Cookie{{Name: "session", Value: "tok"}}}
	prior := priorResponses{"issue_code": []byte(`{"code":"abc123"}`)}
	req, err := prepareRequest(spec, "http://127.0.0.1:18743", sess, prior)
	if err != nil {
		t.Fatalf("prepareRequest: %v", err)
	}
	if req.URL != "http://127.0.0.1:18743/redeem" {
		t.Errorf("URL = %q", req.URL)
	}
	if string(req.Body) != `{"code":"abc123"}` {
		t.Errorf("Body = %q, want code substituted", req.Body)
	}
	if req.Headers["Cookie"] != "session=tok" {
		t.Errorf("Cookie header = %q, want session=tok", req.Headers["Cookie"])
	}
}
