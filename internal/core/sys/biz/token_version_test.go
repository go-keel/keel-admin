package biz

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/go-keel/keel/pkg/pwd"
)

// fakeAdminRepo extends fakeUserRepo with the UserAdminRepo surface needed by
// UserAdminUseCase tests (List/Create/SetProfile/UpdateAccount/Edit).
// editCalls/lastEdit record how many times Edit was called and the last change set: assertions such as
// "which fields were really written" and "nothing was written when out of range" rely on them rather than
// on database side effects.
// Edit calls back changes.Roles the way the real repo does: when rolesInTx is non-nil, it is "the current
// roles read after locking the user row", which may differ from the roles seen before the transaction,
// simulating someone else changing the roles between the two reads. The callback's result is recorded in
// rolesWritten / writtenRoles.
type fakeAdminRepo struct {
	fakeUserRepo
	editCalls    int
	lastEdit     UserEditChanges
	rolesInTx    []int64
	rolesWritten bool
	writtenRoles []int64
	createCalls  int
	created      *User
}

var _ UserAdminRepo = (*fakeAdminRepo)(nil)

func (f *fakeAdminRepo) List(context.Context, []int64, UserFilter, int, int) ([]*User, int64, error) {
	return nil, 0, nil
}
func (f *fakeAdminRepo) Create(_ context.Context, u *User, _ []int64) error {
	f.createCalls++
	f.created = u
	return nil
}
func (f *fakeAdminRepo) RolesOf(context.Context, int64) ([]UserRole, error) { return nil, nil }
func (f *fakeAdminRepo) ForceOTPUsers(context.Context, []int64) (map[int64]bool, error) {
	return nil, nil
}
func (f *fakeAdminRepo) RolesOfUsers(context.Context, []int64) (map[int64][]UserRole, error) {
	return nil, nil
}
func (f *fakeAdminRepo) Edit(_ context.Context, _ int64, changes UserEditChanges) error {
	f.editCalls++
	f.lastEdit = changes
	if changes.Roles == nil {
		return nil
	}
	current := f.roles
	if f.rolesInTx != nil {
		current = f.rolesInTx
	}
	final, changed, err := changes.Roles(current)
	if err != nil {
		return err
	}
	f.rolesWritten, f.writtenRoles = changed, final
	return nil
}
func (f *fakeAdminRepo) SetProfile(context.Context, int64, ProfileEdit) error { return nil }
func (f *fakeAdminRepo) UpdateSettings(context.Context, int64, func(*UserSettings) error) (*UserSettings, error) {
	return &UserSettings{}, nil
}

func TestResolveRejectsStaleTokenVersion(t *testing.T) {
	repo := &fakeUserRepo{user: &User{ID: 7, Status: 1, TokenVersion: 2}}
	uc := NewAuthUseCase(repo, noopLoginLog{}, &fakePermCache{}, nil, nil, time.Hour, nil, AuthConfig{}, nil)
	if _, err := uc.ResolveWithVersion(context.Background(), 7, 1); err == nil {
		t.Fatal("ver=1 < token_version=2 should be rejected")
	}
	if _, err := uc.ResolveWithVersion(context.Background(), 7, 2); err != nil {
		t.Fatalf("a matching ver should pass: %v", err)
	}
}

// Non-root self-management is still blocked by the "fetch target" entry.
func TestKickRejectsSelfNonRoot(t *testing.T) {
	repo := &fakeAdminRepo{fakeUserRepo: fakeUserRepo{user: &User{ID: 1, Status: 1}}}
	arbac := NewARBACUseCase(nil, nil)
	uc := NewUserAdminUseCase(repo, arbac, nil)
	actor := &Actor{UserID: 1, IsRoot: false}
	if err := uc.Kick(context.Background(), actor, 1); !errors.Is(err, ErrSelfManageForbidden) {
		t.Fatalf("non-root kicking itself should be rejected: %v", err)
	}
}

// Root has no self restriction (except disabling itself): a self Kick is allowed and bumps.
func TestKickRootSelfAllowed(t *testing.T) {
	repo := &fakeAdminRepo{fakeUserRepo: fakeUserRepo{user: &User{ID: 1, Status: 1, IsRoot: true}}}
	arbac := NewARBACUseCase(nil, nil)
	uc := NewUserAdminUseCase(repo, arbac, nil)
	actor := &Actor{UserID: 1, IsRoot: true}
	if err := uc.Kick(context.Background(), actor, 1); err != nil {
		t.Fatalf("root kicking itself should be allowed: %v", err)
	}
	if len(repo.bumpCalls) != 1 || repo.bumpCalls[0] != 1 {
		t.Fatalf("root self Kick should bump token_version: %v", repo.bumpCalls)
	}
}

// SetRoot forbids acting on oneself (root included): nobody can change their own root status. Together with
// root-only, this guarantees at least one root (the operator) always remains, with no need to count roots
// to prevent a lockout.
func TestSetRootRejectsSelf(t *testing.T) {
	repo := &fakeAdminRepo{fakeUserRepo: fakeUserRepo{user: &User{ID: 1, Status: 1, IsRoot: true}}}
	uc := NewUserAdminUseCase(repo, NewARBACUseCase(nil, nil), nil)
	actor := &Actor{UserID: 1, IsRoot: true}
	if err := uc.SetRoot(context.Background(), actor, 1, false); !errors.Is(err, ErrSelfManageForbidden) {
		t.Fatalf("changing one's own root status should be rejected: %v", err)
	}
}

// Idempotent: already in the target state means no write and no bump (avoiding a pointless forced logout).
func TestSetRootIdempotentNoBump(t *testing.T) {
	repo := &fakeAdminRepo{fakeUserRepo: fakeUserRepo{user: &User{ID: 8, Status: 1, IsRoot: true}}}
	uc := NewUserAdminUseCase(repo, NewARBACUseCase(nil, nil), nil)
	actor := &Actor{UserID: 1, IsRoot: true}
	if err := uc.SetRoot(context.Background(), actor, 8, true); err != nil {
		t.Fatalf("the idempotent call should succeed: %v", err)
	}
	if len(repo.bumpCalls) != 0 {
		t.Fatalf("already in the target state must not bump: %v", repo.bumpCalls)
	}
}

// Disabling oneself is forbidden for everyone (root included), so the only super admin cannot lock themselves out.
func TestDisableRejectsRootSelf(t *testing.T) {
	repo := &fakeAdminRepo{fakeUserRepo: fakeUserRepo{user: &User{ID: 1, Status: 1, IsRoot: true}}}
	arbac := NewARBACUseCase(nil, nil)
	uc := NewUserAdminUseCase(repo, arbac, nil)
	actor := &Actor{UserID: 1, IsRoot: true}
	if err := uc.Disable(context.Background(), actor, 1); !errors.Is(err, ErrSelfManageForbidden) {
		t.Fatalf("root disabling itself should be rejected: %v", err)
	}
	if len(repo.bumpCalls) != 0 {
		t.Fatalf("a rejected self-disable must not bump: %v", repo.bumpCalls)
	}
}

// Revoking sessions is sent together with the same write (one statement): written in two steps, if the second
// failed the password would be changed while old sessions stayed alive; if revoking failed after disabling,
// old sessions would come back when the user is re-enabled. Enabling does not revoke (disabling already did).
func TestAccountWritesRevokeInSameCall(t *testing.T) {
	ctx := context.Background()
	root := &Actor{UserID: 1, IsRoot: true}
	cases := []struct {
		name   string
		run    func(uc *UserAdminUseCase) error
		check  func(c AccountChange) bool
		revoke bool
	}{
		{"reset password", func(uc *UserAdminUseCase) error { return uc.ResetPassword(ctx, root, 8, "new-pass") },
			func(c AccountChange) bool { return c.PasswordHash != nil && *c.PasswordHash != "" }, true},
		{"disable", func(uc *UserAdminUseCase) error { return uc.Disable(ctx, root, 8) },
			func(c AccountChange) bool { return c.Status != nil && *c.Status == 0 }, true},
		{"set super admin", func(uc *UserAdminUseCase) error { return uc.SetRoot(ctx, root, 8, true) },
			func(c AccountChange) bool { return c.IsRoot != nil && *c.IsRoot }, true},
		{"force logout", func(uc *UserAdminUseCase) error { return uc.Kick(ctx, root, 8) },
			func(c AccountChange) bool { return c.PasswordHash == nil && c.Status == nil && c.IsRoot == nil }, true},
		{"reset authenticator", func(uc *UserAdminUseCase) error { return uc.ResetOTP(ctx, root, 8) },
			func(c AccountChange) bool { return c.PasswordHash == nil && c.Status == nil && c.IsRoot == nil }, true},
		{"enable", func(uc *UserAdminUseCase) error { return uc.Enable(ctx, root, 8) },
			func(c AccountChange) bool { return c.Status != nil && *c.Status == 1 }, false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			repo := &fakeAdminRepo{fakeUserRepo: fakeUserRepo{user: &User{ID: 8, Status: 1, DeptID: 1}}}
			uc := NewUserAdminUseCase(repo, NewARBACUseCase(nil, nil), &fakeOTPResetter{})
			if err := tc.run(uc); err != nil {
				t.Fatal(err)
			}
			if len(repo.accountChanges) != 1 {
				t.Fatalf("want a single write, UpdateAccount was called %d times", len(repo.accountChanges))
			}
			c := repo.accountChanges[0]
			if !tc.check(c) || c.RevokeSessions != tc.revoke {
				t.Fatalf("change = %+v, revoke want %v", c, tc.revoke)
			}
			if tc.revoke && (len(repo.bumpCalls) != 1 || repo.bumpCalls[0] != 8) {
				t.Fatalf("user 8's sessions should be revoked: %v", repo.bumpCalls)
			}
		})
	}
}

func TestChangeMyPasswordRevokesInSameCall(t *testing.T) {
	hash, err := pwd.HashPassword("old-pass")
	if err != nil {
		t.Fatal(err)
	}
	repo := &fakeUserRepo{user: &User{ID: 7, Password: hash}}
	uc := NewAuthUseCase(repo, noopLoginLog{}, &fakePermCache{}, nil, nil, time.Hour, nil, AuthConfig{}, nil)
	if err := uc.ChangeMyPassword(context.Background(), 7, "old-pass", "new-pass"); err != nil {
		t.Fatal(err)
	}
	if len(repo.accountChanges) != 1 || repo.accountChanges[0].PasswordHash == nil || !repo.accountChanges[0].RevokeSessions ||
		len(repo.bumpCalls) != 1 || repo.bumpCalls[0] != 7 {
		t.Fatalf("the password change and session revocation should be one write: %+v", repo.accountChanges)
	}
}
