package service

import (
	"context"
	"strings"
	"testing"

	kerrors "github.com/go-kratos/kratos/v3/errors"

	"github.com/go-keel/keel-admin/internal/core/perm"
	"github.com/go-keel/keel-admin/internal/i18n"
	"github.com/go-keel/keel/middleware/locale"
	"github.com/go-keel/keel/middleware/metadata"
)

// The IP allowlist rejection message must carry the request's client IP, so the user can ask an administrator to
// add that address. It renders the real zh-CN message, so a misspelled template variable in the message is caught
// here too; without a localizer the English text carries the IP as well.
func TestIPNotAllowedMessageCarriesRemoteIP(t *testing.T) {
	ctx, err := metadata.Set(context.Background(), metadata.KeyRemoteAddress, "203.0.113.7")
	if err != nil {
		t.Fatal(err)
	}
	zhCtx, err := metadata.Set(ctx, metadata.KeyAcceptLanguage, "zh-CN")
	if err != nil {
		t.Fatal(err)
	}
	h := func(ctx context.Context, _ any) (any, error) { return nil, ipNotAllowed(ctx, nil) }

	_, zhErr := locale.Server(i18n.NewBundle())(h)(zhCtx, nil)
	_, enErr := h(ctx, nil)
	for name, err := range map[string]error{"zh-CN": zhErr, "no localizer": enErr} {
		msg := kerrors.FromError(err).Message
		if !strings.Contains(msg, "203.0.113.7") || strings.Contains(msg, "{{") {
			t.Errorf("%s: the message should carry the client IP, got %q", name, msg)
		}
	}
	if kerrors.FromError(zhErr).Message == kerrors.FromError(enErr).Message {
		t.Errorf("zh-CN message is the English one: %q", kerrors.FromError(zhErr).Message)
	}
}

// A use case reached without a principal answers "sign in first" (401 UNAUTHENTICATED, like the authz middleware),
// never the login page's "invalid username or password".
func TestNoPrincipalMapsToUnauthenticated(t *testing.T) {
	h := locale.Server(i18n.NewBundle())(NewErrorSentinel().Middleware()(func(ctx context.Context, _ any) (any, error) {
		return perm.Current(ctx)
	}))
	_, err := h(context.Background(), nil)
	if e := kerrors.FromError(err); e.Code != 401 || e.Reason != "UNAUTHENTICATED" {
		t.Fatalf("got %d %s, want 401 UNAUTHENTICATED", e.Code, e.Reason)
	}
}
