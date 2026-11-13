package biz

import (
	"context"
	"errors"
	"testing"
)

// fakeARBACRepoDept is an ARBACRepo fake with a configurable ManageDeptIDs
// return value, used to exercise EnsureTargetManageable's non-root branch
// (dept subtree/union range check) without a real repo/db.
type fakeARBACRepoDept struct {
	deptIDs    []int64
	assignable []int64
	protected  []int64
}

var _ ARBACRepo = (*fakeARBACRepoDept)(nil)

func (f *fakeARBACRepoDept) AssignableRoleIDs(context.Context, []int64) ([]int64, error) {
	return f.assignable, nil
}
func (f *fakeARBACRepoDept) ManageDeptIDs(context.Context, []int64) ([]int64, error) {
	return f.deptIDs, nil
}
func (f *fakeARBACRepoDept) SensitiveRoleIDs(_ context.Context, roleIDs []int64) ([]int64, error) {
	set := toSet(f.protected)
	var out []int64
	for _, r := range roleIDs {
		if set[r] {
			out = append(out, r)
		}
	}
	return out, nil
}
func (f *fakeARBACRepoDept) AssignableConfigs(context.Context, []int64) (map[int64][]int64, error) {
	return nil, nil
}
func (f *fakeARBACRepoDept) ManageDeptConfigs(context.Context, []int64) (map[int64][]int64, error) {
	return nil, nil
}

// resolveFor runs both halves of a role edit: planRoleEdit before the transaction, then the callback it returns,
// called with current as the roles read under the lock.
func resolveFor(uc *UserAdminUseCase, actor *Actor, current, submitted []int64) ([]int64, bool, error) {
	resolve, err := uc.planRoleEdit(context.Background(), actor, submitted)
	if err != nil {
		return nil, false, err
	}
	return resolve(current)
}

// TestResolveRoleEditPinsUnassignable: the target's existing roles include roles the actor may not assign
// (protected 7, and 9 outside assignable). When the submitted set omits them they must be pinned back, not
// removed by the full replacement just because "they are not visible in the dropdown" (that would strip
// them beyond the actor's authority); the submitted set itself is validated as usual.
// planRoleEdit + resolveRoleEdit are Edit's role branch and where the former AssignRole pin logic lives.
func TestResolveRoleEditPinsUnassignable(t *testing.T) {
	repo := &fakeAdminRepo{fakeUserRepo: fakeUserRepo{
		user:  &User{ID: 30, Status: 1, DeptID: 1},
		roles: []int64{5, 7, 9}, // existing: 5 assignable, 7 protected, 9 outside the whitelist
	}}
	arbac := NewARBACUseCase(&fakeARBACRepoDept{
		deptIDs:    []int64{1},
		assignable: []int64{5, 6, 7}, // 7 was mistakenly whitelisted; being sensitive, it still cannot be assigned
		protected:  []int64{7},
	}, repo)
	uc := NewUserAdminUseCase(repo, arbac, nil)
	actor := &Actor{UserID: 1, RoleIDs: []int64{2}}

	// Switch to [6]: 5 is removed (within authority, allowed), 7 and 9 are pinned back.
	final, changed, err := resolveFor(uc, actor, repo.roles, []int64{6})
	if err != nil {
		t.Fatalf("resolve: %v", err)
	}
	if !changed {
		t.Fatal("the submitted set differs from the current assignable part, should count as changed")
	}
	got := toSet(final)
	if len(final) != 3 || !got[6] || !got[7] || !got[9] {
		t.Fatalf("expected final {6,7,9}, got %v", final)
	}

	// Putting a protected role directly in the submitted set is rejected as usual.
	if _, _, err := resolveFor(uc, actor, repo.roles, []int64{7}); !errors.Is(err, ErrRoleOutOfRange) {
		t.Fatalf("expected ErrRoleOutOfRange for protected in submitted set, got %v", err)
	}

	// Root does a full replacement, what you see is what you get, nothing is pinned.
	root := &Actor{UserID: 2, IsRoot: true}
	final, changed, err = resolveFor(uc, root, repo.roles, []int64{6})
	if err != nil {
		t.Fatalf("root resolve: %v", err)
	}
	if !changed {
		t.Fatal("root: submitting a subset should count as changed")
	}
	if len(final) != 1 || final[0] != 6 {
		t.Fatalf("root: expected final {6}, got %v", final)
	}
}

// TestResolveRoleEditChanged: change detection compares only the "assignable part". The target's existing
// roles the actor may not assign (protected 7, 9 outside the whitelist) are always pinned and left out of the
// comparison; otherwise it would always report "changed".
func TestResolveRoleEditChanged(t *testing.T) {
	repo := &fakeAdminRepo{fakeUserRepo: fakeUserRepo{
		user:  &User{ID: 30, Status: 1, DeptID: 1},
		roles: []int64{5, 7, 9}, // 5 assignable, 7 protected, 9 outside the whitelist, so the assignable part = {5}
	}}
	arbac := NewARBACUseCase(&fakeARBACRepoDept{
		assignable: []int64{5, 6},
		protected:  []int64{7},
	}, repo)
	uc := NewUserAdminUseCase(repo, arbac, nil)
	actor := &Actor{UserID: 1, RoleIDs: []int64{2}}

	cases := []struct {
		name      string
		submitted []int64
		want      bool
	}{
		{"same-assignable-part", []int64{5}, false},
		{"order-insensitive", []int64{5}, false},
		{"replace", []int64{6}, true},
		{"clear", []int64{}, true},
		{"add", []int64{5, 6}, true},
	}
	for _, c := range cases {
		_, got, err := resolveFor(uc, actor, repo.roles, c.submitted)
		if err != nil {
			t.Fatalf("%s: %v", c.name, err)
		}
		if got != c.want {
			t.Fatalf("%s: changed=%v, want %v", c.name, got, c.want)
		}
	}

	// Root can assign everything, so nothing is pinned and the full set is compared: what you see is what you get.
	root := &Actor{UserID: 2, IsRoot: true}
	if _, changed, _ := resolveFor(uc, root, repo.roles, []int64{5, 7, 9}); changed {
		t.Fatal("root: submitting all current roles should count as unchanged")
	}
	if _, changed, _ := resolveFor(uc, root, repo.roles, []int64{5}); !changed {
		t.Fatal("root: submitting a subset should count as changed")
	}
}

// TestKickNonRootRejectsRootTarget: when a non-root actor tries to Kick a root target,
// EnsureTargetManageable must stop it with ErrTargetProtected before the dept check, and must never trigger
// a bump (the fake ARBACUseCase needs a non-nil repo/users; passing nil, nil would short-circuit the check
// and leave this path untested).
func TestKickNonRootRejectsRootTarget(t *testing.T) {
	repo := &fakeAdminRepo{fakeUserRepo: fakeUserRepo{user: &User{ID: 20, Status: 1, IsRoot: true, DeptID: 1}}}
	arbac := NewARBACUseCase(&fakeARBACRepoDept{deptIDs: []int64{1}}, repo)
	uc := NewUserAdminUseCase(repo, arbac, nil)
	actor := &Actor{UserID: 1, IsRoot: false, RoleIDs: []int64{2}}

	err := uc.Kick(context.Background(), actor, 20)
	if !errors.Is(err, ErrTargetProtected) {
		t.Fatalf("non-root kicking a root target should be rejected with ErrTargetProtected: %v", err)
	}
	if len(repo.bumpCalls) != 0 {
		t.Fatalf("the rejection path must not bump: %v", repo.bumpCalls)
	}
}

// TestKickNonRootRejectsDeptOutOfRange: the target is not root, but its dept is outside the union of the actor's
// manageable departments, so ErrDeptOutOfRange, and no bump.
func TestKickNonRootRejectsDeptOutOfRange(t *testing.T) {
	repo := &fakeAdminRepo{fakeUserRepo: fakeUserRepo{user: &User{ID: 21, Status: 1, DeptID: 99}}}
	arbac := NewARBACUseCase(&fakeARBACRepoDept{deptIDs: []int64{1, 2}}, repo)
	uc := NewUserAdminUseCase(repo, arbac, nil)
	actor := &Actor{UserID: 1, IsRoot: false, RoleIDs: []int64{2}}

	err := uc.Kick(context.Background(), actor, 21)
	if !errors.Is(err, ErrDeptOutOfRange) {
		t.Fatalf("non-root kicking an out-of-range target should get ErrDeptOutOfRange: %v", err)
	}
	if len(repo.bumpCalls) != 0 {
		t.Fatalf("the rejection path must not bump: %v", repo.bumpCalls)
	}
}

// TestCreateRequiresRole: a new account needs at least one role; an empty role set is rejected outright.
func TestCreateRequiresRole(t *testing.T) {
	repo := &fakeAdminRepo{}
	arbac := NewARBACUseCase(&fakeARBACRepoDept{}, repo)
	uc := NewUserAdminUseCase(repo, arbac, nil)
	root := &Actor{UserID: 1, IsRoot: true}
	if _, err := uc.Create(context.Background(), root, UserCreate{Username: "u", Password: "p", Nickname: "n"}); !errors.Is(err, ErrRoleRequired) {
		t.Fatalf("an empty role set should be rejected with ErrRoleRequired, got %v", err)
	}
}

// TestCreateIPWhitelist: the initial IP whitelist is written together with the account; if it is invalid the
// whole request is rejected and no row is written. Previously the account was created first and the whitelist
// set separately: with an invalid whitelist the account already existed while the call reported an error, and
// a retry hit "user already exists".
func TestCreateIPWhitelist(t *testing.T) {
	root := &Actor{UserID: 1, IsRoot: true}
	newUC := func() (*UserAdminUseCase, *fakeAdminRepo) {
		repo := &fakeAdminRepo{fakeUserRepo: fakeUserRepo{findByUsernameErr: ErrInvalidCredential}}
		return NewUserAdminUseCase(repo, NewARBACUseCase(&fakeARBACRepoDept{}, repo), nil), repo
	}

	uc, repo := newUC()
	if _, err := uc.Create(context.Background(), root, UserCreate{Username: "u", Password: "p", RoleIDs: []int64{2}, IPWhitelist: "not-an-ip"}); err == nil {
		t.Fatal("an invalid whitelist should be rejected")
	}
	if repo.createCalls != 0 {
		t.Fatalf("an invalid whitelist must not write, repo.Create was called %d times", repo.createCalls)
	}

	uc, repo = newUC()
	if _, err := uc.Create(context.Background(), root, UserCreate{Username: "u", Password: "p", RoleIDs: []int64{2}, IPWhitelist: "10.0.0.0/8"}); err != nil {
		t.Fatalf("Create: %v", err)
	}
	if repo.createCalls != 1 || repo.created.IPWhitelist != "10.0.0.0/8" {
		t.Fatalf("the whitelist should be written with the account: calls=%d created=%+v", repo.createCalls, repo.created)
	}
}

// TestCreateUsernameLookup: looking up the username before creating only serves to reject early: insert only
// when it is not found (ErrInvalidCredential), reject with ErrUserExists when found, and return a failing
// lookup's error unchanged instead of treating it as "does not exist" and inserting.
func TestCreateUsernameLookup(t *testing.T) {
	lookupErr := errors.New("connection lost")
	cases := []struct {
		name       string
		user       *User
		findErr    error
		wantErr    error
		wantCreate int
	}{
		{name: "insert only when absent", findErr: ErrInvalidCredential, wantCreate: 1},
		{name: "already exists", user: &User{ID: 9, Username: "u"}, wantErr: ErrUserExists},
		{name: "lookup fails", findErr: lookupErr, wantErr: lookupErr},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			repo := &fakeAdminRepo{fakeUserRepo: fakeUserRepo{user: tc.user, findByUsernameErr: tc.findErr}}
			uc := NewUserAdminUseCase(repo, NewARBACUseCase(&fakeARBACRepoDept{}, repo), nil)
			root := &Actor{UserID: 1, IsRoot: true}
			_, err := uc.Create(context.Background(), root, UserCreate{Username: "u", Password: "p", RoleIDs: []int64{2}})
			if (tc.wantErr == nil && err != nil) || (tc.wantErr != nil && !errors.Is(err, tc.wantErr)) {
				t.Fatalf("Create = %v, want %v", err, tc.wantErr)
			}
			if repo.createCalls != tc.wantCreate {
				t.Fatalf("repo.Create was called %d times, want %d", repo.createCalls, tc.wantCreate)
			}
		})
	}
}

// TestKickNonRootAllowsInRangeTarget: the positive control: the target is not root and its dept is inside the
// union of the actor's manageable range, so the non-root path really lets it through (rather than rejecting
// everything), with exactly one bump.
func TestKickNonRootAllowsInRangeTarget(t *testing.T) {
	repo := &fakeAdminRepo{fakeUserRepo: fakeUserRepo{user: &User{ID: 22, Status: 1, DeptID: 2}}}
	arbac := NewARBACUseCase(&fakeARBACRepoDept{deptIDs: []int64{1, 2}}, repo)
	uc := NewUserAdminUseCase(repo, arbac, nil)
	actor := &Actor{UserID: 1, IsRoot: false, RoleIDs: []int64{2}}

	if err := uc.Kick(context.Background(), actor, 22); err != nil {
		t.Fatalf("non-root kicking a target inside the manageable range should succeed: %v", err)
	}
	if len(repo.bumpCalls) != 1 || repo.bumpCalls[0] != 22 {
		t.Fatalf("want exactly one bump: %v", repo.bumpCalls)
	}
}
