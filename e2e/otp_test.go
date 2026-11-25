//go:build e2e

package e2e

import (
	"strings"
	"testing"
)

// Basis: docs/admin/permission-model/otp.md §7.1 (root is always forced: the first login gets a restricted bind
// token and no session); auth.proto LoginReply, BindOTP, ConfirmOTPReply.
func testRootFirstLoginBinds(t *testing.T, e *env) {
	login := e.rootLogin.ok(t)
	if login.str("bindToken") == "" || e.rootLogin.setCookie() != "" {
		t.Fatalf("first root login: want a bindToken and no session cookie: %s %q", e.rootLogin.body, e.rootLogin.setCookie())
	}
	if !strings.HasPrefix(e.rootBind.setCookie(), cookieName+"=") || !e.rootBind.json(t).boolean("loggedIn") {
		t.Fatalf("confirm: want loggedIn and a session cookie: %s", e.rootBind.body)
	}
	p := e.root().get(t, "/v1/sys/auth/profile").ok(t)
	eq(t, "isRoot", p.boolean("isRoot"), true)
	eq(t, "otpBound", p.boolean("otpBound"), true)

	// Bound now: a password alone fails, with the code it passes.
	r := e.user(rootName, rootPassword, e.root().id)
	r.loginWith(t, rootPassword, "").wantError(t, 401, "INVALID_CREDENTIAL")
	r.secret = e.root().secret
	r.login(t).want(t, 200)
}

// bearer is the Authorization header carrying a restricted bind token.
func bearer(token string) []string { return []string{"Authorization", "Bearer " + token} }

// Basis: docs/admin/permission-model/scenarios.md S9 (a role forcing an authenticator: the first login walks the
// binding, confirm logs in); function-permissions.md §4.4 step 4 (the restricted token reaches only BindOTP and
// ConfirmOTP); docs/admin/auth.md, "Sessions" (the Authorization header wins over the cookie);
// internal/core/sys/biz/otp.go Confirm; sessions-and-audit.md §8 (the restricted token carries the token version).
func testForcedBinding(t *testing.T, e *env) {
	root := e.root()
	u := e.mkAccount(t, uniq("force"), e.fx.dEast, e.fx.rForce)
	r := u.login(t)
	bt := r.ok(t).str("bindToken")
	if bt == "" || r.setCookie() != "" {
		t.Fatalf("forced, unbound: want a bindToken and no cookie: %s", r.body)
	}
	u.get(t, "/v1/sys/auth/profile", bearer(bt)...).wantError(t, 403, "FORBIDDEN")
	root.get(t, "/v1/sys/auth/profile", bearer(bt)...).wantError(t, 403, "FORBIDDEN") // header over root's cookie
	u.post(t, "/v1/sys/auth/otp/bind", nil, bearer(bt)...).want(t, 200)
	u.post(t, "/v1/sys/auth/otp/confirm", obj{"code": "000000"}, bearer(bt)...).wantError(t, 401, "OTP_INVALID")
	c := u.bind(t, bt, true)
	if !c.ok(t).boolean("loggedIn") || c.setCookie() == "" {
		t.Fatalf("confirm: want loggedIn and a cookie: %s", c.body)
	}
	u.get(t, "/v1/sys/auth/profile").want(t, 200)
	u.as().login(t).want(t, 200)

	// Unbound again, the forced role walks the binding again; disabling the account voids the restricted token.
	u.post(t, "/v1/sys/auth/otp/unbind", obj{"password": password}).want(t, 200)
	u.secret = ""
	bt = u.as().login(t).ok(t).str("bindToken")
	if bt == "" {
		t.Fatal("unbound forced account: no bindToken")
	}
	root.post(t, path("/v1/sys/users", u.id, "disable"), nil, root.otp(t)...).want(t, 200)
	if r := u.post(t, "/v1/sys/auth/otp/bind", nil, bearer(bt)...); r.code != 401 {
		t.Fatalf("bind with the token of a disabled account: %d %s", r.code, r.body)
	}
}

// Basis: docs/admin/permission-model/otp.md §7.2 (step-up applies to root too); scenarios.md S6.
func testStepUp(t *testing.T, e *env) {
	root := e.root()
	body := obj{"parentId": e.fx.dSouth, "name": uniq("QA-临时")}
	root.post(t, "/v1/sys/depts", body).wantError(t, 401, "OTP_STEPUP")
	root.post(t, "/v1/sys/depts", body, "X-OTP-Code", "000000").wantError(t, 401, "OTP_INVALID")
	id := root.post(t, "/v1/sys/depts", body, root.otp(t)...).ok(t).id("id")
	root.del(t, path("/v1/sys/depts", id), root.otp(t)...).want(t, 200)
}

// Basis: docs/admin/permission-model/otp.md §7.3 (one code is accepted sys.otp.max_uses times, 3 by default, login
// and step-up drawing on one quota); configs/app.yaml otp.max_uses.
func testOTPQuota(t *testing.T, e *env) {
	root := e.root()
	upd := func(code string) response {
		return root.put(t, path("/v1/sys/depts", e.fx.dSouth), obj{"name": "QA-华南", "sort": 20, "status": 1}, "X-OTP-Code", code)
	}
	c := root.code(t)
	for range 3 {
		upd(c).want(t, 200)
	}
	upd(c).wantError(t, 401, "OTP_ALREADY_USED")

	c = root.code(t)
	root.as().loginWith(t, rootPassword, c).want(t, 200)
	upd(c).want(t, 200)
	upd(c).want(t, 200)
	upd(c).wantError(t, 401, "OTP_ALREADY_USED")
}

// Basis: docs/admin/permission-model/otp.md §7.2 (an operation needing a code asks an account without an
// authenticator to bind one); auth.proto BindOTP, ConfirmOTPReply, UnbindOTP; internal/core/sys/biz/otp.go Bind;
// internal/core/sys/biz/auth.go Login step 3 (a bound account always needs the code); proto LoginRequest.otp_code.
func testSelfBinding(t *testing.T, e *env) {
	u := e.mkUser(t, uniq("selfbind"), e.fx.dEast, e.fx.rMgr)
	newUser := obj{"username": uniq("x"), "password": password, "deptId": e.fx.dEast, "roleIds": []int64{e.fx.rView}}
	u.post(t, "/v1/sys/users", newUser, "X-OTP-Code", "123456").wantError(t, 401, "OTP_BIND_REQUIRED")
	u.as().login(t).want(t, 200) // unbound and not forced: no code needed

	c := u.bind(t, "", false)
	if c.ok(t).boolean("loggedIn") {
		t.Fatalf("self binding from a session must not log in again: %s", c.body)
	}
	eq(t, "otpBound", u.get(t, "/v1/sys/auth/profile").ok(t).boolean("otpBound"), true)
	u.post(t, "/v1/sys/auth/otp/bind", nil).wantError(t, 409, "OTP_ALREADY_BOUND")
	u.as().loginWith(t, password, "").wantError(t, 401, "INVALID_CREDENTIAL")
	u.as().login(t).want(t, 200)

	u.post(t, "/v1/sys/auth/otp/unbind", obj{"password": "wrong-password"}).wantError(t, 401, "INVALID_CREDENTIAL")
	u.post(t, "/v1/sys/auth/otp/unbind", obj{"password": password}).want(t, 200)
	u.secret = ""
	eq(t, "otpBound", u.get(t, "/v1/sys/auth/profile").ok(t).boolean("otpBound"), false)
}

// Basis: docs/admin/permission-model/sessions-and-audit.md §8 (ResetOTP logs the account out everywhere);
// scenarios.md S6; user.proto ResetOTP.
func testResetOTP(t *testing.T, e *env) {
	root := e.root()
	u := e.mkUser(t, uniq("reset"), e.fx.dEast, e.fx.rMgr)
	u.bind(t, "", false).want(t, 200)
	root.post(t, path("/v1/sys/users", u.id, "reset_otp"), nil, root.otp(t)...).want(t, 200)
	eq(t, "otpBound", root.get(t, path("/v1/sys/users", u.id)).ok(t).boolean("otpBound"), false)
	u.get(t, "/v1/sys/auth/profile").wantError(t, 401, "UNAUTHENTICATED")
	u.secret = ""
	u.mustLogin(t)
	u.post(t, "/v1/sys/users", obj{"username": uniq("x"), "password": password, "deptId": e.fx.dEast, "roleIds": []int64{e.fx.rView}},
		"X-OTP-Code", "123456").wantError(t, 401, "OTP_BIND_REQUIRED")
}
