//go:build e2e

package e2e

import (
	"slices"
	"strings"
	"sync"
	"testing"
)

// listUsers lists accounts as u with the query (q()) and returns the items.
func listUsers(t testing.TB, u *user, query string) []obj {
	t.Helper()
	return u.get(t, "/v1/sys/users"+query).ok(t).items()
}

// newUser is a CreateUser body.
func newUser(name string, dept int64, roles ...int64) obj {
	return obj{"username": name, "password": password, "nickname": name, "deptId": dept, "roleIds": roles}
}

// Basis: user.proto ListUser, UserListItem, UserInfo; docs/admin/api-endpoint.md, "List endpoints" (limit at most 200).
func testUserListAndDetail(t *testing.T, e *env) {
	root := e.root()
	r := root.get(t, "/v1/sys/users"+q("limit", 20)).ok(t)
	if r.total() < 8 || len(r.items()) == 0 {
		t.Fatalf("list: %v", r)
	}
	for _, it := range r.items() {
		for _, k := range []string{"deptName", "roles", "otpBound", "otpForced", "hasIpWhitelist"} {
			if _, ok := it[k]; !ok {
				t.Fatalf("list row lacks %s: %v", k, it)
			}
		}
		for _, k := range []string{"ipWhitelist", "avatar", "deptId"} {
			if _, ok := it[k]; ok {
				t.Fatalf("list row has %s: %v", k, it)
			}
		}
	}
	root.get(t, "/v1/sys/users"+q("limit", 200)).want(t, 200)
	root.get(t, "/v1/sys/users"+q("limit", 201)).wantError(t, 400, "VALIDATOR")

	u := e.mkUser(t, uniq("detail"), e.fx.dSouth, e.fx.rView)
	e.updateUser(t, root, u.id, obj{"ipWhitelist": "198.51.100.0/24", "email": "d@example.com", "phone": "123456"}).want(t, 200)
	d := root.get(t, path("/v1/sys/users", u.id)).ok(t)
	eq(t, "ipWhitelist", d.str("ipWhitelist"), "198.51.100.0/24")
	eq(t, "email", d.str("email"), "d@example.com")
	eq(t, "phone", d.str("phone"), "123456")
	if !strings.HasSuffix(d.str("createdAt"), "Z") {
		t.Fatalf("createdAt: %q", d.str("createdAt"))
	}
	for _, it := range listUsers(t, root, q("keyword", u.name)) {
		eq(t, "hasIpWhitelist", it.boolean("hasIpWhitelist"), true)
		eq(t, "deptName", it.str("deptName"), "QA-华南")
		eq(t, "role name", it.list("roles")[0].str("name"), "qa_viewer")
	}
}

// Basis: user.proto ListUserRequest (status, otpBound, roleId, deptId, keyword over username, nickname, email and
// phone); internal/core/sys/data/like.go (the keyword's _ and % match literally).
func testUserListFilters(t *testing.T, e *env) {
	root := e.root()
	dis := e.mkUser(t, uniq("dis"), e.fx.dEast, e.fx.rView)
	root.post(t, path("/v1/sys/users", dis.id, "disable"), nil, root.otp(t)...).want(t, 200)
	mail := e.mkUser(t, uniq("mail"), e.fx.dEast, e.fx.rView)
	addr := uniq("m") + "@example.com"
	mail.put(t, "/v1/sys/auth/profile", obj{"nickname": mail.name, "email": addr}).want(t, 200)

	check := func(query string, ok func(obj) bool, mustID int64) {
		t.Helper()
		items := listUsers(t, root, query)
		for _, it := range items {
			if !ok(it) {
				t.Fatalf("%s: row %v does not match", query, it)
			}
		}
		mustHave(t, query, ids(items), mustID)
	}
	check(q("status", 2, "limit", 200), func(o obj) bool { return o.num("status") != 1 }, dis.id)
	check(q("otpBound", 1, "limit", 200), func(o obj) bool { return o.boolean("otpBound") }, e.fx.mgr.id)
	check(q("roleId", e.fx.rEd, "limit", 200), func(o obj) bool { return has(col(o.list("roles"), "name"), "qa_dept_editor") }, e.fx.ed1.id)
	check(q("deptId", e.fx.dSH, "limit", 200), func(o obj) bool { return o.str("deptName") == "QA-上海" }, e.fx.ed1.id)
	check(q("keyword", addr), func(o obj) bool { return o.str("email") == addr }, mail.id)

	// _ is literal: "ua..." would match the pattern u_ as a wildcard.
	decoy := e.mkUser(t, uniq("uaq"), e.fx.dEast, e.fx.rView)
	items := listUsers(t, root, q("keyword", "u_", "limit", 200))
	for _, it := range items {
		if !strings.Contains(it.str("username")+it.str("nickname")+it.str("email")+it.str("phone"), "u_") {
			t.Fatalf("keyword u_ matched %v", it)
		}
	}
	mustLack(t, "keyword u_", ids(items), decoy.id)
	mustLack(t, "keyword u_", ids(items), root.id)
	mustHave(t, "keyword u_", ids(items), e.fx.view.id)
}

// Basis: docs/admin/permission-model/function-permissions.md §4.4 (step-up); internal/core/sys/biz/user_admin.go
// Create; error_reason.proto USER_EXISTS, ROLE_REQUIRED; docs/admin/domain-model.md, "Live examples" (avatar size);
// the case-insensitive unique username on both databases (migrations/admin/*/0_schema.sql: utf8mb4_0900_ai_ci on MySQL, the ci collation on PostgreSQL).
func testCreateUserErrors(t *testing.T, e *env) {
	root := e.root()
	name := uniq("cu")
	root.post(t, "/v1/sys/users", newUser(name, e.fx.dEast, e.fx.rView)).wantError(t, 401, "OTP_STEPUP")
	root.post(t, "/v1/sys/users", newUser(name, e.fx.dEast), root.otp(t)...).wantError(t, 400, "ROLE_REQUIRED")
	root.post(t, "/v1/sys/users", newUser("u_view", e.fx.dEast, e.fx.rView), root.otp(t)...).wantError(t, 409, "USER_EXISTS")
	root.post(t, "/v1/sys/users", newUser("U_View", e.fx.dEast, e.fx.rView), root.otp(t)...).wantError(t, 409, "USER_EXISTS")
	b := newUser(name, e.fx.dEast, e.fx.rView)
	b["avatar"] = avatar(300 << 10)
	root.post(t, "/v1/sys/users", b, root.otp(t)...).wantError(t, 400, "AVATAR_TOO_LARGE")
	root.post(t, "/v1/sys/users", newUser(name, e.fx.dEast, e.fx.rView), root.otp(t)...).want(t, 200)
}

// Basis: docs/admin/permission-model/management-boundary.md §6 (departments managed include their subtree, roles
// only from the assignable list, a sensitive role never by a non-root, root and oneself are out of reach);
// scenarios.md S5, S7.
func testManagementBoundary(t *testing.T, e *env) {
	mgr, root := e.fx.mgr, e.root()
	mgr.post(t, "/v1/sys/users", newUser(uniq("b"), e.fx.dSouth, e.fx.rView), mgr.otp(t)...).wantError(t, 403, "DEPT_OUT_OF_RANGE")
	mgr.post(t, "/v1/sys/users", newUser(uniq("b"), e.fx.dEast, e.fx.rAud), mgr.otp(t)...).wantError(t, 403, "ROLE_OUT_OF_RANGE")
	mgr.post(t, "/v1/sys/users", newUser(uniq("b"), e.fx.dEast, e.fx.rSens), mgr.otp(t)...).wantError(t, 403, "ROLE_OUT_OF_RANGE")
	sub := mgr.post(t, "/v1/sys/users", newUser(uniq("b"), e.fx.dSH, e.fx.rEd), mgr.otp(t)...).ok(t).id("id")

	seen := ids(listUsers(t, mgr, q("limit", 200)))
	for _, id := range []int64{mgr.id, e.fx.view.id, e.fx.ed1.id, sub} {
		mustHave(t, "manager's list", seen, id)
	}
	for _, id := range []int64{root.id, e.fx.aud.id, e.fx.south.id} {
		mustLack(t, "manager's list", seen, id)
	}
	eq(t, "no managed departments", e.fx.view.get(t, "/v1/sys/users").ok(t).total(), 0)

	mgr.get(t, path("/v1/sys/users", e.fx.south.id)).wantError(t, 403, "DEPT_OUT_OF_RANGE")
	mgr.get(t, path("/v1/sys/users", root.id)).wantError(t, 403, "TARGET_PROTECTED")
	mgr.post(t, path("/v1/sys/users", root.id, "kick"), nil, mgr.otp(t)...).wantError(t, 403, "TARGET_PROTECTED")
	e.updateUser(t, mgr, mgr.id, obj{"nickname": "me"}).wantError(t, 403, "SELF_MANAGE_FORBIDDEN")
	mgr.post(t, path("/v1/sys/users", mgr.id, "disable"), nil, mgr.otp(t)...).wantError(t, 403, "SELF_MANAGE_FORBIDDEN")
	e.updateUser(t, mgr, e.fx.ed2.id, obj{"deptId": e.fx.dSouth}).wantError(t, 403, "DEPT_OUT_OF_RANGE")
	mgr.post(t, path("/v1/sys/users", e.fx.view.id, "root"), obj{"isRoot": true}, mgr.otp(t)...).wantError(t, 403, "NOT_ROOT")

	// The account operations a department manager may do on its people.
	u := e.mkUser(t, uniq("sub"), e.fx.dEast, e.fx.rView)
	for _, op := range []string{"disable", "enable", "kick"} {
		mgr.post(t, path("/v1/sys/users", u.id, op), nil, mgr.otp(t)...).want(t, 200)
	}
	mgr.post(t, path("/v1/sys/users", u.id, "reset_pwd"), obj{"password": password}, mgr.otp(t)...).want(t, 200)
}

// Basis: docs/admin/permission-model/management-boundary.md §6, "Self-operation" (the lockout-prevention tier:
// root cannot disable itself, drop its own root or change its own IP allow list, but edits the rest).
func testRootSelfExceptions(t *testing.T, e *env) {
	root := e.root()
	root.post(t, path("/v1/sys/users", root.id, "disable"), nil, root.otp(t)...).wantError(t, 403, "SELF_MANAGE_FORBIDDEN")
	root.post(t, path("/v1/sys/users", root.id, "root"), obj{"isRoot": false}, root.otp(t)...).wantError(t, 403, "SELF_MANAGE_FORBIDDEN")
	e.updateUser(t, root, root.id, obj{"ipWhitelist": "127.0.0.1"}).wantError(t, 403, "SELF_MANAGE_FORBIDDEN")
	e.updateUser(t, root, root.id, obj{"nickname": "root"}).want(t, 200)
}

// Basis: docs/admin/permission-model/management-boundary.md §6, "pinning" (roles the editor cannot see stay);
// internal/core/sys/data/user_admin.go Edit; user.proto UpdateUserRequest (role_ids is
// the whole value: empty or missing is refused).
func testUpdateUserRoles(t *testing.T, e *env) {
	root := e.root()
	u := e.mkUser(t, uniq("pin"), e.fx.dEast, e.fx.rView)
	e.updateUser(t, root, u.id, obj{"roleIds": []int64{e.fx.rView, e.fx.rSens}}).want(t, 200)
	e.updateUser(t, e.fx.mgr, u.id, obj{"roleIds": []int64{e.fx.rView, e.fx.rEd}}).want(t, 200)
	names := func() []string {
		return slices.Sorted(slices.Values(col(root.get(t, path("/v1/sys/users", u.id)).ok(t).list("roles"), "name")))
	}
	if got := names(); !slices.Equal(got, []string{"qa_dept_editor", "qa_sensitive", "qa_viewer"}) {
		t.Fatalf("pinned: %v", got)
	}
	e.updateUser(t, root, u.id, obj{"roleIds": []int64{e.fx.rView}}).want(t, 200)
	if got := names(); !slices.Equal(got, []string{"qa_viewer"}) {
		t.Fatalf("root's save: %v", got)
	}
	e.updateUser(t, root, u.id, obj{"roleIds": []int64{}}).wantError(t, 400, "ROLE_REQUIRED")
	root.put(t, path("/v1/sys/users", u.id), obj{"nickname": "x"}, root.otp(t)...).wantError(t, 400, "ROLE_REQUIRED")
}

// Basis: internal/core/sys/biz/user_admin.go ("idempotent: already in the target state").
func testUserStatusIdempotent(t *testing.T, e *env) {
	root := e.root()
	u := e.mkUser(t, uniq("idem"), e.fx.dEast, e.fx.rView)
	root.post(t, path("/v1/sys/users", u.id, "enable"), nil, root.otp(t)...).want(t, 200)
	root.post(t, path("/v1/sys/users", u.id, "disable"), nil, root.otp(t)...).want(t, 200)
	root.post(t, path("/v1/sys/users", u.id, "disable"), nil, root.otp(t)...).want(t, 200)
}

// Basis: user.proto ListUserOptions; ADR admin-0004 (the options' bar is the login log view code).
func testUserOptions(t *testing.T, e *env) {
	aud := e.fx.aud
	r := aud.get(t, "/v1/sys/users/options"+q("keyword", "u_", "limit", 3)).ok(t)
	if len(r.items()) > 3 || r.total() < 3 {
		t.Fatalf("options: %v", r)
	}
	got := ids(aud.get(t, "/v1/sys/users/options"+q("ids", e.fx.view.id, "ids", e.fx.ed1.id)).ok(t).items())
	if !slices.Equal(slices.Sorted(slices.Values(got)), slices.Sorted(slices.Values([]int64{e.fx.view.id, e.fx.ed1.id}))) {
		t.Fatalf("options by ids: %v", got)
	}
	e.fx.mgr.get(t, "/v1/sys/users/options").wantError(t, 403, "FORBIDDEN")
}

// Basis: docs/admin/permission-model/management-boundary.md §6 (assignable = list ∩ enabled ∖ sensitive); role.proto
// ListAssignableRoleOptions (no total), GetRolePermissions, ListRoleOptions.
func testRoleOptions(t *testing.T, e *env) {
	root, mgr := e.root(), e.fx.mgr
	all := root.get(t, "/v1/sys/roles/assignable").ok(t)
	for _, id := range []int64{e.fx.rView, e.fx.rAud, e.fx.rSens, e.fx.rMgr} {
		mustHave(t, "root assignable", ids(all.items()), id)
	}
	m := mgr.get(t, "/v1/sys/roles/assignable").ok(t)
	if _, ok := m["total"]; ok {
		t.Fatalf("assignable has a total: %v", m)
	}
	if got := slices.Sorted(slices.Values(col(m.items(), "code"))); !slices.Equal(got, []string{"qa_dept_editor", "qa_viewer"}) {
		t.Fatalf("manager assignable: %v", got)
	}
	perms := mgr.get(t, path("/v1/sys/roles", e.fx.rEd, "permissions")).ok(t).strs("perms")
	mustHave(t, "role permissions", perms, "sys:dept:create")
	mgr.get(t, path("/v1/sys/roles", e.fx.rAud, "permissions")).wantError(t, 403, "ROLE_OUT_OF_RANGE")
	e.fx.view.get(t, path("/v1/sys/roles", e.fx.rEd, "permissions")).wantError(t, 403, "FORBIDDEN")

	e.mustUpdateRole(t, e.fx.rEd, obj{"status": 0})
	if got := col(mgr.get(t, "/v1/sys/roles/assignable").ok(t).items(), "code"); !slices.Equal(got, []string{"qa_viewer"}) {
		t.Fatalf("assignable with the editor role disabled: %v", got)
	}

	opts := e.fx.view.get(t, "/v1/sys/roles/options").ok(t).items()
	for _, o := range opts {
		for k := range o {
			if k != "id" && k != "code" && k != "name" {
				t.Fatalf("role option has %s: %v", k, o)
			}
		}
	}
	mustHave(t, "options", ids(opts), e.fx.rSens)
	mustLack(t, "excludeSensitive", ids(e.fx.view.get(t, "/v1/sys/roles/options"+q("excludeSensitive", true)).ok(t).items()), e.fx.rSens)
	mustLack(t, "excludeId", ids(e.fx.view.get(t, "/v1/sys/roles/options"+q("excludeId", e.fx.rView)).ok(t).items()), e.fx.rView)
}

// Basis: dept.proto ListDeptOptions (id, parentId, name, disabled; the manager sees its managed subtree).
func testDeptOptions(t *testing.T, e *env) {
	m := ids(e.fx.mgr.get(t, "/v1/sys/depts/options").ok(t).items())
	mustHave(t, "manager's options", m, e.fx.dEast)
	mustHave(t, "manager's options", m, e.fx.dSH)
	mustLack(t, "manager's options", m, e.fx.dSouth)
	mustLack(t, "manager's options", m, rootDept)
	r := e.root().get(t, "/v1/sys/depts/options").ok(t).items()
	for _, id := range []int64{rootDept, e.fx.dEast, e.fx.dSH, e.fx.dSouth} {
		mustHave(t, "root's options", ids(r), id)
	}
	for _, o := range r {
		for k := range o {
			if !has([]string{"id", "parentId", "name", "disabled"}, k) {
				t.Fatalf("department option has %s: %v", k, o)
			}
		}
	}
}

// Basis: internal/core/sys/biz/auth.go Login (root always needs an authenticator); sessions-and-audit.md §8 (granting
// and revoking root end the sessions).
func testSetRoot(t *testing.T, e *env) {
	root := e.root()
	u := e.mkUser(t, uniq("root"), e.fx.dSouth, e.fx.rAud)
	root.post(t, path("/v1/sys/users", u.id, "root"), obj{"isRoot": true}, root.otp(t)...).want(t, 200)
	wantSessionGone(t, u)
	bt := u.login(t).ok(t).str("bindToken")
	if bt == "" {
		t.Fatal("new root, unbound: no bindToken")
	}
	u.bind(t, bt, true).want(t, 200)
	eq(t, "isRoot", u.get(t, "/v1/sys/auth/profile").ok(t).boolean("isRoot"), true)
	root.post(t, path("/v1/sys/users", u.id, "root"), obj{"isRoot": false}, root.otp(t)...).want(t, 200)
	wantSessionGone(t, u)
	u.as().loginWith(t, password, "").wantError(t, 401, "INVALID_CREDENTIAL")
	eq(t, "isRoot", u.login(t).ok(t).boolean("isRoot"), false)
}

// Basis: the case-insensitive unique username (migrations/admin/*/0_schema.sql; error_reason.proto USER_EXISTS):
// of concurrent creations of one name, exactly one succeeds.
func testCreateUserConcurrent(t *testing.T, e *env) {
	root := e.root()
	name := uniq("same")
	code := root.code(t) // one code, three uses
	var wg sync.WaitGroup
	res := make([]response, 3)
	for i := range res {
		wg.Go(func() {
			n := name
			if i == 1 {
				n = strings.ToUpper(name)
			}
			res[i] = send(t, e.base, "POST", "/v1/sys/users", newUser(n, e.fx.dEast, e.fx.rView), root.token, "", "X-OTP-Code", code)
		})
	}
	wg.Wait()
	created := 0
	for _, r := range res {
		switch r.code {
		case 200:
			created++
		case 409:
			r.wantError(t, 409, "USER_EXISTS")
		default:
			t.Fatalf("concurrent create: %d %s", r.code, r.body)
		}
	}
	eq(t, "accounts created", created, 1)
}

// Basis: internal/core/sys/data/user_admin.go Edit (locks the user row first): concurrent edits of one user queue on its row, each a whole
// write.
func testEditUserConcurrent(t *testing.T, e *env) {
	root := e.root()
	u := e.mkUser(t, uniq("ce"), e.fx.dEast, e.fx.rView)
	sets := [][]int64{{e.fx.rView}, {e.fx.rView, e.fx.rEd}, {e.fx.rAud}}
	bodies := make([]obj, len(sets))
	for i, s := range sets {
		bodies[i] = e.userBody(t, u.id, obj{"nickname": "edit" + string(rune('a'+i)), "roleIds": s})
	}
	code := root.code(t)
	var wg sync.WaitGroup
	res := make([]response, len(sets))
	for i := range sets {
		wg.Go(func() {
			res[i] = send(t, e.base, "PUT", path("/v1/sys/users", u.id), bodies[i], root.token, "", "X-OTP-Code", code)
		})
	}
	wg.Wait()
	for i, r := range res {
		if r.code != 200 {
			t.Fatalf("edit %d: %d %s", i, r.code, r.body)
		}
	}
	d := root.get(t, path("/v1/sys/users", u.id)).ok(t)
	i := int(d.str("nickname")[len("edit")] - 'a')
	if got, want := slices.Sorted(slices.Values(ids(d.list("roles")))), slices.Sorted(slices.Values(sets[i])); !slices.Equal(got, want) {
		t.Fatalf("nickname from edit %d but roles %v (want %v): the edits mixed", i, got, want)
	}
}
