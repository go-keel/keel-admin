//go:build e2e

package e2e

import (
	"html"
	"strings"
	"testing"

	"github.com/go-keel/keel-admin/e2e/harness"
)

// Basis: meta.proto SystemInfo, ListPermissions, ListResources; docs/admin/shell.md, "Business time zone";
// docs/admin/permission-model/function-permissions.md §4.1 (codes, prerequisites, step-up marks, names per language),
// §4.3.2 (group order); management-boundary.md §6 (roles are root only, so no sys:role:* codes).
func testMeta(t *testing.T, e *env) {
	eq(t, "businessTimeZone", e.fx.view.get(t, "/v1/sys/meta/system-info").ok(t).str("businessTimeZone"),
		harness.Config(t, "app.yaml", "sys.business_time_zone"))

	root := e.root()
	p := root.get(t, "/v1/sys/meta/permissions").ok(t)
	byCode := map[string]obj{}
	for _, it := range p.items() {
		for _, k := range []string{"code", "domain", "module", "action", "deps", "requireOtp", "name"} {
			if _, ok := it[k]; !ok {
				t.Fatalf("permission lacks %s: %v", k, it)
			}
		}
		if strings.HasPrefix(it.str("code"), "sys:role:") {
			t.Fatalf("role permission listed: %v", it)
		}
		byCode[it.str("code")] = it
	}
	eq(t, "sys:account:update requireOtp", byCode["sys:account:update"].boolean("requireOtp"), true)
	eq(t, "ops:notice:view requireOtp", byCode["ops:notice:view"].boolean("requireOtp"), false)        // Example: delete after forking
	mustHave(t, "ops:notice:create deps", byCode["ops:notice:create"].strs("deps"), "ops:notice:view") // Example: delete after forking
	var sysSort, maxSort float64
	for _, g := range p.list("groups") {
		if g.str("domain") == "sys" && g.num("sort") > sysSort {
			sysSort = g.num("sort")
		}
		if g.num("sort") > maxSort {
			maxSort = g.num("sort")
		}
	}
	eq(t, "sys group sorts last", sysSort, maxSort)

	en := root.as()
	en.token, en.lang = root.token, "en"
	zhName := byCode["sys:account:update"].str("name")
	for _, it := range en.get(t, "/v1/sys/meta/permissions").ok(t).items() {
		if it.str("code") == "sys:account:update" && (it.str("name") == zhName || hasHan(it.str("name"))) {
			t.Fatalf("permission name does not follow the language: %q vs %q", it.str("name"), zhName)
		}
	}

	// Example: delete after forking (through "Example end")
	// internal/biz/notice.go (the notice resources).
	res := map[string]obj{}
	for _, r := range root.get(t, "/v1/sys/meta/resources").ok(t).items() {
		res[r.str("res")] = r
	}
	n, c := res["notice"], res["notice_category"]
	if !n.boolean("anchorSelf") || !n.boolean("anchorDept") || n.boolean("grantable") {
		t.Fatalf("notice resource: %v", n)
	}
	if c.boolean("anchorSelf") || c.boolean("anchorDept") || !c.boolean("grantable") {
		t.Fatalf("notice_category resource: %v", c)
	}
	for _, id := range []int64{e.fx.catA, e.fx.catB} {
		mustHave(t, "grantOptions", ids(c.list("grantOptions")), id)
	}
	// Example end
}

// Example: delete after forking
// Basis: ADR admin-0010 and internal/i18n/sys.ja.toml (Japanese covers the generic errors; untranslated login
// messages fall back to English).
func testJapaneseFallback(t *testing.T, e *env) {
	v := e.fx.view.as()
	v.token, v.lang = e.fx.view.token, "ja"
	eq(t, "forbidden in Japanese", v.get(t, "/v1/sys/users/options").wantError(t, 403, "FORBIDDEN"), "この操作を行う権限がありません。")
	msg := v.loginWith(t, "wrong-password", "").wantError(t, 401, "INVALID_CREDENTIAL")
	if !strings.HasPrefix(msg, "Sign-in failed.") {
		t.Fatalf("login message should fall back to English: %q", msg)
	}
}

// Basis: docs/admin/raw-http.md, "Paths go under /v1/"; internal/server/spa.go (brand JSON, index.html and
// version.json not cached, deep links fall back to index.html, the logo anonymous, unknown /v1/ paths 404); docs/admin/shell.md, "Brand"; ADR admin-0012; the proto comments on the fixed
// paths that must not be read as {id}.
func testRawRoutes(t *testing.T, e *env) {
	anon := e.user("", "", 0)
	anon.get(t, "/health").want(t, 200)
	title := harness.Config(t, "app.yaml", "sys.brand.title")
	b := anon.get(t, "/v1/sys/brand").want(t, 200)
	eq(t, "brand", b.json(t).str("title"), title)
	eq(t, "brand Cache-Control", b.header.Get("Cache-Control"), "no-store")
	l := anon.get(t, "/v1/sys/brand/logo").want(t, 200)
	if !strings.HasPrefix(l.header.Get("Content-Type"), "image/") {
		t.Fatalf("logo Content-Type: %q", l.header.Get("Content-Type"))
	}
	idx := anon.get(t, "/").want(t, 200)
	for _, want := range []string{"<title>" + html.EscapeString(title) + "</title>", "window.__BRAND__"} {
		if !strings.Contains(string(idx.body), want) {
			t.Fatalf("index.html lacks %q", want)
		}
	}
	eq(t, "index Cache-Control", idx.header.Get("Cache-Control"), "no-store")
	eq(t, "version.json Cache-Control", anon.get(t, "/version.json").want(t, 200).header.Get("Cache-Control"), "no-store")
	if deep := anon.get(t, "/sys/users").want(t, 200); !strings.Contains(string(deep.body), "window.__BRAND__") {
		t.Fatalf("a frontend deep link should fall back to index.html: %s", deep.body)
	}
	if r := anon.get(t, "/v1/not-a-route"); r.code != 404 || strings.Contains(string(r.body), "<html") {
		t.Fatalf("unknown /v1/ path: %d %s", r.code, r.body)
	}
	for _, p := range []string{"/v1/sys/users/options", "/v1/sys/roles/assignable", "/v1/sys/audit-logs/actions"} {
		e.root().get(t, p).want(t, 200)
	}
	e.root().get(t, "/v1/notices/categories/options").want(t, 200) // Example: delete after forking
}
