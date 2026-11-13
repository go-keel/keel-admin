package biz

import "context"

// ARBACRepo loads a manager's assignable-role and manageable-dept boundaries,
// and role protection flags.
//
// This port is read-only: the assignable-role and manageable-department configuration is written by RoleRepo.Save
// in the same transaction as the rest of the role's configuration, so there is no separate write path here.
type ARBACRepo interface {
	// AssignableRoleIDs returns the union of roles the given roles may assign.
	// Both ends count only enabled, undeleted roles: a disabled granting role grants no assignment right, and a
	// disabled target role cannot be assigned.
	AssignableRoleIDs(ctx context.Context, roleIDs []int64) ([]int64, error)
	// ManageDeptIDs returns the union of dept subtrees the given roles may
	// manage. Every configured department always expands to its whole subtree (managing a department means
	// managing its whole line).
	ManageDeptIDs(ctx context.Context, roleIDs []int64) ([]int64, error)
	// SensitiveRoleIDs returns which of the given role ids are sensitive
	// (non-root can never hand them out).
	SensitiveRoleIDs(ctx context.Context, roleIDs []int64) ([]int64, error)
	// AssignableConfigs and ManageDeptConfigs read back the raw configuration in batch (roleID -> configured IDs,
	// subtrees not expanded) for root's role configuration interface to show.
	//
	// The difference from ManageDeptIDs above is display versus decision: these return "which departments this
	// role configured explicitly" as is, while ManageDeptIDs expands each into the subtree ID set used for
	// decisions. Mixing them up would write the expanded closure back as explicit configuration.
	AssignableConfigs(ctx context.Context, roleIDs []int64) (map[int64][]int64, error)
	ManageDeptConfigs(ctx context.Context, roleIDs []int64) (map[int64][]int64, error)
}

// ARBACUseCase enforces dimension-three management boundaries with anti-escalation.
type ARBACUseCase struct {
	repo  ARBACRepo
	users UserRepo
}

// NewARBACUseCase builds the ARBAC checker.
func NewARBACUseCase(repo ARBACRepo, users UserRepo) *ARBACUseCase {
	return &ARBACUseCase{repo: repo, users: users}
}

// CanManageDept reports whether the actor may manage the dept (root bypasses).
// ManageableDeptIDs returns the set of departments the actor can manage; all=true means unlimited (root).
func (uc *ARBACUseCase) ManageableDeptIDs(ctx context.Context, actor *Actor) (ids []int64, all bool, err error) {
	if actor.IsRoot {
		return nil, true, nil
	}
	ids, err = uc.repo.ManageDeptIDs(ctx, actor.RoleIDs)
	return ids, false, err
}

func (uc *ARBACUseCase) CanManageDept(ctx context.Context, actor *Actor, deptID int64) (bool, error) {
	if actor.IsRoot {
		return true, nil
	}
	ids, err := uc.repo.ManageDeptIDs(ctx, actor.RoleIDs)
	if err != nil {
		return false, err
	}
	return contains(ids, deptID), nil
}

// AssignableRange returns the set of roles the actor can assign to others; all=true means unlimited (root). The
// definition is assignableRange.
func (uc *ARBACUseCase) AssignableRange(ctx context.Context, actor *Actor) (allowed map[int64]bool, all bool, err error) {
	return assignableRange(ctx, uc.repo, actor)
}

// assignableRange is the one definition of "what can be assigned": unlimited for root; for non-root, the union of
// the held roles' assignable configuration ∩ enabled ∖ sensitive (the repo query enforces enabled; sensitive is
// removed here, because non-root can never assign it).
//
// Everything follows it: checking the submitted set when creating or editing an account (CanAssignRoles), deciding
// which existing roles stay pinned when editing a user, the assignment dropdown (RoleUseCase.ListAssignable) and
// viewing the permissions a role grants (RoleUseCase.PermissionsOf). A disabled role is therefore out of range: it
// is not in the dropdown, submitting it is rejected, and editing a user keeps it pinned as is.
func assignableRange(ctx context.Context, repo ARBACRepo, actor *Actor) (map[int64]bool, bool, error) {
	if actor.IsRoot {
		return nil, true, nil
	}
	assignable, err := repo.AssignableRoleIDs(ctx, actor.RoleIDs)
	if err != nil {
		return nil, false, err
	}
	sensitive, err := repo.SensitiveRoleIDs(ctx, assignable)
	if err != nil {
		return nil, false, err
	}
	allowed := toSet(assignable)
	for _, r := range sensitive {
		delete(allowed, r)
	}
	return allowed, false, nil
}

// CanAssignRoles reports whether the actor may assign all target roles.
// Sensitive roles are never assignable by non-root.
func (uc *ARBACUseCase) CanAssignRoles(ctx context.Context, actor *Actor, targetRoleIDs []int64) (bool, error) {
	if actor.IsRoot || len(targetRoleIDs) == 0 {
		return true, nil
	}
	allowed, _, err := uc.AssignableRange(ctx, actor)
	if err != nil {
		return false, err
	}
	for _, r := range targetRoleIDs {
		if !allowed[r] {
			return false, nil
		}
	}
	return true, nil
}

// EnsureTargetManageable blocks managing root/protected accounts by non-root,
// or accounts outside the actor's manageable dept range. root is not limited.
func (uc *ARBACUseCase) EnsureTargetManageable(ctx context.Context, actor *Actor, targetUserID int64) error {
	if actor.IsRoot {
		return nil
	}
	u, err := uc.users.FindByID(ctx, targetUserID)
	if err != nil {
		return err
	}
	return uc.ensureManageable(ctx, actor, u)
}

// ensureManageable makes the same decision as EnsureTargetManageable but takes a user already loaded instead of
// looking it up by ID again: UserAdminUseCase.target has already called FindByID, so flows that load the detail
// before deciding, such as editing a user, do not query the database twice for the same target.
func (uc *ARBACUseCase) ensureManageable(ctx context.Context, actor *Actor, u *User) error {
	if actor.IsRoot {
		return nil
	}
	if u.IsRoot {
		return ErrTargetProtected
	}
	ok, err := uc.CanManageDept(ctx, actor, u.DeptID)
	if err != nil {
		return err
	}
	if !ok {
		return ErrDeptOutOfRange
	}
	return nil
}

// Actor is the management subject (from principal).
type Actor struct {
	UserID  int64
	IsRoot  bool
	RoleIDs []int64
}

func contains(s []int64, v int64) bool {
	for _, x := range s {
		if x == v {
			return true
		}
	}
	return false
}

func toSet(s []int64) map[int64]bool {
	m := make(map[int64]bool, len(s))
	for _, x := range s {
		m[x] = true
	}
	return m
}
