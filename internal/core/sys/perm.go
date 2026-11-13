package sys

import "github.com/go-keel/keel-admin/internal/core/perm"

// Groups declares the kernel's business domain / module groups.
//
// Groups are only organisation, not permissions: they cannot be granted, and the UI uses them only as expandable
// containers and select-all entries. Seal requires the business domain and module of every registered code to be
// declared here. Display names are not written here: they derive from the slug as i18n keys (perm.Group.NameKey),
// with the entries in internal/i18n.
var Groups = []perm.Group{
	// The kernel domain is the fixed last resort: business extensions normally use a smaller Sort, so system management
	// always shows last.
	{Domain: "sys", Sort: 1_000_000_000},
	// Modules follow the sidebar menu order: users, departments, audit log, login log.
	{Domain: "sys", Module: "account", Sort: 0},
	{Domain: "sys", Module: "dept", Sort: 1},
	{Domain: "sys", Module: "audit", Sort: 2},
	{Domain: "sys", Module: "login_log", Sort: 3},
}

// Perms declares the sys domain's code-fixed permissions (dept/account/audit/login_log),
// registered into the perm kernel at boot via perm.MustRegisterKernel (see
// the entry cmd's di.go).
//
// A code always has three segments, `<business domain>:<module>:<action>`, and the segments are the organisation
// (see perm.Code).
var Perms = []perm.Permission{
	{Code: "sys:dept:view"},
	{Code: "sys:dept:create", Deps: []perm.Code{"sys:dept:view"}},
	{Code: "sys:dept:update", Deps: []perm.Code{"sys:dept:view"}},
	{Code: "sys:dept:delete", Deps: []perm.Code{"sys:dept:view"}},

	{Code: "sys:account:view"},
	{Code: "sys:account:create", Deps: []perm.Code{"sys:account:view"}},
	{Code: "sys:account:update", Deps: []perm.Code{"sys:account:view"}},
	{Code: "sys:account:disable", Deps: []perm.Code{"sys:account:view"}},
	// Enabling and disabling have separate codes, so support staff can be given the right to enable without the right
	// to disable.
	{Code: "sys:account:enable", Deps: []perm.Code{"sys:account:view"}},
	{Code: "sys:account:reset_pwd", Deps: []perm.Code{"sys:account:view"}},
	// Assigning a department, assigning roles and setting the IP allowlist have no codes of their own: they are parts
	// of the single action "edit user", so whoever has update can change them (how far is bounded by ARBAC and
	// validation), just as they are all set together at creation.
	{Code: "sys:account:reset_otp", Deps: []perm.Code{"sys:account:view"}},
	{Code: "sys:account:kick", Deps: []perm.Code{"sys:account:view"}},

	// Each log has one view code and can be granted separately; logs are append-only, so there are no write codes.
	{Code: "sys:audit:view"},
	{Code: "sys:login_log:view"},

	// The role domain has no permission codes: roles are the permission surface, and being able to view and change
	// roles is a foothold for privilege escalation, so ListRole/GetRole/Create/Update/Delete are all RootOnly (RootOnly
	// takes no code). The only two open to non-root are ListAssignableRoleOptions and GetRolePermissions, bound to
	// `sys:account:update`: both are reads serving "assign roles while editing a user".
	//
	// System-level parameters live in the application configuration (configs/app.yaml → conf.Bootstrap), not in the
	// database, with no management endpoint, so they have no permission code.
}

// Resources declares the sys domain's data-scope resources (anchors/derive),
// registered into the perm kernel at boot via perm.MustRegisterKernel (see
// the entry cmd's di.go).
var Resources = []perm.ResourceDef{}
