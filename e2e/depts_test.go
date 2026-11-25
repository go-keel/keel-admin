//go:build e2e

package e2e

import (
	"testing"
)

// Basis: dept.proto ListDept (the whole tree, unpaged, with userCount), DeptInfo; docs/admin/api-endpoint.md (lists
// without pagination); docs/admin/permission-model/management-boundary.md §6, "Department CRUD boundary" (a manager
// works inside its subtree, only root creates top-level departments, departments do not move, only empty ones are
// deleted); function-permissions.md §4.6; internal/core/sys/service/perm.go (step-up); ADR admin-0008 (the options'
// bar follows their purpose).
func testDepartments(t *testing.T, e *env) {
	root, mgr := e.root(), e.fx.mgr
	var east obj
	for _, d := range root.get(t, "/v1/sys/depts").ok(t).items() {
		if d.id("id") == e.fx.dEast {
			east = d
		}
	}
	members := 0
	for _, u := range listUsers(t, root, q("deptId", e.fx.dEast, "limit", 200)) {
		if u.str("deptName") == "QA-华东" {
			members++
		}
	}
	if east == nil || int(east.num("userCount")) != members {
		t.Fatalf("east in the tree: %v, members %d", east, members)
	}
	d := root.get(t, path("/v1/sys/depts", e.fx.dSH)).ok(t)
	eq(t, "parentId", d.id("parentId"), e.fx.dEast)
	eq(t, "name", d.str("name"), "QA-上海")
	eq(t, "sort", d.num("sort"), 10)
	eq(t, "status", d.num("status"), 1)

	mgr.get(t, path("/v1/sys/depts", e.fx.dSouth)).wantError(t, 403, "DEPT_OUT_OF_RANGE")
	mgr.post(t, "/v1/sys/depts", obj{"parentId": 0, "name": uniq("top")}, mgr.otp(t)...).wantError(t, 403, "NOT_ROOT")
	mgr.post(t, "/v1/sys/depts", obj{"parentId": e.fx.dSouth, "name": uniq("x")}, mgr.otp(t)...).wantError(t, 403, "DEPT_OUT_OF_RANGE")
	id := mgr.post(t, "/v1/sys/depts", obj{"parentId": e.fx.dSH, "name": uniq("QA-浦东")}, mgr.otp(t)...).ok(t).id("id")
	mgr.put(t, path("/v1/sys/depts", id), obj{"name": uniq("QA-浦东"), "sort": 5, "status": 1, "parentId": 1}, mgr.otp(t)...).want(t, 200)
	eq(t, "parentId after update", root.get(t, path("/v1/sys/depts", id)).ok(t).id("parentId"), e.fx.dSH)

	root.del(t, path("/v1/sys/depts", e.fx.dEast), root.otp(t)...).wantError(t, 412, "DEPT_NOT_EMPTY")
	root.del(t, path("/v1/sys/depts", e.fx.dSouth), root.otp(t)...).wantError(t, 412, "DEPT_NOT_EMPTY")
	root.del(t, path("/v1/sys/depts", id)).wantError(t, 401, "OTP_STEPUP")
	root.put(t, path("/v1/sys/depts", id), obj{"name": "x", "status": 1}).wantError(t, 401, "OTP_STEPUP")
	mgr.del(t, path("/v1/sys/depts", id), mgr.otp(t)...).want(t, 200)
	root.get(t, path("/v1/sys/depts", id)).wantError(t, 404, "DEPT_NOT_FOUND")

	e.fx.view.get(t, "/v1/sys/depts").wantError(t, 403, "FORBIDDEN")
	e.fx.view.get(t, "/v1/sys/depts/options").want(t, 200)
}

// Basis: docs/admin/permission-model/management-boundary.md §6, "Disabling a department affects only new choices";
// error_reason.proto DEPT_DISABLED.
func testDisabledDepartment(t *testing.T, e *env) {
	root, mgr := e.root(), e.fx.mgr
	dis := e.mkDept(t, e.fx.dEast, uniq("QA-停用"), 30)
	u := e.mkUser(t, uniq("dis"), dis, e.fx.rEd)
	n := u.post(t, "/v1/notices", obj{"title": uniq("NDIS"), "content": "c", "categoryId": e.fx.catA}).ok(t).id("id") // Example: delete after forking
	root.put(t, path("/v1/sys/depts", dis), obj{"name": uniq("QA-停用"), "sort": 30, "status": 0}, root.otp(t)...).want(t, 200)

	root.post(t, "/v1/sys/users", newUser(uniq("x"), dis, e.fx.rView), root.otp(t)...).wantError(t, 409, "DEPT_DISABLED")
	mgr.post(t, "/v1/sys/users", newUser(uniq("x"), dis, e.fx.rView), mgr.otp(t)...).wantError(t, 409, "DEPT_DISABLED")
	e.updateUser(t, root, e.fx.ed2.id, obj{"deptId": dis}).wantError(t, 409, "DEPT_DISABLED")

	u.as().login(t).want(t, 200)
	e.updateUser(t, root, u.id, obj{"nickname": "still here"}).want(t, 200)
	mustHave(t, "manager's list", ids(listUsers(t, mgr, q("limit", 200))), u.id)
	mgr.get(t, path("/v1/sys/users", u.id)).want(t, 200)
	mustHave(t, "notices in scope", ids(e.fx.nd.get(t, "/v1/notices"+q("limit", 200)).ok(t).items()), n) // Example: delete after forking
	for _, o := range root.get(t, "/v1/sys/depts/options").ok(t).items() {
		if o.id("id") == dis && !o.boolean("disabled") {
			t.Fatalf("disabled department not marked: %v", o)
		}
	}
	root.del(t, path("/v1/notices", n)).want(t, 200) // Example: delete after forking
}
