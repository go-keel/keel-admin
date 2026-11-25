//go:build e2e

package e2e

import (
	"testing"
)

// fixture is the data every round starts with, built through the API as an operator would: departments, notice
// categories, roles and accounts. Cases that change it put it back; cases that need to break something make their
// own.
type fixture struct {
	root *user

	dEast, dSH, dSouth int64 // departments: east and south under Root, sh under east

	catA, catB int64 // notice categories. Example: delete after forking

	// roles
	rView  int64 // sys:account:view
	rAud   int64 // audit and login log view
	rEd    int64 // sys:dept:create; the notice example adds notice create/delete, notice SELF, catA granted
	rND    int64 // notice view and categories; notice ALL_SUB, categories ALL. Example: delete after forking
	rSens  int64 // sensitive
	rForce int64 // forces an authenticator
	rMgr   int64 // account and department management, manages east, may assign rView, rEd, rSens

	// accounts, logged in; mgr has an authenticator bound
	mgr, view, aud, ed1, ed2, south *user
	nd                              *user // Example: delete after forking
}

// buildFixture binds root's authenticator on its first login and creates the rest.
func (e *env) buildFixture(t *testing.T) fixture {
	t.Helper()
	var f fixture
	f.root = e.user(rootName, rootPassword, 0)
	e.rootLogin = f.root.loginWith(t, rootPassword, "")
	bt := e.rootLogin.ok(t).str("bindToken")
	f.root.id = e.rootLogin.json(t).id("userId")
	e.rootBind = f.root.bind(t, bt, true)
	e.rootBind.want(t, 200)
	e.fx.root = f.root // the helpers below act as root

	f.dEast = e.mkDept(t, rootDept, "QA-华东", 10)
	f.dSH = e.mkDept(t, f.dEast, "QA-上海", 10)
	f.dSouth = e.mkDept(t, rootDept, "QA-华南", 20)

	f.catA = e.mkCategory(t, f.root, "QA-运营") // Example: delete after forking
	f.catB = e.mkCategory(t, f.root, "QA-研发") // Example: delete after forking

	f.rView = e.mkRole(t, "qa_viewer", obj{"perms": []string{"sys:account:view"}})
	f.rAud = e.mkRole(t, "qa_auditor", obj{"perms": []string{"sys:audit:view", "sys:login_log:view"}})
	// The kernel cases assign and filter by qa_dept_editor as an ordinary role; the notice example adds its codes and,
	// as the data scope under test, its scopes.
	ed := obj{"perms": []string{"sys:dept:create"}}
	// Example: delete after forking (through "Example end")
	ed = obj{
		"perms": []string{"sys:dept:create", "ops:notice:create", "ops:notice:delete"},
		"scopes": []obj{
			{"resourceType": "notice", "scopeType": "SELF"},
			{"resourceType": "notice_category", "scopeType": "RESOURCE_GRANT", "resourceIds": []int64{f.catA}},
		},
	}
	f.rND = e.mkRole(t, "qa_notice_dept", obj{
		"perms": []string{"ops:notice:view", "ops:notice:category"},
		"scopes": []obj{
			{"resourceType": "notice", "scopeType": "ALL_SUB"},
			{"resourceType": "notice_category", "scopeType": "ALL"},
		},
	})
	// Example end
	f.rEd = e.mkRole(t, "qa_dept_editor", ed)
	f.rSens = e.mkRole(t, "qa_sensitive", obj{"sensitive": 1, "perms": []string{"sys:audit:view"}})
	f.rForce = e.mkRole(t, "qa_force_otp", obj{"forceOtp": 1, "perms": []string{"sys:audit:view"}})
	f.rMgr = e.mkRole(t, "qa_east_admin", obj{
		"perms":             mgrPerms,
		"manageDeptIds":     []int64{f.dEast},
		"assignableRoleIds": []int64{f.rView, f.rEd, f.rSens},
	})

	f.mgr = e.mkUser(t, "u_mgr", f.dEast, f.rMgr)
	f.view = e.mkUser(t, "u_view", f.dEast, f.rView)
	f.aud = e.mkUser(t, "u_aud", f.dSouth, f.rAud)
	f.ed1 = e.mkUser(t, "u_ed1", f.dSH, f.rEd)
	f.ed2 = e.mkUser(t, "u_ed2", f.dEast, f.rEd)
	f.nd = e.mkUser(t, "u_nd", f.dEast, f.rND) // Example: delete after forking
	f.south = e.mkUser(t, "u_south", f.dSouth, f.rView)
	f.mgr.bind(t, "", false).want(t, 200)
	return f
}

// mgrPerms are the codes of qa_east_admin: account and department management.
var mgrPerms = []string{
	"sys:account:view", "sys:account:create", "sys:account:update", "sys:account:disable", "sys:account:enable",
	"sys:account:reset_pwd", "sys:account:reset_otp", "sys:account:kick",
	"sys:dept:view", "sys:dept:create", "sys:dept:update", "sys:dept:delete",
}

// mkDept creates a department as root and returns its id.
func (e *env) mkDept(t testing.TB, parent int64, name string, sort int) int64 {
	t.Helper()
	root := e.fx.root
	return root.post(t, "/v1/sys/depts", obj{"parentId": parent, "name": name, "sort": sort}, root.otp(t)...).ok(t).id("id")
}

// Example: delete after forking
// mkCategory creates a notice category as u and returns its id.
func (e *env) mkCategory(t testing.TB, u *user, name string) int64 {
	t.Helper()
	return u.post(t, "/v1/notices/categories", obj{"name": name}).ok(t).id("id")
}

// mkRole creates an enabled role as root (code and name both code, plus the fields in body) and returns its id.
func (e *env) mkRole(t testing.TB, code string, body obj) int64 {
	t.Helper()
	b := obj{"code": code, "name": code, "status": 1}
	for k, v := range body {
		b[k] = v
	}
	root := e.fx.root
	return root.post(t, "/v1/sys/roles", b, root.otp(t)...).ok(t).id("id")
}

// mkUser creates an account as root in dept with roles and logs it in.
func (e *env) mkUser(t testing.TB, name string, dept int64, roles ...int64) *user {
	t.Helper()
	u := e.mkAccount(t, name, dept, roles...)
	u.mustLogin(t)
	return u
}

// mkAccount creates an account as root in dept with roles, not logged in.
func (e *env) mkAccount(t testing.TB, name string, dept int64, roles ...int64) *user {
	t.Helper()
	root := e.fx.root
	id := root.post(t, "/v1/sys/users", obj{
		"username": name, "password": password, "nickname": name, "deptId": dept, "roleIds": roles,
	}, root.otp(t)...).ok(t).id("id")
	return e.user(name, password, id)
}

// getRole returns root's view of a role.
func (e *env) getRole(t testing.TB, id int64) obj {
	t.Helper()
	return e.fx.root.get(t, path("/v1/sys/roles", id)).ok(t)
}

// updateRole saves a role as root: its current state from GetRole with the fields in change.
func (e *env) updateRole(t testing.TB, id int64, change obj) response {
	t.Helper()
	cur := e.getRole(t, id)
	b := obj{
		"name": cur["name"], "forceOtp": cur["forceOtp"], "status": cur["status"], "sensitive": cur["sensitive"],
		"perms": cur["perms"], "scopes": cur["scopes"],
		"assignableRoleIds": ids(cur.list("assignableRoles")), "manageDeptIds": ids(cur.list("manageDepts")),
	}
	for k, v := range change {
		b[k] = v
	}
	root := e.fx.root
	return root.put(t, path("/v1/sys/roles", id), b, root.otp(t)...)
}

// mustUpdateRole is updateRole that must succeed; it registers a cleanup putting the role back unless keep is set.
func (e *env) mustUpdateRole(t testing.TB, id int64, change obj) {
	t.Helper()
	before := e.getRole(t, id)
	e.updateRole(t, id, change).want(t, 200)
	t.Cleanup(func() {
		e.updateRole(t, id, obj{
			"name": before["name"], "forceOtp": before["forceOtp"], "status": before["status"],
			"sensitive": before["sensitive"], "perms": before["perms"], "scopes": before["scopes"],
			"assignableRoleIds": ids(before.list("assignableRoles")), "manageDeptIds": ids(before.list("manageDepts")),
		}).want(t, 200)
	})
}

// userBody is the full UpdateUser body of an account as root sees it, with the fields in change.
func (e *env) userBody(t testing.TB, id int64, change obj) obj {
	t.Helper()
	cur := e.fx.root.get(t, path("/v1/sys/users", id)).ok(t)
	b := obj{
		"nickname": cur["nickname"], "avatar": cur["avatar"], "deptId": cur.id("deptId"),
		"roleIds": ids(cur.list("roles")), "ipWhitelist": cur["ipWhitelist"], "email": cur["email"], "phone": cur["phone"],
	}
	for k, v := range change {
		b[k] = v
	}
	return b
}

// updateUser saves an account as actor with the full body from userBody.
func (e *env) updateUser(t testing.TB, actor *user, id int64, change obj, headers ...string) response {
	t.Helper()
	return actor.put(t, path("/v1/sys/users", id), e.userBody(t, id, change), append(actor.otp(t), headers...)...)
}

// root returns the root client.
func (e *env) root() *user { return e.fx.root }
