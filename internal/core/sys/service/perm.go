package service

import (
	v1 "github.com/go-keel/keel-admin/api/keel_admin/v1"
	"github.com/go-keel/keel-admin/internal/core/perm"
)

// Bindings declares the sys domain's endpoint -> access requirement map,
// registered into the perm kernel at boot via perm.MustRegisterKernel (see
// the entry cmd's di.go). Un-listed operations are denied by default.
var Bindings = perm.Bindings{
	// auth (Login is anonymous via proto allow_anonymous annotation, no binding needed)
	v1.OperationAuthServiceProfile: {}, // authenticated only
	// The user's own account details: reads only their own, so login is enough (the login records are their own too,
	// so no log view code).
	v1.OperationAuthServiceGetMyAccount: {},
	v1.OperationAuthServiceBindOTP:      {},
	// Changing nickname or avatar: any logged-in user changes their own, touching neither permissions nor password, so
	// no step-up.
	v1.OperationAuthServiceUpdateMyProfile: {Audit: true},
	// Changing one's own password: with a hijacked session, changing the password takes over the account (the old
	// password is often obtainable in the same session), so it needs step-up.
	v1.OperationAuthServiceChangeMyPassword: {RequireOTP: true, Audit: true},
	v1.OperationAuthServiceConfirmOTP:       {},
	// Unbinding: the password is the bar (see biz.Unbind), with no step-up, so someone who lost the authenticator can
	// recover on their own.
	v1.OperationAuthServiceUnbindOTP: {Audit: true},
	// Favorite menus: login is enough. They only change the layout of one's own sidebar and touch neither permissions
	// nor other people's data, so no Audit (an audit log full of "who favorited which page" would drown the records
	// that matter) and no step-up.
	v1.OperationAuthServiceAddFavorite:      {},
	v1.OperationAuthServiceRemoveFavorite:   {},
	v1.OperationAuthServiceReorderFavorites: {},

	// dept
	v1.OperationDeptServiceListDept: {Codes: []perm.Code{"sys:dept:view"}},
	v1.OperationDeptServiceGetDept:  {Codes: []perm.Code{"sys:dept:view"}},
	// A picker's option set: the URL is in the department domain, but the bar is bound by **purpose**. It serves the
	// department filter and the "Department" picker of the user management page, so account view permission is
	// enough for those who need it; binding sys:dept:view would make someone with only account permissions hit a
	// 403 on opening the user management page, while the page needs this data to render. The payload has only the
	// three identifying fields (no status/user_count), which the user list's deptName already exposes. The role side
	// of the same principle is ListRoleOptions below.
	v1.OperationDeptServiceListDeptOptions: {Codes: []perm.Code{"sys:account:view"}},
	// Every department write needs step-up: departments carry the scope of permissions (manageable departments and
	// data scopes are both anchored on them), so a change indirectly moves other people's permission boundaries and
	// is treated like a user write.
	v1.OperationDeptServiceCreateDept: {Codes: []perm.Code{"sys:dept:create"}, RequireOTP: true, Audit: true},
	v1.OperationDeptServiceUpdateDept: {Codes: []perm.Code{"sys:dept:update"}, RequireOTP: true, Audit: true},
	v1.OperationDeptServiceDeleteDept: {Codes: []perm.Code{"sys:dept:delete"}, RequireOTP: true, Audit: true},

	// user: privilege escalation and password changes need step-up (see authz.go: users without a binding are guided
	// through OTP_BIND_REQUIRED).
	v1.OperationUserServiceListUser: {Codes: []perm.Code{"sys:account:view"}},
	// The user option set serves the login log's user filter: the bar follows the purpose, the same code as the login
	// log page entry (admin-0008).
	v1.OperationUserServiceListUserOptions: {Codes: []perm.Code{"sys:login_log:view"}},
	v1.OperationUserServiceGetUser:         {Codes: []perm.Code{"sys:account:view"}},
	// Every user management write needs step-up: they all change other people's accounts (create, edit, disable,
	// unlock, kick, reset password, reset OTP), and any of them can affect someone else's login and permissions, so
	// all of them require a code.
	v1.OperationUserServiceCreateUser: {Codes: []perm.Code{"sys:account:create"}, RequireOTP: true, Audit: true},
	// UpdateUser is the single entry point of editing a user, with sys:account:update as the baseline. Nickname,
	// avatar, department, roles and IP allowlist all belong to update (assigning departments or roles and setting
	// IPs have no codes of their own; ARBAC and validation guard the range). The whole form takes one step-up (one
	// code per endpoint, so no code is used twice).
	v1.OperationUserServiceUpdateUser:    {Codes: []perm.Code{"sys:account:update"}, RequireOTP: true, Audit: true},
	v1.OperationUserServiceDisableUser:   {Codes: []perm.Code{"sys:account:disable"}, RequireOTP: true, Audit: true},
	v1.OperationUserServiceEnableUser:    {Codes: []perm.Code{"sys:account:enable"}, RequireOTP: true, Audit: true},
	v1.OperationUserServiceResetPassword: {Codes: []perm.Code{"sys:account:reset_pwd"}, RequireOTP: true, Audit: true},
	v1.OperationUserServiceResetOTP:      {Codes: []perm.Code{"sys:account:reset_otp"}, RequireOTP: true, Audit: true},
	v1.OperationUserServiceKickUser:      {Codes: []perm.Code{"sys:account:kick"}, RequireOTP: true, Audit: true},
	// Granting or revoking super administrator: the ultimate escalation, root only, step-up + audit (no permission
	// codes).
	v1.OperationUserServiceSetUserRoot: {RootOnly: true, RequireOTP: true, Audit: true},

	// role: the whole group is root-only, because roles are the permission surface and seeing or changing them puts a
	// hand on the door to escalation; writes add step-up. RootOnly uses no permission codes, so the sys domain has no
	// codes for the role module.
	v1.OperationRoleServiceListRole: {RootOnly: true},
	v1.OperationRoleServiceGetRole:  {RootOnly: true},
	// A picker's option set, the group's only exception to root-only, bound to sys:account:view: it serves the user
	// list's role filter (which filters by "roles in the system", not "roles I may assign") and the "Assignable roles"
	// dropdown of the role page (root, always allowed). Role names are not new information (the role tags of user
	// list rows already show them); perms and the configuration stay behind GetRolePermissions / GetRole.
	v1.OperationRoleServiceListRoleOptions: {Codes: []perm.Code{"sys:account:view"}},

	v1.OperationRoleServiceCreateRole: {RootOnly: true, RequireOTP: true, Audit: true},
	// Basic info + permissions + the three-dimension boundary are submitted at once: the bar is the same, so it is not
	// split, and one form takes one code.
	v1.OperationRoleServiceUpdateRole: {RootOnly: true, RequireOTP: true, Audit: true},
	v1.OperationRoleServiceDeleteRole: {RootOnly: true, RequireOTP: true, Audit: true},
	// The only two role reads open to non-root, both bound to `sys:account:update`: they serve "assign roles while
	// editing a user", and to hand out a role you must be able to list it and see what it grants. Assigning roles is
	// part of editing a user, not a permission of its own, so it follows update; the guard against overreach is in
	// biz (limited to the union of the actor's assignable roles), not in a code.
	v1.OperationRoleServiceListAssignableRoleOptions: {Codes: []perm.Code{"sys:account:update"}},
	v1.OperationRoleServiceGetRolePermissions:        {Codes: []perm.Code{"sys:account:update"}},

	// meta
	v1.OperationMetaServiceListPermissions: {RootOnly: true},
	v1.OperationMetaServiceListResources:   {RootOnly: true},
	v1.OperationMetaServiceMyPermissions:   {},
	v1.OperationMetaServiceSystemInfo:      {},

	// audit: each of the two logs has its own view code and can be granted separately (for example, only security staff
	// see the login log). Read only, no step-up. Holders of the code see everything, with no data scope: the logs do
	// not record the department at the time, and filtering by the current department would move a person's history
	// along whenever they change departments (see admin-0017).
	v1.OperationAuditServiceListAudit: {Codes: []perm.Code{"sys:audit:view"}},
	v1.OperationAuditServiceGetAudit:  {Codes: []perm.Code{"sys:audit:view"}},
	v1.OperationAuditServiceListLogin: {Codes: []perm.Code{"sys:login_log:view"}},
	// The action option set serves the audit log page's filter: the bar follows the purpose, the same code as the page
	// entry (admin-0008).
	v1.OperationAuditServiceListAuditActions: {Codes: []perm.Code{"sys:audit:view"}},
}
