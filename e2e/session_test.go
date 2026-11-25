//go:build e2e

package e2e

import (
	"encoding/base64"
	"fmt"
	"slices"
	"strings"
	"sync"
	"testing"
)

// wantSessionGone fails the test unless u's session is rejected as one of the session failure reasons.
func wantSessionGone(t testing.TB, u *user) {
	t.Helper()
	r := u.get(t, "/v1/sys/auth/profile").want(t, 401)
	if reason := r.json(t).str("reason"); !has([]string{"UNAUTHORIZED", "UNAUTHENTICATED", "SESSION_EXPIRED"}, reason) {
		t.Fatalf("reason %s is not a session failure: %s", reason, r.body)
	}
}

// Basis: docs/admin/errors.md, "The frontend has two rules"; docs/admin/auth.md, "Sessions";
// internal/core/sys/service/cookie.go; auth.proto Logout (anonymous, clears an expired session too).
func testLogout(t *testing.T, e *env) {
	wantSessionGone(t, e.user("", "", 0))
	u := e.fx.view.as()
	u.mustLogin(t)
	c := u.post(t, "/v1/sys/auth/logout", nil).want(t, 200).setCookie()
	if !strings.HasPrefix(c, cookieName+"=;") || !strings.Contains(c, "Max-Age=0") {
		t.Fatalf("logout must clear the cookie: %q", c)
	}
	u.token = ""
	wantSessionGone(t, u)
	e.user("", "", 0).post(t, "/v1/sys/auth/logout", nil).want(t, 200)
}

// Basis: docs/admin/permission-model/sessions-and-audit.md §8 (kicking, a password reset, disabling, changing one's
// own password and granting or revoking root end every session of the account at once); otp.md §7.1 (a disabled
// account gets the same rejection as a wrong password).
func testSessionsEnd(t *testing.T, e *env) {
	root := e.root()
	u := e.mkUser(t, uniq("sess"), e.fx.dEast, e.fx.rView)
	u2 := u.as()
	u2.mustLogin(t)

	root.post(t, path("/v1/sys/users", u.id, "kick"), nil, root.otp(t)...).want(t, 200)
	for _, j := range []*user{u, u2} {
		j.get(t, "/v1/sys/auth/profile").wantError(t, 401, "UNAUTHENTICATED")
	}
	u.mustLogin(t)

	root.post(t, path("/v1/sys/users", u.id, "reset_pwd"), obj{"password": "E2e-reset-1"}, root.otp(t)...).want(t, 200)
	u.get(t, "/v1/sys/auth/profile").want(t, 401)
	u.as().login(t).wantError(t, 401, "INVALID_CREDENTIAL")
	u.password = "E2e-reset-1"
	u.mustLogin(t)

	root.post(t, path("/v1/sys/users", u.id, "disable"), nil, root.otp(t)...).want(t, 200)
	u.get(t, "/v1/sys/auth/profile").wantError(t, 401, "UNAUTHENTICATED")
	u.as().login(t).wantError(t, 401, "INVALID_CREDENTIAL")
	eq(t, "msg", e.lastLogin(t, u.name).str("msg"), "disabled")
	root.post(t, path("/v1/sys/users", u.id, "enable"), nil, root.otp(t)...).want(t, 200)
	u.mustLogin(t)

	root.post(t, path("/v1/sys/users", u.id, "root"), obj{"isRoot": true}, root.otp(t)...).want(t, 200)
	u.get(t, "/v1/sys/auth/profile").want(t, 401)
}

// Basis: docs/admin/permission-model/sessions-and-audit.md §8 (changing one's own password ends every session,
// the one that changed it included); docs/admin/forms.md, "Success messages"; internal/core/sys/biz/auth.go
// ChangeMyPassword; auth.proto ChangeMyPasswordRequest (at least 8 characters).
func testChangeMyPassword(t *testing.T, e *env) {
	u := e.mkUser(t, uniq("pwd"), e.fx.dEast, e.fx.rMgr)
	u.bind(t, "", false).want(t, 200)
	u2 := u.as()
	u2.mustLogin(t)
	u.post(t, "/v1/sys/auth/password", obj{"oldPassword": "wrong-password", "newPassword": "E2e-xyz-123"}, u.otp(t)...).
		wantError(t, 401, "INVALID_CREDENTIAL")
	u.post(t, "/v1/sys/auth/password", obj{"oldPassword": password, "newPassword": "E2e-xyz-123"}).wantError(t, 401, "OTP_STEPUP")
	u.post(t, "/v1/sys/auth/password", obj{"oldPassword": password, "newPassword": "short"}, u.otp(t)...).wantError(t, 400, "VALIDATOR")
	u.post(t, "/v1/sys/auth/password", obj{"oldPassword": password, "newPassword": "E2e-xyz-123"}, u.otp(t)...).want(t, 200)
	wantSessionGone(t, u)
	wantSessionGone(t, u2)
	u.password = "E2e-xyz-123"
	u.mustLogin(t)
}

// Basis: docs/admin/permission-model/sessions-and-audit.md §8 (permission caches are dropped per role, so a change
// applies on the next request).
func testRoleChangeAppliesNow(t *testing.T, e *env) {
	role := e.mkRole(t, uniq("live"), obj{"perms": []string{"sys:account:view"}})
	u := e.mkUser(t, uniq("live"), e.fx.dEast, role)
	u.get(t, "/v1/sys/users").want(t, 200)
	e.updateRole(t, role, obj{"perms": []string{"sys:audit:view"}}).want(t, 200)
	u.get(t, "/v1/sys/users").wantError(t, 403, "FORBIDDEN")
	e.updateRole(t, role, obj{"perms": []string{"sys:account:view"}}).want(t, 200)
	u.get(t, "/v1/sys/users").want(t, 200)
}

// Basis: auth.proto ProfileReply, MyPermissions; docs/admin/permission-model/frontend.md §14 and scenarios.md S2
// (perms are expanded with their prerequisites).
func testProfileAndMyPermissions(t *testing.T, e *env) {
	p := e.fx.ed1.get(t, "/v1/sys/auth/profile").ok(t)
	eq(t, "userId", p.id("userId"), e.fx.ed1.id)
	eq(t, "deptId", p.id("deptId"), e.fx.dSH)
	eq(t, "isRoot", p.boolean("isRoot"), false)
	if _, ok := p["settings"]; !ok {
		t.Fatalf("profile lacks settings: %v", p)
	}
	mustHave(t, "perms", p.strs("perms"), "sys:dept:view")
	my := e.fx.ed1.get(t, "/v1/sys/meta/my-permissions").ok(t)
	eq(t, "isRoot", my.boolean("isRoot"), false)
	a, b := slices.Sorted(slices.Values(p.strs("perms"))), slices.Sorted(slices.Values(my.strs("codes")))
	if !slices.Equal(a, b) {
		t.Fatalf("my-permissions %v differs from profile perms %v", b, a)
	}
	eq(t, "root my-permissions", e.root().get(t, "/v1/sys/meta/my-permissions").ok(t).boolean("isRoot"), true)
}

// Basis: auth.proto MyAccountReply (createdAt in UTC RFC 3339; recentLogins newest first, failures included, only
// one's own).
func testMyAccount(t *testing.T, e *env) {
	u := e.mkAccount(t, uniq("acct"), e.fx.dSH, e.fx.rView, e.fx.rEd)
	u.loginWith(t, "wrong-password", "").want(t, 401)
	u.mustLogin(t)
	e.fx.view.as().loginWith(t, "wrong-password", "").want(t, 401) // someone else's attempt
	a := u.get(t, "/v1/sys/auth/account").ok(t)
	eq(t, "deptName", a.str("deptName"), "QA-上海")
	if !slices.Equal(slices.Sorted(slices.Values(col(a.list("roles"), "name"))), []string{"qa_dept_editor", "qa_viewer"}) {
		t.Fatalf("roles: %v", a["roles"])
	}
	if !strings.HasSuffix(a.str("createdAt"), "Z") {
		t.Fatalf("createdAt not UTC: %q", a.str("createdAt"))
	}
	eq(t, "otpBound", a.boolean("otpBound"), false)
	if _, ok := a["ipWhitelist"]; ok && a.str("ipWhitelist") != "" {
		t.Fatalf("ipWhitelist: %v", a["ipWhitelist"])
	}
	rl := a.list("recentLogins")
	if len(rl) != 2 || !rl[0].boolean("success") || rl[1].boolean("success") || rl[0].id("id") < rl[1].id("id") {
		t.Fatalf("recentLogins: want this account's success then its failure: %v", rl)
	}
}

// avatar returns a PNG data URI decoding to n bytes.
func avatar(n int) string {
	return "data:image/png;base64," + base64.StdEncoding.EncodeToString(make([]byte, n))
}

// Basis: auth.proto UpdateMyProfile and its field rules; docs/admin/errors.md, "Contract-layer validation messages";
// internal/core/sys/biz/avatar.go (256 KB); management-boundary.md §6 ("Self-operation": one edits oneself through the profile).
func testUpdateMyProfile(t *testing.T, e *env) {
	u := e.mkUser(t, uniq("prof"), e.fx.dEast, e.fx.rView)
	u.put(t, "/v1/sys/auth/profile", obj{"nickname": "视图", "email": "v@example.com", "phone": "+86 138-0000-0000"}).want(t, 200)
	a := u.get(t, "/v1/sys/auth/account").ok(t)
	eq(t, "nickname", a.str("nickname"), "视图")
	eq(t, "email", a.str("email"), "v@example.com")
	eq(t, "phone", a.str("phone"), "+86 138-0000-0000")

	for field, bad := range map[string]obj{"email": {"nickname": "x", "email": "bad"}, "phone": {"nickname": "x", "phone": "12"}} {
		if msg := u.put(t, "/v1/sys/auth/profile", bad).wantError(t, 400, "VALIDATOR"); !strings.Contains(msg, field) {
			t.Fatalf("validation message %q should name the field %s", msg, field)
		}
	}
	u.put(t, "/v1/sys/auth/profile", obj{"nickname": "x", "avatar": avatar(300 << 10)}).wantError(t, 400, "AVATAR_TOO_LARGE")
	u.put(t, "/v1/sys/auth/profile", obj{"nickname": "x", "avatar": avatar(1 << 10)}).want(t, 200)
	eq(t, "avatar set", u.get(t, "/v1/sys/auth/account").ok(t).str("avatar"), avatar(1<<10))
	u.put(t, "/v1/sys/auth/profile", obj{"nickname": "x", "avatar": ""}).want(t, 200)
	eq(t, "avatar cleared", u.get(t, "/v1/sys/auth/account").ok(t).str("avatar"), "")

	e.fx.mgr.put(t, "/v1/sys/auth/profile", obj{"nickname": "u_mgr"}).want(t, 200)
}

// Basis: auth.proto AddFavorite, RemoveFavorite, ReorderFavorites, ProfileReply.settings; docs/admin/auth.md,
// "User settings" (paths are not checked); internal/core/sys/biz/settings.go (a permutation of the current set,
// MaxFavorites 30).
func testFavorites(t *testing.T, e *env) {
	u := e.mkUser(t, uniq("fav"), e.fx.dEast, e.fx.rView)
	favs := func(r response) []string {
		t.Helper()
		return r.ok(t).strs("favoriteMenus")
	}
	add := func(p string) response { return u.post(t, "/v1/sys/auth/favorites/add", obj{"path": p}) }
	favs(add("/sys/users"))
	if got := favs(add("/sys/users")); !slices.Equal(got, []string{"/sys/users"}) {
		t.Fatalf("adding twice: %v", got)
	}
	if got := favs(u.post(t, "/v1/sys/auth/favorites/remove", obj{"path": "/not-there"})); !slices.Equal(got, []string{"/sys/users"}) {
		t.Fatalf("removing an absent path: %v", got)
	}
	favs(add("/sys/depts"))
	favs(add("/sys/audit-logs"))
	got := favs(u.post(t, "/v1/sys/auth/favorites/reorder", obj{"paths": []string{"/sys/depts", "/sys/users"}}))
	if !slices.Equal(got, []string{"/sys/depts", "/sys/users", "/sys/audit-logs"}) {
		t.Fatalf("reorder: %v", got)
	}
	u.post(t, "/v1/sys/auth/favorites/reorder", obj{"paths": []string{"/sys/depts", "/x"}}).wantError(t, 400, "INVALID_ARGUMENT")
	favs(add("/no-such-page"))
	for i := len(got) + 1; i < 30; i++ {
		favs(add(fmt.Sprintf("/qa/%d", i)))
	}
	add("/qa/one-too-many").wantError(t, 412, "FAVORITE_LIMIT")
	last := favs(u.post(t, "/v1/sys/auth/favorites/remove", obj{"path": "/qa/29"}))
	settings, _ := u.get(t, "/v1/sys/auth/profile").ok(t)["settings"].(map[string]any)
	if !slices.Equal(obj(settings).strs("favoriteMenus"), last) {
		t.Fatalf("profile settings %v, last reply %v", settings, last)
	}
	add(strings.Repeat("a", 129)).wantError(t, 400, "VALIDATOR")
}

// Basis: internal/core/sys/data/settings.go (the read-modify-write of the settings runs under a row lock so that
// concurrent changes are not lost); docs/practices/transaction.md.
func testFavoritesConcurrent(t *testing.T, e *env) {
	u := e.mkUser(t, uniq("favc"), e.fx.dEast, e.fx.rView)
	const n = 20
	var wg sync.WaitGroup
	codes := make([]int, n)
	for i := range n {
		wg.Go(func() {
			codes[i] = u.post(t, "/v1/sys/auth/favorites/add", obj{"path": fmt.Sprintf("/c/%d", i)}).code
		})
	}
	wg.Wait()
	for i, c := range codes {
		if c != 200 {
			t.Fatalf("add %d: status %d", i, c)
		}
	}
	settings, _ := u.get(t, "/v1/sys/auth/profile").ok(t)["settings"].(map[string]any)
	if got := obj(settings).strs("favoriteMenus"); len(got) != n {
		t.Fatalf("concurrent adds lost some: %d of %d kept: %v", len(got), n, got)
	}
}
