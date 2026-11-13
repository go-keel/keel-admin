package service

import (
	"context"
	"net/http"
	"testing"
	"time"

	"github.com/go-kratos/kratos/v3/transport"
)

type cookieTransport struct {
	transport.Transporter
	reply transport.Header
}

func (t *cookieTransport) ReplyHeader() transport.Header { return t.reply }

type cookieHeader http.Header

func (h cookieHeader) Get(key string) string      { return http.Header(h).Get(key) }
func (h cookieHeader) Set(key, value string)      { http.Header(h).Set(key, value) }
func (h cookieHeader) Add(key, value string)      { http.Header(h).Add(key, value) }
func (h cookieHeader) Keys() []string             { return nil }
func (h cookieHeader) Values(key string) []string { return http.Header(h).Values(key) }

func cookieTestContext() (context.Context, http.Header) {
	header := make(http.Header)
	ctx := transport.NewServerContext(context.Background(), &cookieTransport{
		reply: cookieHeader(header),
	})
	return ctx, header
}

func TestSessionCookieUsesConfiguredName(t *testing.T) {
	ctx, header := cookieTestContext()

	setSessionCookie(ctx, "custom_session", "token", time.Hour, true, true)

	cookies := (&http.Response{Header: header}).Cookies()
	if len(cookies) != 1 {
		t.Fatalf("cookies = %d, want 1", len(cookies))
	}
	got := cookies[0]
	if got.Name != "custom_session" {
		t.Fatalf("cookie name = %q, want custom_session", got.Name)
	}
	if got.Value != "token" || got.MaxAge != int(time.Hour.Seconds()) {
		t.Fatalf("cookie = %+v, want configured token and max age", got)
	}
}

func TestClearSessionCookieUsesConfiguredName(t *testing.T) {
	ctx, header := cookieTestContext()

	clearSessionCookie(ctx, "custom_session", true)

	cookies := (&http.Response{Header: header}).Cookies()
	if len(cookies) != 1 {
		t.Fatalf("cookies = %d, want 1", len(cookies))
	}
	got := cookies[0]
	if got.Name != "custom_session" || got.MaxAge != -1 {
		t.Fatalf("cookie = %+v, want custom_session removed", got)
	}
}

func TestNewAuthServiceRequiresCookieName(t *testing.T) {
	if _, err := NewAuthService(nil, nil, nil, nil, time.Hour, "  ", true); err == nil {
		t.Fatal("empty cookie name should fail")
	}
}
