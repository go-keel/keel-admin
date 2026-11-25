//go:build e2e

package e2e

import (
	"slices"
	"strings"
	"sync"
	"testing"
)

// listRoles lists roles as root with the query.
func listRoles(t testing.TB, e *env, query string) []obj {
	t.Helper()
	return e.root().get(t, "/v1/sys/roles"+query).ok(t).items()
}

// Basis: role.proto ListRole, RoleListItem (permCount and createdAt, no perms); internal/core/sys/data/like.go (the
// keyword's _ matches literally).
func testRoleList(t *testing.T, e *env) {
	items := listRoles(t, e, q("keyword", "qa_", "sensitive", 1))
	if got := col(items, "code"); !slices.Equal(got, []string{"qa_sensitive"}) {
		t.Fatalf("sensitive qa_ roles: %v", got)
	}
	if _, ok := items[0]["perms"]; ok || items[0].num("permCount") != 1 || items[0].str("createdAt") == "" {
		t.Fatalf("list row: %v", items[0])
	}
	mustHave(t, "forceOtp=1", col(listRoles(t, e, q("forceOtp", 1, "limit", 200)), "code"), "qa_force_otp")
	for _, it := range listRoles(t, e, q("forceOtp", 1, "limit", 200)) {
		eq(t, "forceOtp", it.num("forceOtp"), 1)
	}
	off := e.mkRole(t, uniq("off"), obj{"status": 0})
	for _, it := range listRoles(t, e, q("status", 2, "limit", 200)) {
		eq(t, "status", it.num("status"), 0)
	}
	mustHave(t, "status=2", ids(listRoles(t, e, q("status", 2, "limit", 200))), off)

	got := col(listRoles(t, e, q("keyword", "e_", "limit", 200)), "code")
	mustHave(t, "keyword e_", got, "qa_force_otp")
	for _, c := range []string{"qa_viewer", "qa_sensitive", "qa_dept_editor"} {
		mustLack(t, "keyword e_", got, c)
	}
}

// Basis: docs/admin/permission-model/management-boundary.md §6 (roles are root only); scenarios.md S7.
func testRolesRootOnly(t *testing.T, e *env) {
	mgr := e.fx.mgr
	mgr.get(t, "/v1/sys/roles").wantError(t, 403, "NOT_ROOT")
	mgr.get(t, path("/v1/sys/roles", e.fx.rView)).wantError(t, 403, "NOT_ROOT")
	mgr.get(t, "/v1/sys/meta/permissions").wantError(t, 403, "NOT_ROOT")
	mgr.get(t, "/v1/sys/meta/resources").wantError(t, 403, "NOT_ROOT")
}

// Basis: error_reason.proto DUPLICATE_ROLE (a deleted role keeps its code; codes compare case-insensitively on both
// databases, migrations/admin/*/0_schema.sql); role.proto CreateRoleRequest (status left out is disabled);
// internal/core/sys/service/perm.go (step-up).
func testCreateRole(t *testing.T, e *env) {
	root := e.root()
	root.post(t, "/v1/sys/roles", obj{"code": uniq("r"), "name": "x"}).wantError(t, 401, "OTP_STEPUP")
	for _, code := range []string{"qa_viewer", "QA_Viewer"} {
		root.post(t, "/v1/sys/roles", obj{"code": code, "name": "x", "status": 1}, root.otp(t)...).wantError(t, 409, "DUPLICATE_ROLE")
	}
	code := uniq("tmp")
	r := root.post(t, "/v1/sys/roles", obj{"code": code, "name": code}, root.otp(t)...).ok(t)
	eq(t, "status", r.num("status"), 0)
	root.del(t, path("/v1/sys/roles", r.id("id")), root.otp(t)...).want(t, 200)
	root.post(t, "/v1/sys/roles", obj{"code": code, "name": code, "status": 1}, root.otp(t)...).wantError(t, 409, "DUPLICATE_ROLE")
}

// Basis: role.proto RoleInfo, UpdateRoleRequest (the whole value: scopes left out are cleared);
// docs/admin/permission-model/data-scope.md §5.4 and internal/core/sys/biz/role.go (invalid data scopes).
func testRoleDetailAndScopes(t *testing.T, e *env) {
	g := e.getRole(t, e.fx.rMgr)
	if len(g.strs("perms")) != len(mgrPerms) || col(g.list("assignableRoles"), "name")[0] == "" || col(g.list("manageDepts"), "name")[0] != "QA-华东" {
		t.Fatalf("GetRole: %v", g)
	}
	if _, ok := g["scopes"]; !ok {
		t.Fatalf("GetRole lacks scopes: %v", g)
	}
	id := e.mkRole(t, uniq("scope"), nil)
	e.updateRole(t, id, obj{"scopes": []obj{{"resourceType": "nope", "scopeType": "ALL"}}}).wantError(t, 400, "INVALID_ARGUMENT")

	// Example: delete after forking (through "Example end")
	// The kernel registers no data-scope resource: valid scopes need the notice example's.
	scopes := e.getRole(t, e.fx.rEd).list("scopes")
	if len(scopes) != 2 {
		t.Fatalf("editor scopes: %v", scopes)
	}
	for _, bad := range [][]obj{
		{{"resourceType": "notice_category", "scopeType": "SELF"}},
		{{"resourceType": "notice", "scopeType": "RESOURCE_GRANT"}},
		{{"resourceType": "notice_category", "scopeType": "RESOURCE_GRANT", "resourceIds": []int64{}}},
		{{"resourceType": "notice_category", "scopeType": "RESOURCE_GRANT", "resourceIds": []int64{999999}}},
		{{"resourceType": "notice", "scopeType": "ALL", "resourceIds": []int64{e.fx.catA}}},
		{{"resourceType": "notice", "scopeType": "ALL"}, {"resourceType": "notice", "scopeType": "SELF"}},
	} {
		e.updateRole(t, id, obj{"scopes": bad}).wantError(t, 400, "INVALID_ARGUMENT")
	}
	e.updateRole(t, id, obj{"scopes": []obj{
		{"resourceType": "notice", "scopeType": "DEPT"}, {"resourceType": "notice_category", "scopeType": "ALL"},
	}}).want(t, 200)
	eq(t, "scopes set", len(e.getRole(t, id).list("scopes")), 2)
	b := e.getRole(t, id)
	root := e.root()
	root.put(t, path("/v1/sys/roles", id), obj{"name": b["name"], "status": 1, "perms": b["perms"]}, root.otp(t)...).want(t, 200)
	eq(t, "scopes left out", len(e.getRole(t, id).list("scopes")), 0)
	// Example end
}

// Basis: docs/admin/permission-model/function-permissions.md §4.1.2 (prerequisites expand), §4.6 (deleting a role
// removes its permissions at once; a disabled role keeps its members); management-boundary.md §6 (sensitive does not restrict
// root); user.proto UserRoleRef.disabled.
func testRoleLifecycle(t *testing.T, e *env) {
	root := e.root()
	id := e.mkRole(t, uniq("life"), obj{"perms": []string{"sys:dept:create"}})
	u := e.mkUser(t, uniq("life"), e.fx.dEast, e.fx.rView, id)
	mustHave(t, "prerequisite closure", u.get(t, "/v1/sys/auth/profile").ok(t).strs("perms"), "sys:dept:view")

	e.updateRole(t, id, obj{"perms": []string{"sys:audit:view"}}).want(t, 200)
	u.get(t, "/v1/sys/audit-logs").want(t, 200)

	e.updateRole(t, id, obj{"status": 0}).want(t, 200)
	u.get(t, "/v1/sys/audit-logs").wantError(t, 403, "FORBIDDEN")
	for _, rr := range root.get(t, path("/v1/sys/users", u.id)).ok(t).list("roles") {
		if rr.id("id") == id && !rr.boolean("disabled") {
			t.Fatalf("disabled role not marked: %v", rr)
		}
	}
	row := listUsers(t, root, q("roleId", id))
	if len(row) != 1 || !slices.ContainsFunc(row[0].list("roles"), func(o obj) bool { return o.id("id") == id && o.boolean("disabled") }) {
		t.Fatalf("list row of a disabled role: %v", row)
	}
	e.updateRole(t, id, obj{"status": 1}).want(t, 200)
	u.get(t, "/v1/sys/audit-logs").want(t, 200)

	root.del(t, path("/v1/sys/roles", id), root.otp(t)...).want(t, 200)
	u.get(t, "/v1/sys/audit-logs").wantError(t, 403, "FORBIDDEN")
	root.get(t, path("/v1/sys/roles", id)).wantError(t, 404, "ROLE_NOT_FOUND")
	mustLack(t, "roles after delete", ids(root.get(t, path("/v1/sys/users", u.id)).ok(t).list("roles")), id)

	sens := e.mkRole(t, uniq("sens"), obj{"sensitive": 1, "perms": []string{"sys:audit:view"}})
	e.updateRole(t, sens, obj{"name": "renamed"}).want(t, 200)
	root.del(t, path("/v1/sys/roles", sens), root.otp(t)...).want(t, 200)
}

// Basis: internal/core/sys/data/role.go Save (locks the role row first): concurrent saves of one role queue, each a whole write.
func testSaveRoleConcurrent(t *testing.T, e *env) {
	root := e.root()
	id := e.mkRole(t, uniq("cr"), nil)
	sets := [][]string{{"sys:account:view"}, {"sys:audit:view", "sys:login_log:view"}, {"sys:dept:view"}}
	depts := []int64{e.fx.dEast, e.fx.dSH, e.fx.dSouth}
	code := root.code(t)
	var wg sync.WaitGroup
	res := make([]response, len(sets))
	for i, s := range sets {
		wg.Go(func() {
			b := obj{"name": "save" + string(rune('a'+i)), "status": 1, "perms": s, "manageDeptIds": []int64{depts[i]}}
			res[i] = send(t, e.base, "PUT", path("/v1/sys/roles", id), b, root.token, "", "X-OTP-Code", code)
		})
	}
	wg.Wait()
	for i, r := range res {
		if r.code != 200 {
			t.Fatalf("save %d: %d %s", i, r.code, r.body)
		}
	}
	g := e.getRole(t, id)
	i := int(g.str("name")[len("save")] - 'a')
	if !slices.Equal(slices.Sorted(slices.Values(g.strs("perms"))), slices.Sorted(slices.Values(sets[i]))) ||
		!slices.Equal(ids(g.list("manageDepts")), []int64{depts[i]}) {
		t.Fatalf("name from save %d, but perms %v and managed departments %v: the saves mixed", i, g.strs("perms"), g.list("manageDepts"))
	}
}

// Basis: docs/admin/permission-model/function-permissions.md §4.7 (permission preview: root sees and acts with the
// chosen roles under its own identity, step-up unaffected, the choice cannot change during a preview);
// internal/core/sys/service/auth.go Profile (isRoot from the principal); data-scope.md §5.2.
func testPermissionPreview(t *testing.T, e *env) {
	root := e.root()
	pv := func(id int64) []string { return []string{"X-Preview-Roles", fmtID(id)} }
	p := root.get(t, "/v1/sys/auth/profile", pv(e.fx.rView)...).ok(t)
	eq(t, "isRoot", p.boolean("isRoot"), false)
	eq(t, "userId", p.id("userId"), root.id)
	if !slices.Equal(p.strs("perms"), []string{"sys:account:view"}) {
		t.Fatalf("preview perms: %v", p.strs("perms"))
	}
	root.get(t, "/v1/sys/roles", pv(e.fx.rView)...).wantError(t, 403, "NOT_ROOT")
	dept := path("/v1/sys/depts", e.fx.dEast)
	body := obj{"name": "QA-华东", "sort": 10, "status": 1}
	root.put(t, dept, body, with(pv(e.fx.rAud), root.otp(t))...).wantError(t, 403, "FORBIDDEN")
	root.put(t, dept, body, pv(e.fx.rMgr)...).wantError(t, 401, "OTP_STEPUP")
	reason := uniq("preview")
	root.put(t, dept, body, with(pv(e.fx.rMgr), root.otp(t), []string{"X-Audit-Reason", reason})...).want(t, 200)
	a := e.auditByReason(t, reason)
	eq(t, "audit operator", a.id("operatorId"), root.id)

	view := e.fx.view
	if a, b := view.get(t, "/v1/sys/auth/profile").ok(t).strs("perms"), view.get(t, "/v1/sys/auth/profile", pv(e.fx.rMgr)...).ok(t).strs("perms"); !slices.Equal(a, b) {
		t.Fatalf("a non-root's preview header must be ignored: %v vs %v", a, b)
	}
	p = root.get(t, "/v1/sys/auth/profile", "X-Preview-Roles", "999999").ok(t)
	if p.boolean("isRoot") || len(p.strs("perms")) != 0 {
		t.Fatalf("preview of an unknown role: %v", p)
	}
	root.get(t, "/v1/sys/roles/options", pv(e.fx.rAud)...).wantError(t, 403, "FORBIDDEN")

	// Example: delete after forking (through "Example end")
	// Data scope follows the chosen roles, computed for root itself (SELF ∩ categories granted).
	mine := root.post(t, "/v1/notices", obj{"title": uniq("pv"), "content": "c", "categoryId": e.fx.catA}).ok(t).id("id")
	other := root.post(t, "/v1/notices", obj{"title": uniq("pv"), "content": "c", "categoryId": e.fx.catB}).ok(t).id("id")
	theirs := e.fx.ed1.post(t, "/v1/notices", obj{"title": uniq("pv"), "content": "c", "categoryId": e.fx.catA}).ok(t).id("id")
	items := root.get(t, "/v1/notices"+q("limit", 200), pv(e.fx.rEd)...).ok(t).items()
	for _, it := range items {
		if it.id("ownerUserId") != root.id || it.id("categoryId") != e.fx.catA {
			t.Fatalf("preview as editor shows %v", it)
		}
	}
	mustHave(t, "preview notices", ids(items), mine)
	mustLack(t, "preview notices", ids(items), other)
	mustLack(t, "preview notices", ids(items), theirs)
	// Example end
}

func fmtID(id int64) string { return strings.TrimSpace(path(id)) }
