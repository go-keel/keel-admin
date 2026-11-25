//go:build e2e

// Example file: delete the whole file after forking

package e2e

import (
	"slices"
	"sync"
	"testing"
	"time"
)

// notice creates a notice as u in category and returns its id.
func notice(t testing.TB, u *user, title string, category int64) int64 {
	t.Helper()
	return u.post(t, "/v1/notices", obj{"title": title, "content": "c", "categoryId": category}).ok(t).id("id")
}

// noticeIDs lists the notices u sees with the query.
func noticeIDs(t testing.TB, u *user, query string) []int64 {
	t.Helper()
	return ids(u.get(t, "/v1/notices"+query).ok(t).items())
}

// Basis: docs/admin/permission-model/data-scope.md §5.2 (SELF, DEPT, ALL_SUB; no condition denies all; root is ALL),
// §5.5 (ownership comes from the identity; out of scope and missing are indistinguishable); scenarios.md S3, S4;
// notice.proto and error_reason.proto of api/admin_app/v1; internal/service/notice_perm.go; ADR admin-0010 and
// internal/i18n/app.ja.toml (Japanese messages).
func testNoticeScope(t *testing.T, e *env) {
	root, ed1, ed2, nd := e.root(), e.fx.ed1, e.fx.ed2, e.fx.nd
	root.post(t, "/v1/notices/categories", obj{"name": "QA-运营"}).wantError(t, 409, "NOTICE_CATEGORY_EXISTS")

	r := ed1.post(t, "/v1/notices", obj{"title": uniq("N1"), "content": "c", "categoryId": e.fx.catA, "ownerUserId": 1, "ownerDeptId": 1}).ok(t)
	n1 := r.id("id")
	eq(t, "ownerUserId", r.id("ownerUserId"), ed1.id)
	eq(t, "ownerDeptId", r.id("ownerDeptId"), e.fx.dSH)
	ed1.post(t, "/v1/notices", obj{"title": "x", "content": "c", "categoryId": e.fx.catB}).wantError(t, 404, "NOTICE_CATEGORY_NOT_FOUND")
	ed1.post(t, "/v1/notices", obj{"title": "x", "content": "c", "categoryId": 999999}).wantError(t, 404, "NOTICE_CATEGORY_NOT_FOUND")
	nra, nrb := notice(t, root, uniq("NRA"), e.fx.catA), notice(t, root, uniq("NRB"), e.fx.catB)

	for _, it := range ed1.get(t, "/v1/notices"+q("limit", 200)).ok(t).items() {
		if it.id("ownerUserId") != ed1.id || it.id("categoryId") != e.fx.catA {
			t.Fatalf("u_ed1 sees %v", it)
		}
	}
	mustHave(t, "u_ed1", noticeIDs(t, ed1, q("limit", 200)), n1)
	for _, id := range []int64{n1, nra, nrb} {
		mustLack(t, "u_ed2", noticeIDs(t, ed2, q("limit", 200)), id)
	}
	msg := ed1.del(t, path("/v1/notices", nra)).wantError(t, 404, "NOTICE_NOT_FOUND")
	eq(t, "out of category", ed1.del(t, path("/v1/notices", nrb)).wantError(t, 404, "NOTICE_NOT_FOUND"), msg)
	eq(t, "missing", ed1.del(t, "/v1/notices/999999").wantError(t, 404, "NOTICE_NOT_FOUND"), msg)
	ed2.del(t, path("/v1/notices", n1)).wantError(t, 404, "NOTICE_NOT_FOUND")
	eq(t, "other category", len(noticeIDs(t, ed1, q("categoryId", e.fx.catB))), 0)

	opts := ed1.get(t, "/v1/notices/categories/options").ok(t)
	if _, ok := opts["total"]; ok || !slices.Equal(ids(opts.items()), []int64{e.fx.catA}) {
		t.Fatalf("u_ed1 category options: %v", opts)
	}
	ed1.get(t, "/v1/notices/categories").wantError(t, 403, "FORBIDDEN")

	seen := noticeIDs(t, nd, q("limit", 200))
	mustHave(t, "u_nd", seen, n1)
	mustLack(t, "u_nd", seen, nra)
	mustLack(t, "u_nd", seen, nrb)
	cats := nd.get(t, "/v1/notices/categories").ok(t)
	if cats.total() < 2 || !has(ids(cats.items()), e.fx.catA) || !has(ids(cats.items()), e.fx.catB) {
		t.Fatalf("u_nd categories: %v", cats)
	}

	scope := func(notice string) []obj {
		return []obj{
			{"resourceType": "notice", "scopeType": notice},
			{"resourceType": "notice_category", "scopeType": "RESOURCE_GRANT", "resourceIds": []int64{e.fx.catA}},
		}
	}
	e.mustUpdateRole(t, e.fx.rEd, obj{"scopes": scope("DEPT")})
	mustLack(t, "DEPT", noticeIDs(t, ed2, q("limit", 200)), n1)
	e.updateRole(t, e.fx.rEd, obj{"scopes": scope("ALL_SUB")}).want(t, 200)
	mustHave(t, "ALL_SUB", noticeIDs(t, ed2, q("limit", 200)), n1)

	e.mustUpdateRole(t, e.fx.rND, obj{"scopes": []obj{{"resourceType": "notice", "scopeType": "ALL_SUB"}}})
	eq(t, "no category condition", len(noticeIDs(t, nd, q("limit", 200))), 0)

	all := noticeIDs(t, root, q("limit", 200))
	for _, id := range []int64{n1, nra, nrb} {
		mustHave(t, "root", all, id)
	}
	for _, it := range root.get(t, "/v1/notices"+q("limit", 200)).ok(t).items() {
		if it.str("ownerDeptName") == "" || it.str("ownerUserName") == "" {
			t.Fatalf("row without owner names: %v", it)
		}
	}
	e.fx.view.get(t, "/v1/notices").wantError(t, 403, "FORBIDDEN")
	ja := ed1.as()
	ja.token, ja.lang = ed1.token, "ja"
	if m := ja.del(t, "/v1/notices/999999").wantError(t, 404, "NOTICE_NOT_FOUND"); m != "お知らせが見つかりません。" {
		t.Fatalf("Japanese message: %q", m)
	}
}

// Basis: notice.proto BatchDeleteNotice and its request rules (unique ids, 1 to 200); internal/biz/notice.go (all or
// nothing, one audit entry); docs/admin/tables.md, "Pagination"; ADR admin-0003 (date filters count business days in
// sys.business_time_zone, both ends included).
func testNoticeBatchAndFilters(t *testing.T, e *env) {
	root, ed1 := e.root(), e.fx.ed1
	n2, n3 := notice(t, ed1, uniq("N2"), e.fx.catA), notice(t, ed1, uniq("N3"), e.fx.catA)
	nra := notice(t, root, uniq("NRA"), e.fx.catA)
	ed1.post(t, "/v1/notices/batch-delete", obj{"ids": []int64{n2, nra}}).wantError(t, 404, "NOTICE_NOT_FOUND")
	mustHave(t, "after a failed batch", noticeIDs(t, ed1, q("limit", 200)), n2)
	before := e.audits(t, q("action", "NoticeService.BatchDeleteNotice", "limit", 1)).total()
	ed1.post(t, "/v1/notices/batch-delete", obj{"ids": []int64{n2, n3}}).want(t, 200)
	eq(t, "batch audit entries", e.audits(t, q("action", "NoticeService.BatchDeleteNotice", "limit", 1)).total(), before+1)
	many := make([]int64, 201)
	for i := range many {
		many[i] = int64(i + 1)
	}
	for _, bad := range [][]int64{{nra, nra}, {}, many} {
		ed1.post(t, "/v1/notices/batch-delete", obj{"ids": bad}).wantError(t, 400, "VALIDATOR")
	}

	shanghai := time.FixedZone("Asia/Shanghai", 8*3600) // no daylight saving
	today := time.Now().In(shanghai)
	d, y := today.Format(time.DateOnly), today.AddDate(0, 0, -1).Format(time.DateOnly)
	mustHave(t, "today", noticeIDs(t, root, q("createdFromDate", d, "createdToDate", d, "limit", 200)), nra)
	mustLack(t, "yesterday", noticeIDs(t, root, q("createdFromDate", y, "createdToDate", y, "limit", 200)), nra)
	root.get(t, "/v1/notices"+q("limit", 201)).wantError(t, 400, "VALIDATOR")
}

// Basis: notice.proto ListNoticeRequest.keyword ("fuzzy match on the title"); internal/data/notice.go List. The
// keyword matches literally, as the kernel's keywords do (internal/core/sys/data/like.go): % and _ are not wildcards.
func testNoticeKeyword(t *testing.T, e *env) {
	root := e.root()
	tag := uniq("kw")
	lit := notice(t, root, tag+" 50%_off 公告", e.fx.catA)
	wild := notice(t, root, tag+" 50xyoff 公告", e.fx.catA)
	plain := noticeIDs(t, root, q("keyword", tag+" 50", "limit", 200))
	mustHave(t, "plain keyword", plain, lit)
	mustHave(t, "plain keyword", plain, wild)
	mustHave(t, "Chinese keyword", noticeIDs(t, root, q("keyword", "off 公", "limit", 200)), lit)
	got := noticeIDs(t, root, q("keyword", tag+" 50%_off", "limit", 200))
	mustHave(t, "literal keyword", got, lit)
	mustLack(t, "literal keyword", got, wild)
	upper := noticeIDs(t, root, q("keyword", "KW", "limit", 200))
	mustHave(t, "case-insensitive keyword", upper, lit)
}

// Basis: error_reason.proto NOTICE_CATEGORY_IN_USE; docs/admin/permission-model/data-scope.md §5.5 (deleting a
// category removes its grants), §5.2 ("A new instance is granted to no role automatically"), §5.4b (notices keep
// their owner department; a department owning notices cannot be deleted); management-boundary.md §6.
func testNoticeCategoriesAndDepartments(t *testing.T, e *env) {
	root := e.root()
	root.del(t, path("/v1/notices/categories", e.fx.catA)).wantError(t, 412, "NOTICE_CATEGORY_IN_USE")

	catC := e.mkCategory(t, root, uniq("QA-临时"))
	e.mustUpdateRole(t, e.fx.rEd, obj{"scopes": []obj{
		{"resourceType": "notice", "scopeType": "SELF"},
		{"resourceType": "notice_category", "scopeType": "RESOURCE_GRANT", "resourceIds": []int64{e.fx.catA, catC}},
	}})
	root.del(t, path("/v1/notices/categories", catC)).want(t, 200)
	for _, s := range e.getRole(t, e.fx.rEd).list("scopes") {
		if s.str("resourceType") == "notice_category" && !slices.Equal(s.strs("resourceIds"), []string{path(e.fx.catA)}) {
			t.Fatalf("grant of a deleted category kept: %v", s)
		}
	}
	for _, r := range root.get(t, "/v1/sys/meta/resources").ok(t).items() {
		mustLack(t, "grantOptions", ids(r.list("grantOptions")), catC)
	}

	// A category made by someone whose grants do not cover it is invisible to them, the creator included.
	e.mustUpdateRole(t, e.fx.rND, obj{"scopes": []obj{
		{"resourceType": "notice", "scopeType": "ALL_SUB"},
		{"resourceType": "notice_category", "scopeType": "RESOURCE_GRANT", "resourceIds": []int64{e.fx.catA}},
	}})
	own := e.mkCategory(t, e.fx.nd, uniq("QA-自建"))
	mustLack(t, "own category", ids(e.fx.nd.get(t, "/v1/notices/categories"+q("limit", 200)).ok(t).items()), own)
	mustLack(t, "own category", ids(e.fx.nd.get(t, "/v1/notices/categories/options").ok(t).items()), own)
	e.fx.nd.del(t, path("/v1/notices/categories", own)).wantError(t, 404, "NOTICE_CATEGORY_NOT_FOUND")
	root.del(t, path("/v1/notices/categories", own)).want(t, 200)

	// Notices keep the department they were posted from.
	dept := e.mkDept(t, e.fx.dEast, uniq("QA-迁出"), 40)
	u := e.mkUser(t, uniq("mover"), dept, e.fx.rEd)
	n := notice(t, u, uniq("N"), e.fx.catA)
	e.updateUser(t, root, u.id, obj{"deptId": e.fx.dEast}).want(t, 200)
	root.del(t, path("/v1/sys/depts", dept), root.otp(t)...).wantError(t, 412, "DEPT_NOT_EMPTY")
	mustHave(t, "own notice after moving", noticeIDs(t, u, q("limit", 200)), n)
	eq(t, "new notice's department", u.post(t, "/v1/notices", obj{"title": uniq("N4"), "content": "c", "categoryId": e.fx.catA}).ok(t).id("ownerDeptId"), e.fx.dEast)
	root.del(t, path("/v1/notices", n)).want(t, 200)
	root.del(t, path("/v1/sys/depts", dept), root.otp(t)...).want(t, 200)
}

// Basis: internal/data/notice.go Create and DeleteCategory (both lock the category row, so a notice is never left in
// a deleted category); docs/practices/transaction.md.
func testNoticeVsCategoryDeleteConcurrent(t *testing.T, e *env) {
	root := e.root()
	for range 3 {
		cat := e.mkCategory(t, root, uniq("QA-并发"))
		var wg sync.WaitGroup
		creates := make([]response, 5)
		var del response
		for i := range creates {
			wg.Go(func() {
				creates[i] = send(t, e.base, "POST", "/v1/notices", obj{"title": uniq("c"), "content": "c", "categoryId": cat}, root.token, "")
			})
		}
		wg.Go(func() { del = send(t, e.base, "DELETE", path("/v1/notices/categories", cat), nil, root.token, "") })
		wg.Wait()
		created := 0
		for _, r := range creates {
			switch r.code {
			case 200:
				created++
			case 404:
				r.wantError(t, 404, "NOTICE_CATEGORY_NOT_FOUND")
			default:
				t.Fatalf("create: %d %s", r.code, r.body)
			}
		}
		left := len(noticeIDs(t, root, q("categoryId", cat, "limit", 200)))
		switch del.code {
		case 200:
			if created != 0 || left != 0 {
				t.Fatalf("category deleted, yet %d notices were created and %d remain in it", created, left)
			}
		case 412:
			del.wantError(t, 412, "NOTICE_CATEGORY_IN_USE")
			eq(t, "notices in the kept category", left, created)
		default:
			t.Fatalf("delete category: %d %s", del.code, del.body)
		}
	}
}
