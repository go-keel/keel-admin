//go:build e2e

// Package e2e runs the production admin binary against real servers and tests it from outside, over HTTP, once per
// round in rounds. docs/guides/testing.md, "End-to-end tests"; docs/admin/testing.md for what the admin adds.
package e2e

import (
	"fmt"
	"testing"
	"time"

	"github.com/redis/go-redis/v9"

	"github.com/go-keel/keel-admin/e2e/harness"
	"github.com/go-keel/keel-admin/internal/testdb"
)

// round is one deployment the tests run against: the driver of each database. A repository edits rounds to match how
// it deploys (the admin and business databases may use different drivers); run a single round with
// make e2e ARGS='-run /<name>'.
type round struct {
	name     string
	admin    string // database.admin: migrations/admin/<driver>/0_schema.sql
	business string // database.default: migrations/business/<driver>/0_schema.sql
}

var rounds = []round{
	{name: "mysql", admin: "mysql", business: "mysql"},
	{name: "postgres", admin: "postgres", business: "postgres"},
}

// timeout is the default wait for anything a test waits on.
const timeout = harness.WaitTimeout

// The root account the admin schema seeds (migrations/admin/*/0_schema.sql, README "Quick start").
const (
	rootName     = "root"
	rootPassword = "admin888"
	rootDept     = 1
	password     = "E2ePass-2026" // every account the tests create
)

// cases run in this order in every round, one after another; each makes its own uniquely named data and asserts only
// on that, or puts back what it changed in the shared fixture.
var cases = []struct {
	name string
	fn   func(t *testing.T, e *env)
}{
	{"root_first_login_binds", testRootFirstLoginBinds},
	{"login_basics", testLoginBasics},
	{"login_validator_message", testLoginValidatorMessage},
	{"login_meta", testLoginMeta},
	{"lockout", testLockout},
	{"ip_allow_list", testIPAllowList},
	{"session_cookie", testSessionCookie},
	{"login_username_case_insensitive", testLoginUsernameCaseInsensitive},
	{"forced_binding", testForcedBinding},
	{"step_up", testStepUp},
	{"otp_quota", testOTPQuota},
	{"self_binding", testSelfBinding},
	{"reset_otp", testResetOTP},
	{"logout", testLogout},
	{"sessions_end", testSessionsEnd},
	{"change_my_password", testChangeMyPassword},
	{"role_change_applies_now", testRoleChangeAppliesNow},
	{"profile_and_my_permissions", testProfileAndMyPermissions},
	{"my_account", testMyAccount},
	{"update_my_profile", testUpdateMyProfile},
	{"favorites", testFavorites},
	{"favorites_concurrent", testFavoritesConcurrent},
	{"user_list_and_detail", testUserListAndDetail},
	{"user_list_filters", testUserListFilters},
	{"create_user_errors", testCreateUserErrors},
	{"management_boundary", testManagementBoundary},
	{"root_self_exceptions", testRootSelfExceptions},
	{"update_user_roles", testUpdateUserRoles},
	{"user_status_idempotent", testUserStatusIdempotent},
	{"user_options", testUserOptions},
	{"role_options", testRoleOptions},
	{"dept_options", testDeptOptions},
	{"set_root", testSetRoot},
	{"create_user_concurrent", testCreateUserConcurrent},
	{"edit_user_concurrent", testEditUserConcurrent},
	{"role_list", testRoleList},
	{"roles_root_only", testRolesRootOnly},
	{"create_role", testCreateRole},
	{"role_detail_and_scopes", testRoleDetailAndScopes},
	{"role_lifecycle", testRoleLifecycle},
	{"save_role_concurrent", testSaveRoleConcurrent},
	{"permission_preview", testPermissionPreview},
	{"audit_entries", testAuditEntries},
	{"audit_language", testAuditLanguage},
	{"audit_filters", testAuditFilters},
	{"login_log", testLoginLog},
	{"departments", testDepartments},
	{"disabled_department", testDisabledDepartment},
	// Example: delete after forking (through "Example end")
	{"notice_scope", testNoticeScope},
	{"notice_batch_and_filters", testNoticeBatchAndFilters},
	{"notice_keyword", testNoticeKeyword},
	{"notice_categories_and_departments", testNoticeCategoriesAndDepartments},
	{"notice_vs_category_delete_concurrent", testNoticeVsCategoryDeleteConcurrent},
	// Example end
	{"audit_coverage", testAuditCoverage},
	{"meta", testMeta},
	{"japanese_fallback", testJapaneseFallback}, // Example: delete after forking
	{"raw_routes", testRawRoutes},
	{"lockout_config", testLockoutConfig},
	{"captcha_base64", testCaptchaBase64},
	{"captcha_turnstile_meta", testCaptchaTurnstileMeta},
	{"otp_max_uses_config", testOTPMaxUsesConfig},
	{"otp_key_changed", testOTPKeyChanged},
	{"session_ttl", testSessionTTL},
	{"brand_config", testBrandConfig},
	{"startup_errors", testStartupErrors},
	{"lockout_fails_open_without_redis", testLockoutFailsOpenWithoutRedis},
}

// env is one round: the shared admin process, the fixture every case starts from, and clients for what the tests
// inspect.
type env struct {
	round     round
	rt        *testing.T // the round's test: owns the shared process
	adminDSN  string     // database.admin
	bizDSN    string     // database.default
	base      string     // base URL of the shared admin
	admin     *harness.Process
	rdb       *redis.Client
	rootLogin response // the first root login, before binding
	rootBind  response // its confirm response
	fx        fixture
}

func TestE2E(t *testing.T) {
	for _, v := range harness.Vars {
		harness.Env(t, v)
	}
	cookieName = harness.Config(t, "config.yaml", "server.http.default.auth.cookie_name")
	harness.BuildTags = "embed_ui" // the release build (make build) embeds the frontend
	harness.Build(t, "admin")
	for _, r := range rounds {
		t.Run(r.name, func(t *testing.T) {
			e := setup(t, r)
			for _, c := range cases {
				if !t.Run(c.name, func(t *testing.T) { c.fn(t, e) }) && e.admin.Exited() {
					t.Fatalf("the shared admin exited during %s; the rest of the round cannot run", c.name)
				}
			}
		})
	}
}

// setup starts a round from empty servers: fresh admin and business databases, Redis database 0 flushed; then the
// admin and the fixture.
func setup(t *testing.T, r round) *env {
	e := &env{round: r, rt: t}
	stamp := time.Now().UnixNano()
	e.adminDSN = testdb.Create(t, "admin", r.admin, fmt.Sprintf("e2e_%s_admin_%d", r.name, stamp))
	e.bizDSN = testdb.Create(t, "business", r.business, fmt.Sprintf("e2e_%s_biz_%d", r.name, stamp))

	harness.FlushRedis(t)
	e.rdb = redis.NewClient(&redis.Options{Addr: harness.Env(t, "TEST_REDIS_ADDR")})
	t.Cleanup(func() { _ = e.rdb.Close() })

	e.base, e.admin = e.start(t, harness.Workdir(t))
	e.fx = e.buildFixture(t)
	return e
}

// processEnv is what every admin process of the round gets: test servers, JSON log, the round's two databases.
func (e *env) processEnv(t testing.TB) []string {
	return append(append(harness.ServerEnv(t),
		harness.DBEnv("admin", e.round.admin, e.adminDSN)...),
		harness.DBEnv("default", e.round.business, e.bizDSN)...)
}

// start starts an admin owned by t in dir, with extra variables, and waits until /health answers; it returns the
// base URL.
func (e *env) start(t testing.TB, dir string, extra ...string) (string, *harness.Process) {
	t.Helper()
	addr := harness.FreeAddr(t)
	p := harness.Start(t, "admin", dir, append(append(e.processEnv(t), "KEEL_HTTP_ADDR="+addr), extra...)...)
	p.WaitHTTP(t, "http://"+addr+"/health")
	return "http://" + addr, p
}

// user returns a client for an existing account on the shared admin.
func (e *env) user(name, pass string, id int64) *user {
	return &user{e: e, name: name, password: pass, id: id}
}
