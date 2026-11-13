package service

import (
	"context"
	"strings"
	"testing"

	"github.com/BurntSushi/toml"
	kerrors "github.com/go-kratos/kratos/v3/errors"
	kmd "github.com/go-kratos/kratos/v3/metadata"
	goi18n "github.com/nicksnyder/go-i18n/v2/i18n"
	"golang.org/x/text/language"

	"github.com/go-keel/keel/middleware/metadata"
)

// The IP allowlist rejection message must carry the request's client IP, so the user can ask an administrator to
// add that address. It renders the real zh-CN message, so a misspelled template variable in the message is caught
// here too.
func TestIPNotAllowedMessageCarriesRemoteIP(t *testing.T) {
	b := goi18n.NewBundle(language.English)
	b.RegisterUnmarshalFunc("toml", toml.Unmarshal)
	if _, err := b.LoadMessageFile("../../../i18n/sys.zh-CN.toml"); err != nil {
		t.Fatalf("load zh-CN: %v", err)
	}
	ctx := kmd.NewServerContext(context.Background(), kmd.Metadata{metadata.KeyRemoteAddress: {"203.0.113.7"}})

	err := ipNotAllowed(ctx, goi18n.NewLocalizer(b, "zh-CN"), nil)

	msg := kerrors.FromError(err).Message
	if !strings.Contains(msg, "203.0.113.7") {
		t.Fatalf("the message should carry the client IP, got %q", msg)
	}
}
