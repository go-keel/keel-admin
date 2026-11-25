package server

import (
	"crypto/sha256"
	"fmt"
	"time"

	"github.com/go-keel/keel-admin/internal/core/errs"

	"github.com/go-keel/keel-admin/internal/conf"
	"github.com/go-keel/keel-admin/internal/core/captcha"
	base64captcha "github.com/go-keel/keel-admin/internal/core/captcha/base64"
	"github.com/go-keel/keel-admin/internal/core/captcha/turnstile"
	"github.com/go-keel/keel-admin/internal/core/sys/biz"
	"github.com/go-keel/keel-admin/internal/core/sys/totp"
	"github.com/go-keel/keel/core/kconfig"
	"github.com/go-keel/keel/core/kredis"
	"github.com/go-keel/keel/pkg/jwt"
)

// provideBusinessZone loads sys.business_time_zone (required by its rule in app.proto); a name the Go runtime does not
// know fails startup.
func provideBusinessZone(s *conf.Sys) (*time.Location, error) {
	name := s.GetBusinessTimeZone()
	loc, err := time.LoadLocation(name)
	if err != nil {
		return nil, errs.Rules.Wrap(err, "sys.business-zone-invalid", fmt.Sprintf("sys.business_time_zone %q is not a known time zone (%v)", name, err), "use an IANA zone name such as Asia/Shanghai")
	}
	return loc, nil
}

// provideAuthUseCase assembles the auth use case. The login JWT is signed with
// the SAME secret Keel's auth middleware verifies with (Server.Http auth config),
// so there is a single source of truth for the token secret. The lockout + captcha
// config comes from app.yaml sys.* (startup, static).
//
// An empty secret returns an error and the service does not start: anyone could forge login tokens signed with an
// empty secret.
func provideAuthUseCase(users biz.UserRepo, ll biz.LoginLogger, cache biz.PermCache, gate biz.OTPGate, s *conf.Sys, kc *kconfig.Config, guard biz.LockoutGuard, captchaProvider captcha.Provider) (*biz.AuthUseCase, error) {
	signer, err := jwt.NewHS256Signer([]byte(serverAuthSecret(kc)))
	if err != nil {
		return nil, errs.Rules.Wrap(err, "sys.login-signer", fmt.Sprintf("cannot build the login token signer from server.http.default.auth.jwt_secret (%v)", err), "set server.http.default.auth.jwt_secret to a non-empty secret")
	}
	cfg := biz.AuthConfig{
		CaptchaProvider:    s.GetCaptcha().GetProvider(),
		CaptchaSiteKey:     captchaSiteKey(s.GetCaptcha()),
		LockoutEnabled:     s.GetLockout().GetEnabled(),
		LockoutMaxFailures: s.GetLockout().GetMaxFailures(),
		LockoutLockMinutes: s.GetLockout().GetLockMinutes(),
		LockoutScope:       s.GetLockout().GetScope(),
	}
	return biz.NewAuthUseCase(users, ll, cache, gate, signer, s.GetSessionTtl().AsDuration(), guard, cfg, captchaProvider), nil
}

// newCaptchaProvider builds the captcha provider sys.captcha.provider names, or returns nil when sys.captcha.enabled is
// false (login then skips the captcha, and the anonymous captcha endpoint issues nothing). The selected provider must
// pass its self-check here: without it, turnstile missing a secret would show up as "every login is rejected", with
// no clue why. Only the built-in providers are supported: the login page renders only them.
func newCaptchaProvider(c *conf.Captcha, rdb *kredis.Redis) (captcha.Provider, func(), error) {
	noop := func() {}
	if !c.GetEnabled() {
		return nil, noop, nil
	}
	var p captcha.Provider
	cleanup := noop
	switch c.GetProvider() {
	case base64captcha.Name:
		p = base64captcha.New(rdb, int(c.GetBase64Length()), c.GetBase64Ttl().AsDuration())
	case turnstile.Name:
		var err error
		if p, cleanup, err = turnstile.New(c.GetTurnstile().GetSecretKey()); err != nil {
			return nil, noop, err
		}
	default:
		return nil, noop, errs.Rules.New("captcha.provider-unknown",
			fmt.Sprintf("sys.captcha.provider %q is not a built-in provider (%s, %s)", c.GetProvider(), base64captcha.Name, turnstile.Name),
			"set sys.captcha.provider to base64 or turnstile")
	}
	if err := p.Ready(); err != nil {
		cleanup()
		return nil, noop, errs.Rules.Wrap(err, "captcha.provider-not-ready",
			fmt.Sprintf("sys.captcha.provider %q is not ready: %v", c.GetProvider(), err),
			"supply the provider's settings named above, or set sys.captcha.enabled to false")
	}
	return p, cleanup, nil
}

// captchaSiteKey returns a value only when a widget provider is selected: the site key is provider-specific, and
// sending an unused key for the image code would only make the login page guess what to render. The page looks at
// captchaEnabled first, so the key is not gated on it here.
func captchaSiteKey(c *conf.Captcha) string {
	if c.GetProvider() != turnstile.Name {
		return ""
	}
	return c.GetTurnstile().GetSiteKey()
}

func serverAuthSecret(kc *kconfig.Config) string {
	h := kc.GetServer().GetHttp()["default"]
	if h == nil || h.GetAuth() == nil {
		return ""
	}
	return h.GetAuth().GetJwtSecret()
}

// provideOTPCipher derives the OTP secret-encryption cipher (SHA-256 → AES-256) from sys.otp.aes_key (required by its
// rule in app.proto; the committed value is a development placeholder, production value injected by the deployment).
func provideOTPCipher(s *conf.Sys) (*totp.Cipher, error) {
	sum := sha256.Sum256([]byte(s.GetOtp().GetAesKey()))
	return totp.NewCipher(sum[:])
}
