package server

import (
	"crypto/sha256"
	"fmt"
	"time"

	"github.com/go-keel/keel-admin/internal/core/errs"

	"github.com/go-keel/keel-admin/internal/conf"
	"github.com/go-keel/keel-admin/internal/core/audit"
	"github.com/go-keel/keel-admin/internal/core/captcha/turnstile"
	"github.com/go-keel/keel-admin/internal/core/sys/biz"
	"github.com/go-keel/keel-admin/internal/core/sys/totp"
	"github.com/go-keel/keel/core/kconfig"
	"github.com/go-keel/keel/pkg/jwt"
)

// ProvideSysConf extracts the Sys (framework) config section from the dynamic
// bootstrap (app.yaml). Read once at startup — changes require a restart.
func ProvideSysConf(b *kconfig.Dynamic[conf.Bootstrap]) *conf.Sys {
	return b.Get().Sys
}

// ProvideBrand validates and extracts the deploy-time brand (sys.brand).
// title is required: the template's default brand must not slip into production silently, so empty fails startup
// (same as otp.aes_key). An empty logo means the embedded default.
func ProvideBrand(s *conf.Sys) (*conf.Brand, error) {
	b := s.GetBrand()
	if b.GetTitle() == "" {
		return nil, errs.Rules.New("sys.brand-title-missing", "sys.brand.title is empty", "set sys.brand.title in the config")
	}
	return b, nil
}

// ProvideBusinessZone validates and loads sys.business_time_zone. It is required: the time zone that splits business
// days is a business rule, and silently splitting by UTC when it is missing would shift filters and totals by hours
// without any error.
func ProvideBusinessZone(s *conf.Sys) (*time.Location, error) {
	name := s.GetBusinessTimeZone()
	if name == "" {
		return nil, errs.Rules.New("sys.business-zone-missing", "sys.business_time_zone is empty", "set sys.business_time_zone to an IANA zone such as Asia/Shanghai")
	}
	loc, err := time.LoadLocation(name)
	if err != nil {
		return nil, errs.Rules.Wrap(err, "sys.business-zone-invalid", fmt.Sprintf("sys.business_time_zone %q is not a known time zone (%v)", name, err), "use an IANA zone name such as Asia/Shanghai")
	}
	return loc, nil
}

// ProvideAuthUseCase assembles the auth use case. The login JWT is signed with
// the SAME secret Keel's auth middleware verifies with (Server.Http auth config),
// so there is a single source of truth for the token secret. The lockout + captcha
// config comes from app.yaml sys.* (startup, static).
//
// An empty secret returns an error and the service does not start: anyone could forge login tokens signed with an
// empty secret.
func ProvideAuthUseCase(users biz.UserRepo, ll biz.LoginLogger, cache biz.PermCache, gate biz.OTPGate, s *conf.Sys, kc *kconfig.Config, guard biz.LockoutGuard, captchaGate biz.CaptchaGate) (*biz.AuthUseCase, error) {
	signer, err := jwt.NewHS256Signer([]byte(serverAuthSecret(kc)))
	if err != nil {
		return nil, errs.Rules.Wrap(err, "sys.login-signer", fmt.Sprintf("cannot build the login token signer from server.http.default.auth.jwt_secret (%v)", err), "set server.http.default.auth.jwt_secret to a non-empty secret")
	}
	cfg := biz.AuthConfig{
		CaptchaEnabled:     s.GetCaptcha().GetEnabled(),
		CaptchaProvider:    s.GetCaptcha().GetProvider(),
		CaptchaSiteKey:     captchaSiteKey(s.GetCaptcha()),
		LockoutEnabled:     s.GetLockout().GetEnabled(),
		LockoutMaxFailures: s.GetLockout().GetMaxFailures(),
		LockoutLockMinutes: s.GetLockout().GetLockMinutes(),
		LockoutScope:       s.GetLockout().GetScope(),
	}
	return biz.NewAuthUseCase(users, ll, cache, gate, signer, s.GetSessionTtl().AsDuration(), guard, cfg, captchaGate), nil
}

// captchaSiteKey returns a value only when a widget provider is selected: the site key is provider-specific, and
// sending an unused key for the image code or when captcha is off would only make the login page guess what to render.
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

// ProvideOTPCipher derives the OTP secret-encryption cipher (SHA-256 → AES-256)
// from sys.otp.aes_key (app.yaml; the committed value is a development placeholder, production value injected by the deployment). Empty → startup fails.
func ProvideOTPCipher(s *conf.Sys) (*totp.Cipher, error) {
	key := s.GetOtp().GetAesKey()
	if key == "" {
		return nil, errs.Rules.New("sys.otp-key-missing", "sys.otp.aes_key is empty", "inject sys.otp.aes_key from the deployment environment (never commit it)")
	}
	sum := sha256.Sum256([]byte(key))
	return totp.NewCipher(sum[:])
}

// ProvideOTPUseCase assembles the OTP use case. The issuer shown by
// authenticator apps shares the deploy-time brand title used by the SPA.
func ProvideOTPUseCase(repo biz.OTPRepo, cipher *totp.Cipher, brand *conf.Brand, guard biz.ReplayGuard) *biz.OTPUseCase {
	return biz.NewOTPUseCase(repo, cipher, brand.GetTitle(), guard)
}

// ProvideOTPGate exposes the OTP use case as the login OTP gate.
func ProvideOTPGate(uc *biz.OTPUseCase) biz.OTPGate { return uc }

// ProvideAuditRecorder exposes the audit recorder for the middleware chain.
//
// The middleware and biz share core/audit.Entry, so AuditUseCase satisfies audit.Recorder directly with no adapter
// (why there are not two definitions copied by hand: see the package comment of core/audit).
func ProvideAuditRecorder(uc *biz.AuditUseCase) audit.Recorder { return uc }
