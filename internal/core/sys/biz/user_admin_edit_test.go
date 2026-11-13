package biz

import (
	"context"
	"errors"
	"testing"
)

// baseEditUser is the fixed current state of the target in Edit tests: department 1 (inside the manageable
// range of the fixture below), no roles, no IP whitelist. Each case starts from it and changes one field, so
// it can assert "only this one changed".
func baseEditUser() *User {
	return &User{ID: 30, Status: 1, DeptID: 1, Nickname: "old", Avatar: "", IPWhitelist: ""}
}

// baseEdit copies u's current values into a UserEdit unchanged: a "baseline submission" that changes nothing.
// A test changes one thing on top of it, and every other field naturally equals the current value (so the
// matching validation branches do not fire).
func baseEdit(u *User) UserEdit {
	return UserEdit{Nickname: u.Nickname, Avatar: u.Avatar, DeptID: u.DeptID, RoleIDs: nil, IPWhitelist: u.IPWhitelist}
}

func newEditUC(u *User, roles []int64, arbacRepo *fakeARBACRepoDept) (*fakeAdminRepo, *UserAdminUseCase) {
	repo := &fakeAdminRepo{fakeUserRepo: fakeUserRepo{user: u, roles: roles}}
	uc := NewUserAdminUseCase(repo, NewARBACUseCase(arbacRepo, repo), nil)
	return repo, uc
}

// TestEditRejectsOutOfRangeWithoutWriting covers "write only after every check passes": if any one check fails
// (department out of range, role out of range, malformed IP whitelist), repo.Edit must not be called. Leaving
// no half-applied change is the core problem this refactor solves.
func TestEditRejectsOutOfRangeWithoutWriting(t *testing.T) {
	actor := &Actor{UserID: 1, RoleIDs: []int64{2}}
	cases := []struct {
		name    string
		roles   []int64
		arbac   *fakeARBACRepoDept
		mutate  func(e *UserEdit)
		wantErr error
	}{
		{
			name:    "department out of range",
			arbac:   &fakeARBACRepoDept{deptIDs: []int64{1}},
			mutate:  func(e *UserEdit) { e.DeptID = 99 },
			wantErr: ErrDeptOutOfRange,
		},
		{
			name:    "role out of range",
			arbac:   &fakeARBACRepoDept{deptIDs: []int64{1}, assignable: []int64{5}},
			mutate:  func(e *UserEdit) { e.RoleIDs = []int64{7} },
			wantErr: ErrRoleOutOfRange,
		},
		{
			name:    "malformed IP whitelist",
			arbac:   &fakeARBACRepoDept{deptIDs: []int64{1}},
			mutate:  func(e *UserEdit) { e.IPWhitelist = "not-an-ip" },
			wantErr: ErrInvalidArgument,
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			u := baseEditUser()
			repo, uc := newEditUC(u, nil, tc.arbac)
			in := baseEdit(u)
			tc.mutate(&in)
			if _, err := uc.Edit(context.Background(), actor, u.ID, in); !errors.Is(err, tc.wantErr) {
				t.Fatalf("Edit() error = %v, want %v", err, tc.wantErr)
			}
			if repo.editCalls != 0 {
				t.Fatalf("a failed check must write nothing, repo.Edit was called %d times", repo.editCalls)
			}
		})
	}
}

// TestEditWritesOnlyChangedField: only the nickname changes, so the change set carries only the nickname and
// every other field (RolesChanged included) stays zero, keeping the "save only what really changed" semantics.
func TestEditWritesOnlyChangedField(t *testing.T) {
	u := baseEditUser()
	repo, uc := newEditUC(u, nil, &fakeARBACRepoDept{deptIDs: []int64{1}})
	actor := &Actor{UserID: 1, RoleIDs: []int64{2}}

	in := baseEdit(u)
	in.Nickname = "new-nickname"
	if _, err := uc.Edit(context.Background(), actor, u.ID, in); err != nil {
		t.Fatalf("Edit: %v", err)
	}
	if repo.editCalls != 1 {
		t.Fatalf("want exactly one write, got %d", repo.editCalls)
	}
	c := repo.lastEdit
	if c.Nickname == nil || *c.Nickname != "new-nickname" {
		t.Fatalf("the nickname should be written, got %+v", c)
	}
	if c.Avatar != nil || c.DeptID != nil || c.IPWhitelist != nil || repo.rolesWritten {
		t.Fatalf("only the nickname changed, no other field may appear in the change set: %+v", c)
	}
}

// TestEditSkipsRoleWriteWhenUnchanged: when the submitted set equals the current roles (compared after pinning),
// the change set carries no role write, even if another field in the same submission did change.
func TestEditSkipsRoleWriteWhenUnchanged(t *testing.T) {
	u := baseEditUser()
	repo, uc := newEditUC(u, []int64{5}, &fakeARBACRepoDept{deptIDs: []int64{1}, assignable: []int64{5}})
	actor := &Actor{UserID: 1, RoleIDs: []int64{2}}

	in := baseEdit(u)
	in.RoleIDs = []int64{5} // same as current
	in.Nickname = "changed" // makes Edit really write once, isolating the role branch
	if _, err := uc.Edit(context.Background(), actor, u.ID, in); err != nil {
		t.Fatalf("Edit: %v", err)
	}
	if repo.rolesWritten {
		t.Fatalf("unchanged roles must not produce a role write: %+v", repo.lastEdit)
	}
}

// TestEditRootSelfIPWhitelistRejectedWithoutWriting: when root edits itself, changing the IP whitelist is
// rejected on its own (to avoid locking itself out, see the comment on SelfOpAntiLockout) and no field is
// written, even though the default level of the target step itself lets root edit itself.
func TestEditRootSelfIPWhitelistRejectedWithoutWriting(t *testing.T) {
	u := &User{ID: 1, Status: 1, IsRoot: true, DeptID: 1, Nickname: "root"}
	repo, uc := newEditUC(u, nil, &fakeARBACRepoDept{})
	actor := &Actor{UserID: 1, IsRoot: true}

	in := baseEdit(u)
	in.IPWhitelist = "10.0.0.0/8"
	if _, err := uc.Edit(context.Background(), actor, u.ID, in); !errors.Is(err, ErrSelfManageForbidden) {
		t.Fatalf("root changing its own IP whitelist should be rejected: %v", err)
	}
	if repo.editCalls != 0 {
		t.Fatalf("the rejection path must not write: %d", repo.editCalls)
	}
}

// TestEditPinsRoleThatAppearsInsideTransaction: the roles seen before the transaction are {5}, and after
// locking the user row they read {5, 9}: between the two reads someone else added 9, which is outside this
// actor's assignable range. After submitting {6} the written set must still carry 9: which roles are pinned
// is computed from what is read under the lock; computing it from the snapshot outside the transaction would
// drop 9.
func TestEditPinsRoleThatAppearsInsideTransaction(t *testing.T) {
	u := baseEditUser()
	repo, uc := newEditUC(u, []int64{5}, &fakeARBACRepoDept{deptIDs: []int64{1}, assignable: []int64{5, 6}})
	repo.rolesInTx = []int64{5, 9}
	actor := &Actor{UserID: 1, RoleIDs: []int64{2}}

	in := baseEdit(u)
	in.Nickname = "changed"
	in.RoleIDs = []int64{6}
	if _, err := uc.Edit(context.Background(), actor, u.ID, in); err != nil {
		t.Fatalf("Edit: %v", err)
	}
	got := toSet(repo.writtenRoles)
	if !repo.rolesWritten || len(repo.writtenRoles) != 2 || !got[6] || !got[9] {
		t.Fatalf("written set should be {6, 9}, got written=%v roles=%v", repo.rolesWritten, repo.writtenRoles)
	}
}

// TestEditRoleRequiredJudgedInsideTransaction: an account needs at least one role, and this too is judged on the
// current roles read under the lock: when the submitted set is cleared, whether any role can be pinned decides
// the outcome.
func TestEditRoleRequiredJudgedInsideTransaction(t *testing.T) {
	arbac := &fakeARBACRepoDept{deptIDs: []int64{1}, assignable: []int64{5}}
	actor := &Actor{UserID: 1, RoleIDs: []int64{2}}

	u := baseEditUser()
	repo, uc := newEditUC(u, []int64{5}, arbac)
	in := baseEdit(u)
	in.RoleIDs = []int64{}
	if _, err := uc.Edit(context.Background(), actor, u.ID, in); !errors.Is(err, ErrRoleRequired) {
		t.Fatalf("clearing to zero roles should be rejected with ErrRoleRequired, got %v", err)
	}
	if repo.rolesWritten {
		t.Fatal("roles must not be written on rejection")
	}

	// Cleared the same way, but under the lock an out-of-range role 9 that someone just added is read: it is pinned,
	// so the written set {9} is not empty.
	u = baseEditUser()
	repo, uc = newEditUC(u, []int64{5}, arbac)
	repo.rolesInTx = []int64{5, 9}
	if _, err := uc.Edit(context.Background(), actor, u.ID, in); err != nil {
		t.Fatalf("Edit: %v", err)
	}
	if !repo.rolesWritten || len(repo.writtenRoles) != 1 || repo.writtenRoles[0] != 9 {
		t.Fatalf("written set should be {9}, got written=%v roles=%v", repo.rolesWritten, repo.writtenRoles)
	}
}
