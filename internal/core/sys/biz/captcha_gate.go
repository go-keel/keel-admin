package biz

import (
	"context"
	"log/slog"

	"github.com/go-keel/keel-admin/internal/core/captcha"
)

// CaptchaGate is the login captcha admission port consumed by Login's step
// 0. Injected as a small interface — not the captcha.Provider global
// directly — so tests can control Enabled/Verify without touching the
// process-wide captcha registry. See Login's doc comment for the
// fail-closed semantics of a false/error Verify result.
type CaptchaGate interface {
	// Enabled reports whether captcha is required for login.
	Enabled(ctx context.Context) bool
	// Verify checks a challenge answer against the configured provider. Any
	// non-true result — false with no error, or a non-nil error — means
	// "not verified".
	Verify(ctx context.Context, id, answer, remoteIP string) (bool, error)
}

// captchaGate is the production CaptchaGate: enabled flag + active provider
// name come from startup config (app.yaml sys.captcha), then delegate to the
// captcha package's global registry (see di.go for provider registration).
type captchaGate struct {
	enabled  bool
	provider string
}

// NewCaptchaGate builds the production CaptchaGate from startup config.
func NewCaptchaGate(enabled bool, provider string) CaptchaGate {
	return captchaGate{enabled: enabled, provider: provider}
}

func (g captchaGate) Enabled(_ context.Context) bool { return g.enabled }

// Verify resolves the configured provider and delegates to it. A provider
// name that isn't registered (assembly/ops misconfiguration, not a user
// error) fails closed: (false, nil), same treatment Login gives a wrong
// answer — captcha enabled but unusable must not degrade to "no captcha".
func (g captchaGate) Verify(ctx context.Context, id, answer, remoteIP string) (bool, error) {
	p, ok := captcha.Get(g.provider)
	if !ok {
		slog.WarnContext(ctx, "captcha provider not registered, failing closed", "provider", g.provider)
		return false, nil
	}
	return p.Verify(ctx, id, answer, remoteIP)
}
