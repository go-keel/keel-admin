package turnstile

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// newTestProvider points the provider at a fake siteverify, so no real network is needed.
func newTestProvider(t *testing.T, h http.HandlerFunc) *provider {
	t.Helper()
	srv := httptest.NewServer(h)
	t.Cleanup(srv.Close)
	p, cleanup, err := newProvider(srv.URL, "test-secret")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(cleanup)
	return p
}

func TestVerifySuccess(t *testing.T) {
	var got struct{ secret, response, remoteip string }
	p := newTestProvider(t, func(w http.ResponseWriter, r *http.Request) {
		_ = r.ParseForm()
		got.secret = r.PostFormValue("secret")
		got.response = r.PostFormValue("response")
		got.remoteip = r.PostFormValue("remoteip")
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"success":true}`))
	})

	ok, err := p.Verify(context.Background(), "", "widget-token", "203.0.113.7")
	if err != nil || !ok {
		t.Fatalf("success should pass: ok=%v err=%v", ok, err)
	}
	// remoteip must be sent: Cloudflare binds the token to its source by it, and leaving it out silently weakens the
	// check (the token is still "valid", just no longer limited to its source).
	if got.secret != "test-secret" || got.response != "widget-token" || got.remoteip != "203.0.113.7" {
		t.Fatalf("wrong form sent: %+v", got)
	}
}

func TestVerifyFailure(t *testing.T) {
	p := newTestProvider(t, func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"success":false,"error-codes":["invalid-input-response"]}`))
	})

	ok, err := p.Verify(context.Background(), "", "bad-token", "")
	if err != nil {
		t.Fatalf("a failed verdict is not an error: %v", err)
	}
	if ok {
		t.Fatal("success:false should fail")
	}
}

// "Never asked" must stay apart from "asked and failed": the caller fails closed on both, but once they are one
// value the log cannot tell whether Cloudflare was unreachable or someone was guessing codes.
func TestVerifyUpstreamErrorIsError(t *testing.T) {
	p := newTestProvider(t, func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusBadGateway)
	})

	ok, err := p.Verify(context.Background(), "", "token", "")
	if err == nil {
		t.Fatal("an upstream 5xx should return an error, not (false, nil)")
	}
	if ok {
		t.Fatal("must not pass on error")
	}
}

func TestReadyRequiresSecret(t *testing.T) {
	p := newTestProvider(t, func(http.ResponseWriter, *http.Request) {})
	if err := p.Ready(); err != nil {
		t.Fatalf("should be ready with a secret: %v", err)
	}

	p.secret = ""
	err := p.Ready()
	if err == nil {
		t.Fatal("a missing secret should be an error, otherwise the captcha is silently turned off")
	}
	if !strings.Contains(err.Error(), "secret_key") {
		t.Fatalf("the error should name the missing config key: %v", err)
	}
}

func TestIssueReturnsNoChallenge(t *testing.T) {
	p := newTestProvider(t, func(http.ResponseWriter, *http.Request) {})
	c, err := p.Issue(context.Background())
	if err != nil || c != nil {
		t.Fatalf("the frontend renders the widget, so the server has no challenge to issue: c=%v err=%v", c, err)
	}
}
