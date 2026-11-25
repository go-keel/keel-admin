//go:build e2e

package e2e

import (
	"fmt"
	"strings"
	"testing"
	"time"
)

// audits lists audit entries as the auditor with the query.
func (e *env) audits(t testing.TB, query string) obj {
	t.Helper()
	return e.fx.aud.get(t, "/v1/sys/audit-logs"+query).ok(t)
}

// auditDetail returns one audit entry in full.
func (e *env) auditDetail(t testing.TB, id int64) obj {
	t.Helper()
	return e.fx.aud.get(t, path("/v1/sys/audit-logs", id)).ok(t)
}

// auditByReason returns the newest audit entry whose X-Audit-Reason is reason, in full.
func (e *env) auditByReason(t testing.TB, reason string) obj {
	t.Helper()
	for _, it := range e.audits(t, q("limit", 20)).items() {
		if d := e.auditDetail(t, it.id("id")); d.str("reason") == reason {
			return d
		}
	}
	t.Fatalf("no audit entry with reason %q among the newest 20", reason)
	return nil
}

// Basis: docs/admin/permission-model/sessions-and-audit.md §9 (every write marked Audit is recorded, root's too, only
// after success; reads and logins are not); internal/core/sys/service/perm.go and the business *_perm.go files (the
// Audit marks); audit.proto ListAuditActions.
func testAuditCoverage(t *testing.T, e *env) {
	for _, a := range []string{
		"UserService.CreateUser", "UserService.UpdateUser", "UserService.DisableUser", "UserService.EnableUser",
		"UserService.ResetPassword", "UserService.ResetOTP", "UserService.KickUser", "UserService.SetUserRoot",
		"RoleService.CreateRole", "RoleService.UpdateRole", "RoleService.DeleteRole",
		"DeptService.CreateDept", "DeptService.UpdateDept", "DeptService.DeleteDept",
		"AuthService.UpdateMyProfile", "AuthService.ChangeMyPassword", "AuthService.UnbindOTP",
		"NoticeService.CreateNotice", "NoticeService.DeleteNotice", "NoticeService.BatchDeleteNotice", // Example: delete after forking
		"NoticeService.CreateNoticeCategory", "NoticeService.DeleteNoticeCategory", // Example: delete after forking
	} {
		if e.audits(t, q("action", a, "limit", 1)).total() == 0 {
			t.Errorf("no audit entry for %s", a)
		}
	}
	if e.audits(t, q("operatorId", e.root().id, "limit", 1)).total() == 0 {
		t.Error("no audit entry by root")
	}
	var listed []string
	for _, g := range e.fx.aud.get(t, "/v1/sys/audit-logs/actions").ok(t).list("groups") {
		listed = append(listed, col(g.list("actions"), "action")...)
	}
	for _, a := range []string{
		"AuthService.Login", "AuthService.Profile", "UserService.ListUser", "AuthService.AddFavorite",
		"AuthService.BindOTP", "AuthService.ConfirmOTP",
	} {
		mustLack(t, "audited actions", listed, a)
		eq(t, a+" entries", e.audits(t, q("action", a, "limit", 1)).total(), 0)
	}

	root := e.root()
	before := e.audits(t, q("action", "UserService.CreateUser", "limit", 1)).total()
	root.post(t, "/v1/sys/users", newUser("u_view", e.fx.dEast, e.fx.rView), root.otp(t)...).want(t, 409)
	root.post(t, "/v1/sys/users", newUser(uniq("f"), e.fx.dEast), root.otp(t)...).want(t, 400)
	eq(t, "failed creations recorded", e.audits(t, q("action", "UserService.CreateUser", "limit", 1)).total(), before)
}

// Basis: docs/admin/permission-model/sessions-and-audit.md §9 (target type and id; the payload is redacted);
// audit.proto AuditLogInfo (reason from X-Audit-Reason), AuditLogListItem (no payload); docs/admin/auth.md,
// "Client IP".
func testAuditEntries(t *testing.T, e *env) {
	root := e.root()
	u := e.mkUser(t, uniq("aud"), e.fx.dEast, e.fx.rView)
	reason := "端到端原因 " + uniq("r")

	root.post(t, path("/v1/sys/users", u.id, "disable"), nil, with(root.otp(t), []string{"X-Audit-Reason", reason})...).want(t, 200)
	d := e.auditByReason(t, reason)
	eq(t, "targetType", d.str("targetType"), "User")
	eq(t, "targetId", d.str("targetId"), fmt.Sprint(u.id))
	eq(t, "action", d.str("action"), "UserService.DisableUser")

	reason = uniq("dept")
	id := root.post(t, "/v1/sys/depts", obj{"parentId": e.fx.dSouth, "name": uniq("QA-审计")},
		with(root.otp(t), []string{"X-Audit-Reason", reason})...).ok(t).id("id")
	d = e.auditByReason(t, reason)
	eq(t, "targetType", d.str("targetType"), "Dept")
	eq(t, "targetId", d.str("targetId"), "")
	root.del(t, path("/v1/sys/depts", id), root.otp(t)...).want(t, 200)

	reason = uniq("pwd")
	root.post(t, path("/v1/sys/users", u.id, "reset_pwd"), obj{"password": "E2e-secret-9"},
		with(root.otp(t), []string{"X-Audit-Reason", reason, "X-Forwarded-For", "203.0.113.9"})...).want(t, 200)
	d = e.auditByReason(t, reason)
	if p := d.str("requestPayload"); !strings.Contains(p, `"***"`) || strings.Contains(p, "E2e-secret-9") {
		t.Fatalf("password not redacted: %s", p)
	}
	eq(t, "ip", d.str("ip"), "203.0.113.9")

	reason = uniq("avatar")
	u2 := e.mkUser(t, uniq("av"), e.fx.dEast, e.fx.rView)
	u2.put(t, "/v1/sys/auth/profile", obj{"nickname": "x", "avatar": avatar(1 << 10)}, "X-Audit-Reason", reason).want(t, 200)
	if p := e.auditByReason(t, reason).str("requestPayload"); !strings.Contains(p, "[omitted ") || strings.Contains(p, "base64,") {
		t.Fatalf("avatar not omitted: %s", p)
	}
	for _, it := range e.audits(t, q("limit", 20)).items() {
		if _, ok := it["requestPayload"]; ok {
			t.Fatalf("list row has requestPayload: %v", it)
		}
	}
	e.fx.aud.get(t, "/v1/sys/audit-logs/999999").wantError(t, 404, "AUDIT_LOG_NOT_FOUND")
	e.fx.mgr.get(t, "/v1/sys/audit-logs").wantError(t, 403, "FORBIDDEN")
}

// Basis: audit.proto ListAuditActions, AuditLogListItem (display names follow the request language, action and
// targetType do not).
func testAuditLanguage(t *testing.T, e *env) {
	aud := e.fx.aud.as()
	aud.token = e.fx.aud.token
	names := func(lang string) (string, string, string, string) {
		aud.lang = lang
		g := aud.get(t, "/v1/sys/audit-logs/actions").ok(t).list("groups")[0]
		it := aud.get(t, "/v1/sys/audit-logs"+q("limit", 1)).ok(t).items()[0]
		return g.str("name") + g.list("actions")[0].str("name"), it.str("actionName") + it.str("targetName"), it.str("action"), it.str("targetType")
	}
	g1, n1, a1, t1 := names("zh-CN")
	g2, n2, a2, t2 := names("en")
	if g1 == g2 || n1 == n2 || !hasHan(g1) || hasHan(g2) || !hasHan(n1) || hasHan(n2) {
		t.Fatalf("display names do not follow the language: %q/%q, %q/%q", g1, g2, n1, n2)
	}
	if a1 != a2 || t1 != t2 {
		t.Fatalf("action or targetType changed with the language: %s %s, %s %s", a1, a2, t1, t2)
	}
}

// Basis: audit.proto ListAuditRequest; ADR admin-0003 (time filters are half-open instants); docs/admin/permission-model/sessions-and-audit.md §9 (the
// auditor sees everything, no data scope); internal/core/sys/data/like.go (operatorKeyword's _ is literal).
func testAuditFilters(t *testing.T, e *env) {
	mgr := e.fx.mgr
	u := e.mkUser(t, uniq("af"), e.fx.dEast, e.fx.rView)
	ip := xff()
	mgr.post(t, path("/v1/sys/users", u.id, "kick"), nil, with(mgr.otp(t), ip)...).want(t, 200)
	newest := e.audits(t, q("operatorId", mgr.id, "limit", 1)).items()[0]
	eq(t, "action", newest.str("action"), "UserService.KickUser")
	id := newest.id("id")

	for _, query := range []string{
		q("action", "UserService.KickUser", "limit", 200),
		q("operatorKeyword", "u_mgr", "limit", 200),
		q("operatorId", mgr.id, "limit", 200),
		q("targetType", "User", "limit", 200),
		q("targetId", u.id, "limit", 200),
		q("ip", ip[1], "limit", 200),
	} {
		items := e.audits(t, query).items()
		mustHave(t, query, ids(items), id)
		for _, it := range items {
			if strings.Contains(query, "operatorId") && it.id("operatorId") != mgr.id ||
				strings.Contains(query, "action=") && it.str("action") != "UserService.KickUser" ||
				strings.Contains(query, "targetType") && it.str("targetType") != "User" ||
				strings.Contains(query, "targetId") && it.str("targetId") != fmt.Sprint(u.id) ||
				strings.Contains(query, "ip=") && it.str("ip") != ip[1] {
				t.Fatalf("%s returned %v", query, it)
			}
		}
	}
	// Half-open: from includes an entry at that instant, to excludes it.
	at := newest.str("createdAt")
	ts, err := time.Parse(time.RFC3339, at)
	if err != nil {
		t.Fatal(err)
	}
	from, to := ts.Add(-time.Second).Format(time.RFC3339), ts.Add(time.Second).Format(time.RFC3339)
	mustHave(t, "within", ids(e.audits(t, q("createdFromAt", from, "createdToAt", to, "limit", 200)).items()), id)
	mustLack(t, "to at the entry", ids(e.audits(t, q("createdFromAt", from, "createdToAt", at, "limit", 200)).items()), id)
	mustHave(t, "from at the entry", ids(e.audits(t, q("createdFromAt", at, "createdToAt", to, "limit", 200)).items()), id)
	mustLack(t, "from after", ids(e.audits(t, q("createdFromAt", to, "limit", 200)).items()), id)

	items := e.audits(t, q("operatorKeyword", "u_", "limit", 200)).items()
	for _, it := range items {
		if it.id("operatorId") == e.root().id {
			t.Fatalf("operatorKeyword u_ matched root: %v", it)
		}
	}
	mustHave(t, "operatorKeyword u_", ids(items), id)
}

// loginLogs lists the login log as the auditor with the query.
func (e *env) loginLogList(t testing.TB, query string) []obj {
	t.Helper()
	return e.fx.aud.get(t, "/v1/sys/login-logs"+query).ok(t).items()
}

// Basis: docs/admin/permission-model/sessions-and-audit.md §9 (one entry per attempt, with its outcome);
// internal/core/sys/biz/auth.go Login (the messages); audit.proto ListLoginRequest;
// internal/core/sys/data/like.go (username's _ is literal).
func testLoginLog(t *testing.T, e *env) {
	var msgs []string
	for off := 0; ; off += 200 {
		items := e.loginLogList(t, q("offset", off, "limit", 200))
		msgs = append(msgs, col(items, "msg")...)
		if len(items) < 200 {
			break
		}
	}
	for _, m := range []string{"bad password", "user not found", "locked", "ip not allowed", "otp missing", "bad otp", "disabled"} {
		mustHave(t, "login messages", msgs, m)
	}

	u := e.mkUser(t, uniq("ky_"), e.fx.dEast, e.fx.rView)
	decoy := e.mkUser(t, uniq("kya"), e.fx.dEast, e.fx.rView)
	ip := xff()
	u.loginWith(t, "wrong-password", "", ip...).want(t, 401)
	fail := e.loginLogList(t, q("userId", u.id, "limit", 1))[0]
	for _, query := range []string{
		q("username", "y_", "limit", 200), q("userId", u.id), q("status", 2, "limit", 200), q("ip", ip[1]),
	} {
		items := e.loginLogList(t, query)
		mustHave(t, query, ids(items), fail.id("id"))
		for _, it := range items {
			if strings.Contains(query, "username") && !strings.Contains(it.str("username"), "y_") ||
				strings.Contains(query, "userId") && it.id("userId") != u.id ||
				strings.Contains(query, "status") && it.boolean("success") ||
				strings.Contains(query, "ip=") && it.str("ip") != ip[1] {
				t.Fatalf("%s returned %v", query, it)
			}
		}
	}
	mustLack(t, "username y_", col(e.loginLogList(t, q("username", "y_", "limit", 200)), "username"), decoy.name)
	ts, _ := time.Parse(time.RFC3339, fail.str("createdAt"))
	from, to := ts.Add(-time.Second).Format(time.RFC3339), ts.Add(time.Second).Format(time.RFC3339)
	mustHave(t, "time range", ids(e.loginLogList(t, q("createdFromAt", from, "createdToAt", to, "limit", 200))), fail.id("id"))
	mustLack(t, "time range", ids(e.loginLogList(t, q("createdFromAt", to, "limit", 200))), fail.id("id"))
	e.fx.mgr.get(t, "/v1/sys/login-logs").wantError(t, 403, "FORBIDDEN")
}
