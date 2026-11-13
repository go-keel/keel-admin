package biz

import (
	"context"
	"errors"

	"github.com/go-keel/keel/pkg/pwd"
)

// UserFilter is the ListUser query filter.
//
// The zero value filters nothing, matching the existing convention of DeptID. It uses sentinel values rather than
// pointers: this contract has always expressed tri-states that way (see dept_id in the proto), and mixing in a
// second form would leave callers guessing.
type UserFilter struct {
	DeptID  int64
	Keyword string
	// Status is 0 = all, 1 = active, 2 = disabled (sys_user.status 1/0).
	Status int32
	// OTPBound is 0 = all, 1 = authenticator bound, 2 = not bound.
	OTPBound int32
	// RoleID is 0 for all; otherwise it limits the result to users holding that role.
	RoleID int64
	// IDs, when not empty, takes only these IDs (to show already selected options).
	IDs []int64
}

// UserAdminRepo extends persistence for account management.
type UserAdminRepo interface {
	UserRepo
	// List limits the result to the department set inDeptIDs when it is not nil (data scope filtering); nil means
	// no limit.
	List(ctx context.Context, inDeptIDs []int64, f UserFilter, offset, limit int) ([]*User, int64, error)
	Create(ctx context.Context, u *User, roleIDs []int64) error
	// Edit writes the fields UserEditChanges carries in one transaction. Like Create (which writes sys_user and
	// sys_user_role in one transaction), an edit must land completely or not at all, and biz cannot build that
	// guarantee by calling several SetXxx field by field.
	//
	// Inside the transaction it first locks the user's sys_user row by primary key (a missing user is
	// ErrInvalidCredential). When changes.Roles is not nil, it then reads the roles the user currently holds
	// (disabled included, deleted excluded), passes them to it and replaces sys_user_role as a whole if the result
	// says so. An error from it rolls back the whole edit.
	Edit(ctx context.Context, id int64, changes UserEditChanges) error
	// RolesOf returns the roles a user holds (disabled included), ordered by role ID ascending.
	RolesOf(ctx context.Context, id int64) ([]UserRole, error)
	// RolesOfUsers is the page version of RolesOf: it fetches the roles of all these users at once, keyed by user
	// ID; users without roles are absent from the map. Lists fetch per page in batch, not per row.
	RolesOfUsers(ctx context.Context, ids []int64) (map[int64][]UserRole, error)
	// ForceOTPUsers returns those of these users whose roles force them to bind an authenticator (the value is
	// always true), with the same definition as the login check (OTPRepo.UserForceOTP): only effective roles count.
	ForceOTPUsers(ctx context.Context, ids []int64) (map[int64]bool, error)
}

// CacheInvalidator drops cached role permission data (D2: keyed by role, not user).
type CacheInvalidator interface {
	InvalidateRole(ctx context.Context, roleID int64)
}

// OTPResetter is the OTP capability ResetOTP needs: one method rather than the whole *OTPUseCase, so a biz test
// covers it with a one-line fake instead of a full cipher and replay guard setup. *OTPUseCase satisfies the
// interface as is, so di.go passes it in directly with no adapter.
type OTPResetter interface {
	Reset(ctx context.Context, userID int64) error
}

// SelfOpTier is the strictness of the "load the target" entry point towards operations on oneself. The two tiers
// answer two different questions: the default tier asks "may you manage this person" (non-root can never target
// itself, root is not limited); the anti-lockout tier also asks "can anyone still manage the system afterwards"
// (even root cannot act on itself). Mixing them would lead later readers to smooth out the three anti-lockout
// cases as inconsistencies in the permission check, so they get a tier of their own.
type SelfOpTier int

const (
	// SelfOpDefault rejects non-root acting on itself and does not limit root; most management actions use it.
	SelfOpDefault SelfOpTier = iota
	// SelfOpAntiLockout also rejects root acting on itself. The three anti-lockout exceptions use it: Disable,
	// SetRoot and changing the IP allowlist in an edit (see ADR admin-0014).
	SelfOpAntiLockout
)

// checkSelfOp is the one decision of "may you do this to yourself": non-root acting on itself is always rejected,
// and the anti-lockout tier rejects root as well. target and the IP allowlist change in Edit both use it.
func checkSelfOp(actor *Actor, id int64, tier SelfOpTier) error {
	if actor.UserID == id && (!actor.IsRoot || tier == SelfOpAntiLockout) {
		return ErrSelfManageForbidden
	}
	return nil
}

// UserAdminUseCase manages accounts with dimension-three ARBAC enforcement.
type UserAdminUseCase struct {
	repo  UserAdminRepo
	arbac *ARBACUseCase
	otp   OTPResetter
}

// NewUserAdminUseCase builds the account-management use case.
func NewUserAdminUseCase(repo UserAdminRepo, arbac *ARBACUseCase, otp OTPResetter) *UserAdminUseCase {
	return &UserAdminUseCase{repo: repo, arbac: arbac, otp: otp}
}

// target is the single entry point of every management action: it decides "may you act on this person" and
// returns the loaded target for the caller to reuse. It does three things in order (non-root acting on itself is
// rejected, root passes per tier; in the anti-lockout tier root acting on itself is rejected too; the target must
// be within the actor's manageable range) with one database query: it decides on the user already loaded instead
// of letting EnsureTargetManageable call FindByID again, so one edit does not query twice.
func (uc *UserAdminUseCase) target(ctx context.Context, actor *Actor, id int64, tier SelfOpTier) (*User, error) {
	if err := checkSelfOp(actor, id, tier); err != nil {
		return nil, err
	}
	u, err := uc.repo.FindByID(ctx, id)
	if err != nil {
		return nil, err
	}
	if err := uc.arbac.ensureManageable(ctx, actor, u); err != nil {
		return nil, err
	}
	return u, nil
}

// List filters by the actor's manageable department range (no limit for root). The fields of f are filters the user
// chooses, which differ from the department range here (the ARBAC boundary, not optional): the former is a query,
// the latter is authorization.
func (uc *UserAdminUseCase) List(ctx context.Context, actor *Actor, f UserFilter, offset, limit int) ([]*User, int64, error) {
	if limit <= 0 { // a negative limit also counts as "not given": gorm treats a negative Limit as unlimited
		limit = defaultPageSize
	}
	ids, all, err := uc.arbac.ManageableDeptIDs(ctx, actor)
	if err != nil {
		return nil, 0, err
	}
	if all {
		return uc.repo.List(ctx, nil, f, offset, limit)
	}
	if len(ids) == 0 {
		return []*User{}, 0, nil // no manageable departments: empty set, deny-all fallback
	}
	return uc.repo.List(ctx, ids, f, offset, limit)
}

// Default page sizes, shared by this package's lists. The upper limit is not in code: it is the buf.validate lte on
// each list request's limit, so the transport entry rejects values out of range and nothing is clipped here.
const (
	defaultPageSize = 20
	// defaultUserOptionsLimit is the default batch size of user options; the batch limit depends on the use case
	// and is likewise written only as the lte on ListUserOptionsRequest.limit.
	defaultUserOptionsLimit = 50
)

// Options returns user options: with f.IDs it takes only those users, otherwise it fetches batches by f.Keyword.
// It sees all users and ignores the manageable department range: it serves the login log filter, and the login
// log itself shows everyone (ADR admin-0017).
func (uc *UserAdminUseCase) Options(ctx context.Context, f UserFilter, offset, limit int) ([]*User, int64, error) {
	if len(f.IDs) > 0 {
		return uc.repo.List(ctx, nil, UserFilter{IDs: f.IDs}, 0, len(f.IDs))
	}
	if limit <= 0 { // a negative limit also counts as "not given": gorm treats a negative Limit as unlimited
		limit = defaultUserOptionsLimit
	}
	return uc.repo.List(ctx, nil, UserFilter{Keyword: f.Keyword}, offset, limit)
}

// UserCreate is the fields one "create user" submission carries. Most are strings, so they are passed by name to
// avoid mixing up positions.
type UserCreate struct {
	Username string
	Password string
	Nickname string
	Avatar   string
	Email    string
	Phone    string
	DeptID   int64
	RoleIDs  []int64
	// IPWhitelist is the initial login IP allowlist; empty means no restriction. It is written together with the
	// account: creating the account first and setting it separately would, with an invalid allowlist, leave the
	// account created while the endpoint reports an error, and a retry would hit "user already exists".
	IPWhitelist string
}

// Create creates ordinary users only; root status is not set here. Granting super administrator is a separate
// root-only, step-up operation (SetUserRoot) done after creation. Avatar, contact details and the IP allowlist are
// editable like in Edit and can be set at creation. As in Edit, every check that can fail runs before anything is
// written, and the account, roles and allowlist are written at once.
func (uc *UserAdminUseCase) Create(ctx context.Context, actor *Actor, in UserCreate) (*User, error) {
	username, deptID, roleIDs := in.Username, in.DeptID, in.RoleIDs
	if err := validateAvatar(in.Avatar); err != nil {
		return nil, err
	}
	// A new account is never the operator, so the IP allowlist needs no self-lockout guard.
	if err := ValidateIPWhitelist(in.IPWhitelist); err != nil {
		return nil, err
	}
	// Roles are required: an account holds at least one role. The department is "required" through
	// CanManageDept: deptID=0 (no department) is in no non-root manageable set, so a non-root actor assigning 0 is
	// always stopped by ErrDeptOutOfRange, and only root can create an account without a department.
	if len(roleIDs) == 0 {
		return nil, ErrRoleRequired
	}
	if ok, err := uc.arbac.CanManageDept(ctx, actor, deptID); err != nil {
		return nil, err
	} else if !ok {
		return nil, ErrDeptOutOfRange
	}
	if ok, err := uc.arbac.CanAssignRoles(ctx, actor, roleIDs); err != nil {
		return nil, err
	} else if !ok {
		return nil, ErrRoleOutOfRange
	}
	// This lookup only rejects early and cannot stop concurrent creation of the same name: sys_user.uk_username
	// decides in the end, and repo.Create returns ErrUserExists on the key conflict. FindByUsername returns
	// ErrInvalidCredential when nothing is found (the same sentinel login uses against enumeration), and only that
	// lets creation continue; a failing query is returned as is and must not be taken as "does not exist".
	if _, err := uc.repo.FindByUsername(ctx, username); err == nil {
		return nil, ErrUserExists
	} else if !errors.Is(err, ErrInvalidCredential) {
		return nil, err
	}
	hash, err := pwd.HashPassword(in.Password)
	if err != nil {
		return nil, err
	}
	u := &User{
		Username: username, Password: hash, Nickname: in.Nickname, Avatar: in.Avatar,
		Email: in.Email, Phone: in.Phone, DeptID: deptID, IPWhitelist: in.IPWhitelist, Status: 1,
	}
	if err := uc.repo.Create(ctx, u, roleIDs); err != nil {
		return nil, err
	}
	return u, nil
}

// UserEdit is all the fields one "edit user" submission carries. Nickname, avatar, contact details, department,
// roles and IP allowlist are one UX action (ADR admin-0007), so they form one call; Edit compares each with the
// target's current value and checks and stores only what really changed.
type UserEdit struct {
	Nickname    string
	Avatar      string
	Email       string
	Phone       string
	DeptID      int64
	RoleIDs     []int64
	IPWhitelist string
}

// UserEditChanges is the write set Edit computes, carrying only the fields that really changed. repo.Edit honours
// only the fields it carries and does not compare with current values again, so pointers express "this field did
// not change, do not write it".
//
// Roles are given as an algorithm, not a result: which roles invisible to the operator are kept depends on what the
// user holds at the moment of writing, and only what is read after locking the user row counts (see RoleResolver).
type UserEditChanges struct {
	Nickname    *string
	Avatar      *string
	Email       *string
	Phone       *string
	DeptID      *int64
	IPWhitelist *string
	// Roles is nil when this edit does not touch roles.
	Roles RoleResolver
}

// RoleResolver is called by repo.Edit after it locks the sys_user row and reads the user's current roles. It returns
// the final write set and whether to write (with changed=false, final is meaningless and roles are not written).
//
// It is a pure function: while the lock is held it touches only this transaction. The operator's assignable range
// and whether the submitted set is valid are computed before the transaction and captured in the closure, so it
// does not query the database. The transaction function must be able to rerun as a whole, so it also changes no
// outside variable.
type RoleResolver func(current []int64) (final []int64, changed bool, err error)

// planRoleEdit is the half of a role edit before the transaction: it loads the operator's assignable range once,
// checks that the whole submitted set is within it (the range excludes sensitive roles, so sensitive roles are
// rejected here too), and captures the range and the submitted set into the half inside the transaction.
//
// A submitted set beyond the range is rejected at once, without waiting to find that it "changed": such a set can
// never equal the current assignable part (which lies entirely within the range), so both orders reach the same
// conclusion, and rejecting first saves a transaction.
func (uc *UserAdminUseCase) planRoleEdit(ctx context.Context, actor *Actor, submitted []int64) (RoleResolver, error) {
	allowed, all, err := uc.arbac.AssignableRange(ctx, actor)
	if err != nil {
		return nil, err
	}
	if !all {
		for _, rid := range submitted {
			if !allowed[rid] {
				return nil, ErrRoleOutOfRange
			}
		}
	}
	submitted = append([]int64(nil), submitted...)
	return func(current []int64) ([]int64, bool, error) {
		return resolveRoleEdit(allowed, all, submitted, current)
	}, nil
}

// resolveRoleEdit is the half of a role edit inside the transaction: current is the roles read after locking the
// user row. It decides whether the submitted set differs from the "assignable part" of current and, if so,
// computes the final write set including the pinned out-of-range roles.
//
// It compares the "assignable part" rather than everything: roles on the target beyond the actor's assignable
// range (such as a sensitive role root gave) are never in the selector and never in the submitted set, so
// comparing everything would always find a change. The final write set is the submitted set plus the pinned
// out-of-range roles: roles are replaced as a whole, and otherwise "not visible in the dropdown, absent on submit,
// removed" would strip roles beyond the operator's authority. Out-of-range roles must come from current read under
// the lock: with a snapshot from outside the transaction, a role someone else just added between the two reads and
// invisible to this operator would be removed by this save. The write set cannot be empty (an account holds at
// least one role). For root (all=true) what is seen is what is written, with no pinning.
func resolveRoleEdit(allowed map[int64]bool, all bool, submitted, current []int64) (final []int64, changed bool, err error) {
	var pinned []int64
	assignableCur := make([]int64, 0, len(current))
	for _, rid := range current {
		if all || allowed[rid] {
			assignableCur = append(assignableCur, rid)
		} else {
			pinned = append(pinned, rid)
		}
	}
	if sameInt64Set(submitted, assignableCur) {
		return nil, false, nil
	}
	final = append(append([]int64{}, submitted...), pinned...)
	if len(final) == 0 {
		return nil, false, ErrRoleRequired
	}
	return final, true, nil
}

// Edit is the single use case of "edit user": load the target once, check every changed field, and write in one
// transaction. It writes only after every check passes, so a submission either succeeds completely or changes
// nothing. If service orchestrated several biz calls that each stored their part, a rejection midway would leave
// the earlier fields in effect, a half-done change.
//
// The IP allowlist's self-lockout guard is checked separately: target loads the target in the default tier (root
// may edit itself), but a change to the IP allowlist is stopped even for root. The two checks have different tiers
// and cannot merge into one target call (see the comment on SelfOpTier); the decision itself is still checkSelfOp.
//
// Roles are the exception and split in two halves: a submitted set beyond the range is rejected here, and the final
// write set (which invisible roles to pin back, whether it is empty) is computed by repo.Edit after locking the
// user row; see RoleResolver.
func (uc *UserAdminUseCase) Edit(ctx context.Context, actor *Actor, id int64, in UserEdit) (*User, error) {
	cur, err := uc.target(ctx, actor, id, SelfOpDefault)
	if err != nil {
		return nil, err
	}

	var changes UserEditChanges
	if in.Nickname != cur.Nickname {
		nickname := in.Nickname
		changes.Nickname = &nickname
	}
	if in.Avatar != cur.Avatar {
		if err := validateAvatar(in.Avatar); err != nil {
			return nil, err
		}
		avatar := in.Avatar
		changes.Avatar = &avatar
	}
	if in.Email != cur.Email {
		email := in.Email
		changes.Email = &email
	}
	if in.Phone != cur.Phone {
		phone := in.Phone
		changes.Phone = &phone
	}
	if in.DeptID != cur.DeptID {
		if ok, err := uc.arbac.CanManageDept(ctx, actor, in.DeptID); err != nil {
			return nil, err
		} else if !ok {
			return nil, ErrDeptOutOfRange
		}
		deptID := in.DeptID
		changes.DeptID = &deptID
	}
	if changes.Roles, err = uc.planRoleEdit(ctx, actor, in.RoleIDs); err != nil {
		return nil, err
	}
	if in.IPWhitelist != cur.IPWhitelist {
		// Anti-lockout: a wrong network range would shut oneself out, so even root gets no way to lock itself out.
		if err := checkSelfOp(actor, id, SelfOpAntiLockout); err != nil {
			return nil, err
		}
		if err := ValidateIPWhitelist(in.IPWhitelist); err != nil {
			return nil, err
		}
		ipWhitelist := in.IPWhitelist
		changes.IPWhitelist = &ipWhitelist
	}

	if err := uc.repo.Edit(ctx, id, changes); err != nil {
		return nil, err
	}
	return uc.repo.FindByID(ctx, id)
}

// sameInt64Set reports whether two ID slices are the same set (ignoring order and duplicates).
func sameInt64Set(a, b []int64) bool {
	sa, sb := toSet(a), toSet(b)
	if len(sa) != len(sb) {
		return false
	}
	for k := range sa {
		if !sb[k] {
			return false
		}
	}
	return true
}

// Disable disables an account in the anti-lockout tier: even root cannot disable itself (so the only super
// administrator cannot lock itself out).
func (uc *UserAdminUseCase) Disable(ctx context.Context, actor *Actor, id int64) error {
	if _, err := uc.target(ctx, actor, id, SelfOpAntiLockout); err != nil {
		return err
	}
	// Old tokens stop working at once after disabling: the same statement as the status change revokes the
	// sessions. Enable does not revoke again and relies on this step: written in two steps, a failure here would
	// bring old sessions back to life after re-enabling.
	disabled := int8(0)
	return uc.repo.UpdateAccount(ctx, id, AccountChange{Status: &disabled, RevokeSessions: true})
}

// Enable lifts a disable and restores the ability to log in. It uses the default tier, not the anti-lockout tier:
// it is not one of the three anti-lockout exceptions, and "may you manage this person" and "anti-lockout" are two
// questions. That calling it on oneself is most likely dead code here (a disabled user cannot send requests; the
// principal middleware stops them first) is no reason to fold it into the anti-lockout tier.
//
// It does not bump token_version: disabling already did, so the user holds no valid token now; enabling only lets
// them log in again, with no old ticket to revoke.
func (uc *UserAdminUseCase) Enable(ctx context.Context, actor *Actor, id int64) error {
	if _, err := uc.target(ctx, actor, id, SelfOpDefault); err != nil {
		return err
	}
	enabled := int8(1)
	return uc.repo.UpdateAccount(ctx, id, AccountChange{Status: &enabled})
}

// SetRoot grants or revokes super administrator. It is root-only (RootOnly in the transport layer, step-up and
// audit) and in the anti-lockout tier: even root cannot change its own root status.
//
// Anti-lockout needs no count of roots: changing someone else requires being root, and one cannot demote oneself,
// so at least one root (the operator) always remains. After the change it bumps token_version to revoke all of
// the target's sessions at once: a grant takes effect on the next login, and a revocation takes root away
// immediately with no window.
func (uc *UserAdminUseCase) SetRoot(ctx context.Context, actor *Actor, id int64, isRoot bool) error {
	u, err := uc.target(ctx, actor, id, SelfOpAntiLockout)
	if err != nil {
		return err
	}
	if u.IsRoot == isRoot {
		return nil // idempotent: already in the target state, so no write and no bump (no needless kick-out)
	}
	return uc.repo.UpdateAccount(ctx, id, AccountChange{IsRoot: &isRoot, RevokeSessions: true})
}

// Get is the source of the edit form's initial values: it admits only users the actor can manage (the same range
// as the list, so non-root cannot read the details of root or of users out of range). It does not go through
// target: viewing one's own details is not limited by "do not edit oneself", only editing is (see
// ADR admin-0014, "In the interface").
func (uc *UserAdminUseCase) Get(ctx context.Context, actor *Actor, id int64) (*User, error) {
	if err := uc.arbac.EnsureTargetManageable(ctx, actor, id); err != nil {
		return nil, err
	}
	return uc.repo.FindByID(ctx, id)
}

// Kick forces a user offline: it changes no account state and only increments token_version so that issued tokens
// stop working.
func (uc *UserAdminUseCase) Kick(ctx context.Context, actor *Actor, id int64) error {
	if _, err := uc.target(ctx, actor, id, SelfOpDefault); err != nil {
		return err
	}
	return uc.repo.UpdateAccount(ctx, id, AccountChange{RevokeSessions: true})
}

// ResetPassword lets an administrator reset someone else's password (users change their own with
// ChangeMyPassword).
func (uc *UserAdminUseCase) ResetPassword(ctx context.Context, actor *Actor, id int64, newPassword string) error {
	if _, err := uc.target(ctx, actor, id, SelfOpDefault); err != nil {
		return err
	}
	hash, err := pwd.HashPassword(newPassword)
	if err != nil {
		return err
	}
	// Old tokens stop working at once after a reset: the same statement as the new password revokes the sessions.
	return uc.repo.UpdateAccount(ctx, id, AccountChange{PasswordHash: &hash, RevokeSessions: true})
}

// ResetOTP lets an administrator reset someone else's OTP binding, in the default tier (non-root cannot target
// itself, root can; a trade-off accepted on purpose, see ADR admin-0014,
// "A trade-off accepted on purpose"). The injected OTPResetter does the work; this method only decides permission.
func (uc *UserAdminUseCase) ResetOTP(ctx context.Context, actor *Actor, id int64) error {
	if _, err := uc.target(ctx, actor, id, SelfOpDefault); err != nil {
		return err
	}
	if err := uc.otp.Reset(ctx, id); err != nil {
		return err
	}
	// A reset usually means the authenticator was lost or stolen, so sessions signed in with it end too.
	return uc.repo.UpdateAccount(ctx, id, AccountChange{RevokeSessions: true})
}
