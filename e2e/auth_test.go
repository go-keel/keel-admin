//go:build e2e

package e2e

import (
	"strings"
	"testing"
	"unicode"
)

// loginLogs returns the login log entries of username as the auditor sees them, newest first.
func (e *env) loginLogs(t testing.TB, username string) []obj {
	t.Helper()
	var out []obj
	for _, it := range e.fx.aud.get(t, "/v1/sys/login-logs"+q("username", username, "limit", 200)).ok(t).items() {
		if it.str("username") == username {
			out = append(out, it)
		}
	}
	return out
}

// lastLogin returns the newest login log entry of username.
func (e *env) lastLogin(t testing.TB, username string) obj {
	t.Helper()
	l := e.loginLogs(t, username)
	if len(l) == 0 {
		t.Fatalf("no login log entry for %s", username)
	}
	return l[0]
}

func hasHan(s string) bool {
	for _, r := range s {
		if unicode.Is(unicode.Han, r) {
			return true
		}
	}
	return false
}

// Basis: auth.proto LoginReply; docs/admin/auth.md, "Sessions"; docs/admin/permission-model/otp.md §7.1 (a wrong
// password and an unknown user are one indistinguishable rejection).
func testLoginBasics(t *testing.T, e *env) {
	u := e.fx.view.as()
	r := u.login(t)
	m := r.ok(t)
	eq(t, "username", m.str("username"), "u_view")
	eq(t, "userId", m.id("userId"), u.id)
	eq(t, "isRoot", m.boolean("isRoot"), false)
	if _, ok := m["token"]; ok || !strings.HasPrefix(r.setCookie(), cookieName+"=") {
		t.Fatalf("want no token in the body and a session cookie: %s %q", r.body, r.setCookie())
	}

	wrong := u.loginWith(t, "wrong-password", "").wantError(t, 401, "INVALID_CREDENTIAL")
	nobody := e.user(uniq("nobody"), password, 0)
	if msg := nobody.login(t).wantError(t, 401, "INVALID_CREDENTIAL"); msg != wrong || msg == "" {
		t.Fatalf("unknown user %q and wrong password %q must give the same message", msg, wrong)
	}
	last := e.lastLogin(t, nobody.name)
	eq(t, "unknown user userId", last.id("userId"), 0)
	eq(t, "unknown user msg", last.str("msg"), "user not found")
	eq(t, "wrong password msg", e.lastLogin(t, "u_view").str("msg"), "bad password")
}

// Basis: docs/admin/errors.md, "Contract-layer validation messages"; keel:docs/middleware.md, "Validate".
func testLoginValidatorMessage(t *testing.T, e *env) {
	anon := e.user("", "x", 0)
	zh := anon.post(t, "/v1/sys/auth/login", obj{"username": "", "password": "x"}).wantError(t, 400, "VALIDATOR")
	anon.lang = "en"
	en := anon.post(t, "/v1/sys/auth/login", obj{"username": "", "password": "x"}).wantError(t, 400, "VALIDATOR")
	if !hasHan(zh) || hasHan(en) || strings.Contains(en, "username") {
		t.Fatalf("want a generic message per language: zh %q, en %q", zh, en)
	}
}

// Basis: auth.proto LoginMeta; configs/app.yaml sys.captcha (off by default).
func testLoginMeta(t *testing.T, e *env) {
	m := e.user("", "", 0).get(t, "/v1/sys/auth/login-meta").ok(t)
	eq(t, "captchaEnabled", m.boolean("captchaEnabled"), false)
	eq(t, "captchaSiteKey", m.str("captchaSiteKey"), "")
}

// Basis: docs/admin/permission-model/sessions-and-audit.md §8 (lockout after consecutive failures, reset on success);
// configs/app.yaml sys.lockout (5 failures, username_ip); otp.md §7.1 (a missing code does not count, a wrong one
// does).
func testLockout(t *testing.T, e *env) {
	u := e.fx.south.as()
	ip := xff()
	for range 5 {
		u.loginWith(t, "wrong-password", "", ip...).wantError(t, 401, "INVALID_CREDENTIAL")
	}
	u.login(t, ip...).wantError(t, 401, "INVALID_CREDENTIAL") // locked, not revealed
	eq(t, "msg", e.lastLogin(t, u.name).str("msg"), "locked")
	u.login(t, xff()...).want(t, 200) // another IP has its own counter

	// A missing code (account with an authenticator) does not count; a wrong one does.
	m := e.fx.mgr.as()
	ip = xff()
	for range 6 {
		m.loginWith(t, password, "", ip...).wantError(t, 401, "INVALID_CREDENTIAL")
	}
	eq(t, "msg", e.lastLogin(t, m.name).str("msg"), "otp missing")
	m.login(t, ip...).want(t, 200)
	ip = xff()
	for range 5 {
		m.loginWith(t, password, "000000", ip...).wantError(t, 401, "INVALID_CREDENTIAL")
	}
	m.login(t, ip...).wantError(t, 401, "INVALID_CREDENTIAL")
	eq(t, "msg", e.lastLogin(t, m.name).str("msg"), "locked")

	// Success resets the counter.
	v := e.fx.view.as()
	ip = xff()
	for range 2 {
		for range 4 {
			v.loginWith(t, "wrong-password", "", ip...).wantError(t, 401, "INVALID_CREDENTIAL")
		}
		v.login(t, ip...).want(t, 200)
	}
}

// Basis: docs/admin/permission-model/otp.md §7.1 (the login IP allow list, checked only when logging in);
// user.proto UpdateUser; internal/core/sys/biz/ipwhitelist.go.
func testIPAllowList(t *testing.T, e *env) {
	u := e.mkUser(t, uniq("ipwl"), e.fx.dSouth, e.fx.rView)
	e.updateUser(t, e.root(), u.id, obj{"ipWhitelist": "198.51.100.0/24"}).want(t, 200)

	other := u.as()
	msg := other.login(t, "X-Forwarded-For", "203.0.113.5").wantError(t, 403, "IP_NOT_ALLOWED")
	if !strings.Contains(msg, "203.0.113.5") {
		t.Fatalf("message does not name the IP: %q", msg)
	}
	eq(t, "msg", e.lastLogin(t, u.name).str("msg"), "ip not allowed")
	other.login(t, "X-Forwarded-For", "198.51.100.20").want(t, 200)
	u.get(t, "/v1/sys/auth/profile").want(t, 200) // the session from before is not kicked

	e.updateUser(t, e.root(), u.id, obj{"ipWhitelist": "not-an-ip"}).wantError(t, 400, "INVALID_ARGUMENT")
}

// Basis: docs/admin/auth.md, "Sessions"; internal/core/sys/service/cookie.go; configs/app.yaml session_ttl and
// cookie_secure (true in the committed config).
func testSessionCookie(t *testing.T, e *env) {
	u := e.fx.view.as()
	c := u.post(t, "/v1/sys/auth/login", obj{"username": u.name, "password": password, "remember": false}).want(t, 200).setCookie()
	if strings.Contains(c, "Max-Age") {
		t.Fatalf("without remember the cookie must be session-only: %q", c)
	}
	c = u.login(t).want(t, 200).setCookie()
	for _, a := range []string{"Max-Age=2592000", "HttpOnly", "SameSite=Strict", "Path=/", "Secure"} {
		if !strings.Contains(c, a) {
			t.Fatalf("session cookie lacks %s: %q", a, c)
		}
	}
}

// Basis: docs/admin/permission-model/otp.md §7.1 and the PostgreSQL schema's case-insensitive collation
// (migrations/admin/*/0_schema.sql: utf8mb4_0900_ai_ci on MySQL, the ci collation on PostgreSQL): a username matches whatever its case, on both databases.
func testLoginUsernameCaseInsensitive(t *testing.T, e *env) {
	u := e.fx.view.as()
	u.name = "U_VIEW"
	m := u.login(t).ok(t)
	eq(t, "userId", m.id("userId"), e.fx.view.id)
}
