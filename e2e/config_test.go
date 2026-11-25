//go:build e2e

package e2e

import (
	"context"
	"io"
	"net"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/go-keel/keel-admin/e2e/harness"
)

// startWith starts a private admin on the round's databases with configs/app.dev.yaml set to appDev (and
// configs/config.dev.yaml to configDev when not empty), and returns its base URL. sys.* is read once at startup
// (docs/admin/config.md), so each setting needs its own process; the round's Redis and databases are shared.
func (e *env) startWith(t *testing.T, appDev, configDev string, extra ...string) (string, *harness.Process) {
	t.Helper()
	dir := e.overlayDir(t, appDev, configDev)
	return e.start(t, dir, extra...)
}

func (e *env) overlayDir(t *testing.T, appDev, configDev string) string {
	t.Helper()
	dir := harness.Workdir(t)
	write := func(name, s string) {
		if s != "" {
			if err := os.WriteFile(filepath.Join(dir, "configs", name), []byte(s), 0o644); err != nil {
				t.Fatal(err)
			}
		}
	}
	write("app.dev.yaml", appDev)
	write("config.dev.yaml", configDev)
	return dir
}

// mustFail starts an admin in dir that must exit non-zero with rule in its output; it returns the output.
func (e *env) mustFail(t *testing.T, dir, rule string, extra ...string) string {
	t.Helper()
	p := harness.Start(t, "admin", dir, append(append(e.processEnv(t), "KEEL_HTTP_ADDR="+harness.FreeAddr(t)), extra...)...)
	code, ok := p.Wait(timeout)
	if !ok {
		t.Fatalf("admin still running after %s, want a failed start (%s)\n%s", timeout, rule, p.Tail(20))
	}
	out := p.Output()
	if code == 0 || !strings.Contains(out, rule) {
		t.Fatalf("admin exited with %d, want non-zero with %s\n%s", code, rule, p.Tail(20))
	}
	return out
}

// Basis: configs/app.yaml sys.lockout (scope username locks the account from every IP; lock_minutes is the lock and
// the counting window, docs/admin/permission-model/sessions-and-audit.md §8; enabled false turns it off); the lockout
// key lockout:<subject> (internal/core/sys/data/lockout.go). The minute passing is stood in for with PEXPIRE, as in
// docs/admin/testing.md.
func testLockoutConfig(t *testing.T, e *env) {
	base, _ := e.startWith(t, "sys:\n  lockout:\n    scope: username\n    lock_minutes: 1\n", "")
	u := e.mkAccount(t, uniq("lk"), e.fx.dEast, e.fx.rView).at(base)
	for range 5 {
		u.loginWith(t, "wrong-password", "", xff()...).want(t, 401)
	}
	u.login(t, xff()...).wantError(t, 401, "INVALID_CREDENTIAL") // another IP: the whole account is locked
	key := "lockout:" + u.name
	ctx := context.Background()
	if ttl := e.rdb.PTTL(ctx, key).Val(); ttl <= 0 || ttl > time.Minute {
		t.Fatalf("lock_minutes 1: %s has TTL %s", key, ttl)
	}
	e.rdb.PExpire(ctx, key, time.Millisecond)
	time.Sleep(10 * time.Millisecond) // past the 1 ms just set
	u.login(t).want(t, 200)

	base, _ = e.startWith(t, "sys:\n  lockout:\n    enabled: false\n", "")
	u = e.mkAccount(t, uniq("lk"), e.fx.dEast, e.fx.rView).at(base)
	ip := xff()
	for range 6 {
		u.loginWith(t, "wrong-password", "", ip...).want(t, 401)
	}
	u.login(t, ip...).want(t, 200)
}

// Basis: docs/admin/auth.md, "Captcha"; auth.proto LoginMeta, IssueCaptcha;
// docs/admin/permission-model/otp.md §7.4 (one-time challenges; a captcha failure counts neither towards the lockout
// nor into the login log, sessions-and-audit.md §9); configs/app.yaml captcha.base64_length; the answer in Redis
// under captcha:ans:<id> (internal/core/captcha/base64/base64.go).
func testCaptchaBase64(t *testing.T, e *env) {
	base, _ := e.startWith(t, "sys:\n  captcha:\n    enabled: true\n    provider: base64\n    base64_length: 4\n", "")
	anon := e.user("", "", 0).at(base)
	m := anon.get(t, "/v1/sys/auth/login-meta").ok(t)
	if !m.boolean("captchaEnabled") || m.str("captchaProvider") != "base64" || m.str("captchaSiteKey") != "" {
		t.Fatalf("login-meta: %v", m)
	}
	challenge := func() (string, string) {
		c := anon.post(t, "/v1/sys/auth/captcha", nil).ok(t)
		if c.str("captchaId") == "" || c.str("imageBase64") == "" {
			t.Fatalf("captcha: %v", c)
		}
		ans := e.rdb.Get(context.Background(), "captcha:ans:"+c.str("captchaId")).Val()
		eq(t, "answer length", len(ans), 4)
		return c.str("captchaId"), ans
	}
	u := e.mkAccount(t, uniq("cap"), e.fx.dEast, e.fx.rView).at(base)
	login := func(id, ans string, ip []string) response {
		return u.post(t, "/v1/sys/auth/login", obj{"username": u.name, "password": password, "captchaId": id, "captchaAnswer": ans}, ip...)
	}
	logged := len(e.loginLogs(t, u.name))
	login("", "", nil).wantError(t, 401, "CAPTCHA_INVALID")
	id, ans := challenge()
	login(id, "wrong", nil).wantError(t, 401, "CAPTCHA_INVALID")
	login(id, ans, nil).wantError(t, 401, "CAPTCHA_INVALID") // one-time: deleted when checked
	id, ans = challenge()
	login(id, ans, nil).want(t, 200)
	ip := xff()
	for range 6 {
		id, _ := challenge()
		login(id, "wrong", ip).wantError(t, 401, "CAPTCHA_INVALID")
	}
	id, ans = challenge()
	login(id, ans, ip).want(t, 200) // captcha failures do not lock
	eq(t, "login log entries added", len(e.loginLogs(t, u.name))-logged, 2)
}

// Basis: docs/admin/auth.md, "Captcha"; docs/admin/config.md (turnstile: login-meta hands the site key to the login
// page). Only login-meta: verifying a Turnstile answer needs Cloudflare and is not tested here.
func testCaptchaTurnstileMeta(t *testing.T, e *env) {
	base, _ := e.startWith(t, "sys:\n  captcha:\n    enabled: true\n    provider: turnstile\n    turnstile:\n      site_key: e2e-site-key\n      secret_key: e2e-secret-key\n", "")
	m := e.user("", "", 0).at(base).get(t, "/v1/sys/auth/login-meta").ok(t)
	if m.str("captchaProvider") != "turnstile" || m.str("captchaSiteKey") != "e2e-site-key" {
		t.Fatalf("login-meta: %v", m)
	}
}

// Basis: docs/admin/permission-model/otp.md §7.3 (max_uses 1 allows one use per code; 0 or negative means the
// default 3); configs/app.yaml otp.max_uses.
func testOTPMaxUsesConfig(t *testing.T, e *env) {
	for _, c := range []struct {
		setting string
		uses    int
	}{{"1", 1}, {"0", 3}} {
		base, _ := e.startWith(t, "sys:\n  otp:\n    max_uses: "+c.setting+"\n", "")
		root := e.root().at(base)
		root.mustLogin(t)
		code := root.code(t) // the login above used another code or the same one: start this one's count afresh
		upd := func() response {
			return root.put(t, path("/v1/sys/depts", e.fx.dSouth), obj{"name": "QA-华南", "sort": 20, "status": 1}, "X-OTP-Code", code)
		}
		for range c.uses {
			upd().want(t, 200)
		}
		upd().wantError(t, 401, "OTP_ALREADY_USED")
	}
}

// Basis: docs/admin/permission-model/otp.md §7.1, §7.2 (secrets decrypted with sys.otp.aes_key: with another key
// step-up fails closed with 500 and a warning, and the login is refused); scenarios.md S6.
func testOTPKeyChanged(t *testing.T, e *env) {
	base, p := e.startWith(t, "sys:\n  otp:\n    aes_key: another-key-for-the-e2e-test\n", "")
	root := e.root().at(base)
	root.token = e.root().token // the session is valid on any process sharing the JWT secret
	mark := p.Mark()
	root.put(t, path("/v1/sys/depts", e.fx.dSouth), obj{"name": "QA-华南", "sort": 20, "status": 1}, root.otp(t)...).
		wantError(t, 500, "UNEXPECTED_ERROR")
	p.WaitLogSince(t, mark, timeout, "otp step-up verify failed, failing closed", nil)
	if r := e.fx.mgr.at(base).login(t); r.code == 200 {
		t.Fatalf("login with an undecryptable secret succeeded: %s", r.body)
	}
}

// Basis: docs/admin/auth.md, "Sessions" (the token lives sys.session_ttl, no renewal); configs/app.yaml session_ttl.
func testSessionTTL(t *testing.T, e *env) {
	base, _ := e.startWith(t, "sys:\n  session_ttl: 2s\n", "")
	u := e.fx.view.at(base)
	u.mustLogin(t)
	u.get(t, "/v1/sys/auth/profile").want(t, 200)
	time.Sleep(3 * time.Second) // the token's own expiry: the server's clock, nothing to fast-forward
	wantSessionGone(t, u)
}

// Basis: configs/app.yaml sys.brand (logo: a disk path served live, an http(s) URL redirected with 302, an unreadable
// path warned about and replaced by the embedded logo; title without a frontend rebuild); internal/server/spa.go;
// docs/admin/permission-model/deployment-and-config.md §12, §13; docs/admin/config.md (the dev overlay warning).
func testBrandConfig(t *testing.T, e *env) {
	dir := e.overlayDir(t, "sys:\n  brand:\n    title: \"QA 后台\"\n    logo: logo.png\n", "")
	png := func(b byte) []byte { return append([]byte("\x89PNG\r\n\x1a\n"), b) }
	if err := os.WriteFile(filepath.Join(dir, "logo.png"), png(1), 0o644); err != nil {
		t.Fatal(err)
	}
	base, p := e.start(t, dir)
	if !strings.Contains(p.Output(), "kconfig: dev overlay loaded") {
		t.Fatalf("no dev overlay warning:\n%s", p.Tail(10))
	}
	anon := e.user("", "", 0).at(base)
	eq(t, "brand", strings.TrimSpace(string(anon.get(t, "/v1/sys/brand").want(t, 200).body)), `{"title":"QA 后台"}`)
	eq(t, "logo from disk", string(anon.get(t, "/v1/sys/brand/logo").want(t, 200).body), string(png(1)))
	if err := os.WriteFile(filepath.Join(dir, "logo.png"), png(2), 0o644); err != nil {
		t.Fatal(err)
	}
	eq(t, "logo replaced in place", string(anon.get(t, "/v1/sys/brand/logo").want(t, 200).body), string(png(2)))

	base, _ = e.startWith(t, "sys:\n  brand:\n    logo: \"https://example.com/logo.png\"\n", "")
	r := e.user("", "", 0).at(base).get(t, "/v1/sys/brand/logo").want(t, 302)
	eq(t, "Location", r.header.Get("Location"), "https://example.com/logo.png")

	base, p = e.startWith(t, "sys:\n  brand:\n    logo: no-such-logo.png\n", "")
	p.WaitLog(t, "sys.brand.logo not readable, falling back to embedded logo", nil)
	embedded := e.user("", "", 0).get(t, "/v1/sys/brand/logo").want(t, 200).body
	eq(t, "embedded logo", string(e.user("", "", 0).at(base).get(t, "/v1/sys/brand/logo").want(t, 200).body), string(embedded))
}

// Basis: docs/admin/startup-errors.md (each rule); keel:docs/config.md (validation; the dev overlay refused under
// KEEL_ENV=prod).
func testStartupErrors(t *testing.T, e *env) {
	for _, c := range []struct{ app, config, rule string }{
		{"sys:\n  brand:\n    title: \"\"\n", "", "keel-admin/sys.brand-title-missing"},
		{"sys:\n  business_time_zone: \"\"\n", "", "keel-admin/sys.business-zone-missing"},
		{"sys:\n  business_time_zone: Mars/Base\n", "", "keel-admin/sys.business-zone-invalid"},
		{"sys:\n  otp:\n    aes_key: \"\"\n", "", "keel-admin/sys.otp-key-missing"},
		{"", "server:\n  http:\n    default:\n      auth:\n        jwt_secret: \"\"\n", "keel/kconfig.invalid"},
		{"", "server:\n  http:\n    default:\n      auth:\n        cookie_name: \"\"\n", "keel-admin/sys.cookie-name-missing"},
		{"sys:\n  captcha:\n    enabled: true\n    provider: nope\n", "", "keel-admin/captcha.provider-unknown"},
		{"sys:\n  captcha:\n    enabled: true\n    provider: turnstile\n    turnstile:\n      secret_key: \"\"\n", "", "keel-admin/captcha.provider-not-ready"},
	} {
		out := e.mustFail(t, e.overlayDir(t, c.app, c.config), c.rule)
		if c.rule == "keel/kconfig.invalid" && !strings.Contains(out, "jwt_secret") {
			t.Fatalf("kconfig.invalid does not name jwt_secret:\n%s", out)
		}
		if c.rule == "keel-admin/captcha.provider-unknown" && !strings.Contains(out, "base64") {
			t.Fatalf("provider-unknown does not list the registered providers:\n%s", out)
		}
	}
	e.mustFail(t, e.overlayDir(t, "{}\n", ""), "keel/kconfig.dev-overlay-in-prod", "KEEL_ENV=prod")
}

// redisProxy forwards TCP connections to the Redis test server until closed; closing it also cuts the connections
// it carries, so the admin behind it sees Redis go down.
type redisProxy struct {
	ln    net.Listener
	mu    sync.Mutex
	conns []net.Conn
}

func newRedisProxy(t *testing.T, target string) *redisProxy {
	t.Helper()
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	p := &redisProxy{ln: ln}
	go func() {
		for {
			c, err := ln.Accept()
			if err != nil {
				return
			}
			s, err := net.Dial("tcp", target)
			if err != nil {
				_ = c.Close()
				continue
			}
			p.mu.Lock()
			p.conns = append(p.conns, c, s)
			p.mu.Unlock()
			go func() { _, _ = io.Copy(s, c); _ = s.Close() }()
			go func() { _, _ = io.Copy(c, s); _ = c.Close() }()
		}
	}()
	t.Cleanup(p.close)
	return p
}

func (p *redisProxy) close() {
	_ = p.ln.Close()
	p.mu.Lock()
	defer p.mu.Unlock()
	for _, c := range p.conns {
		_ = c.Close()
	}
	p.conns = nil
}

// Basis: internal/core/sys/biz/auth.go Login and the LockoutGuard contract (a Redis outage must not lock everyone
// out: the brute-force check fails open, with a warning). The account is locked first, then Redis goes away.
func testLockoutFailsOpenWithoutRedis(t *testing.T, e *env) {
	proxy := newRedisProxy(t, harness.Env(t, "TEST_REDIS_ADDR"))
	base, p := e.start(t, harness.Workdir(t), "KEEL_REDIS_ADDR="+proxy.ln.Addr().String())
	u := e.mkAccount(t, uniq("down"), e.fx.dEast, e.fx.rView).at(base)
	ip := xff()
	for range 5 {
		u.loginWith(t, "wrong-password", "", ip...).wantError(t, 401, "INVALID_CREDENTIAL")
	}
	u.login(t, ip...).wantError(t, 401, "INVALID_CREDENTIAL") // locked while Redis answers
	proxy.close()
	mark := p.Mark()
	u.login(t, ip...).want(t, 200)
	p.WaitLogSince(t, mark, timeout, "lockout guard Failures failed, failing open", nil)
}
