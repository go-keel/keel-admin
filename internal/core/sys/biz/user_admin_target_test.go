package biz

import (
	"context"
	"errors"
	"testing"
)

// fakeOTPResetter records the reset userIDs for the ResetOTP case to assert on, without setting up a real
// cipher or replay guard; that is exactly why OTPResetter takes only one method.
type fakeOTPResetter struct {
	resetCalls []int64
}

func (f *fakeOTPResetter) Reset(_ context.Context, userID int64) error {
	f.resetCalls = append(f.resetCalls, userID)
	return nil
}

// TestSelfManagementMatrix runs every action through three scenarios: "non-root on itself", "root on itself"
// and "target not manageable". All management actions fetch their target from the same "fetch target"
// entry; add a new action to this table together with its tier, so neither the self rules nor the ARBAC
// boundary is missed (see ADR admin-0014). The tier is written in
// the table: a glance at name+tier shows which tier an action belongs to, and the expected result follows
// from the tier instead of a separate assertion per row.
func TestSelfManagementMatrix(t *testing.T) {
	cases := []struct {
		name string
		tier SelfOpTier
		call func(uc *UserAdminUseCase, actor *Actor, id int64) error
	}{
		{"Disable", SelfOpAntiLockout, func(uc *UserAdminUseCase, actor *Actor, id int64) error {
			return uc.Disable(context.Background(), actor, id)
		}},
		{"Enable", SelfOpDefault, func(uc *UserAdminUseCase, actor *Actor, id int64) error {
			return uc.Enable(context.Background(), actor, id)
		}},
		{"Kick", SelfOpDefault, func(uc *UserAdminUseCase, actor *Actor, id int64) error {
			return uc.Kick(context.Background(), actor, id)
		}},
		{"ResetPassword", SelfOpDefault, func(uc *UserAdminUseCase, actor *Actor, id int64) error {
			return uc.ResetPassword(context.Background(), actor, id, "new-pass")
		}},
		{"SetRoot", SelfOpAntiLockout, func(uc *UserAdminUseCase, actor *Actor, id int64) error {
			return uc.SetRoot(context.Background(), actor, id, true)
		}},
		{"ResetOTP", SelfOpDefault, func(uc *UserAdminUseCase, actor *Actor, id int64) error {
			return uc.ResetOTP(context.Background(), actor, id)
		}},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Run("non-root on itself", func(t *testing.T) {
				repo := &fakeAdminRepo{fakeUserRepo: fakeUserRepo{user: &User{ID: 1, Status: 1, DeptID: 1}}}
				arbac := NewARBACUseCase(&fakeARBACRepoDept{deptIDs: []int64{1}}, repo)
				uc := NewUserAdminUseCase(repo, arbac, &fakeOTPResetter{})
				actor := &Actor{UserID: 1, IsRoot: false}
				if err := tc.call(uc, actor, 1); !errors.Is(err, ErrSelfManageForbidden) {
					t.Fatalf("non-root on itself should be rejected with ErrSelfManageForbidden, got %v", err)
				}
			})

			t.Run("root on itself", func(t *testing.T) {
				repo := &fakeAdminRepo{fakeUserRepo: fakeUserRepo{user: &User{ID: 2, Status: 1, IsRoot: true, DeptID: 1}}}
				arbac := NewARBACUseCase(&fakeARBACRepoDept{}, repo)
				uc := NewUserAdminUseCase(repo, arbac, &fakeOTPResetter{})
				actor := &Actor{UserID: 2, IsRoot: true}
				err := tc.call(uc, actor, 2)
				if tc.tier == SelfOpAntiLockout {
					if !errors.Is(err, ErrSelfManageForbidden) {
						t.Fatalf("anti-lockout tier: root on itself should get ErrSelfManageForbidden, got %v", err)
					}
					return
				}
				if err != nil {
					t.Fatalf("default tier: root on itself should pass, got %v", err)
				}
			})

			t.Run("target not manageable", func(t *testing.T) {
				repo := &fakeAdminRepo{fakeUserRepo: fakeUserRepo{user: &User{ID: 30, Status: 1, DeptID: 99}}}
				arbac := NewARBACUseCase(&fakeARBACRepoDept{deptIDs: []int64{1}}, repo)
				uc := NewUserAdminUseCase(repo, arbac, &fakeOTPResetter{})
				actor := &Actor{UserID: 999, IsRoot: false, RoleIDs: []int64{2}}
				if err := tc.call(uc, actor, 30); !errors.Is(err, ErrDeptOutOfRange) {
					t.Fatalf("an unmanageable target should be rejected with ErrDeptOutOfRange, got %v", err)
				}
			})
		})
	}
}

// TestResetOTPCallsResetter: once the default tier check passes, the work is delegated to the injected
// OTPResetter; UserAdminUseCase itself does not touch the OTP storage details.
func TestResetOTPCallsResetter(t *testing.T) {
	repo := &fakeAdminRepo{fakeUserRepo: fakeUserRepo{user: &User{ID: 5, Status: 1, DeptID: 1}}}
	arbac := NewARBACUseCase(&fakeARBACRepoDept{deptIDs: []int64{1}}, repo)
	otp := &fakeOTPResetter{}
	uc := NewUserAdminUseCase(repo, arbac, otp)
	actor := &Actor{UserID: 1, IsRoot: false, RoleIDs: []int64{2}}

	if err := uc.ResetOTP(context.Background(), actor, 5); err != nil {
		t.Fatalf("ResetOTP: %v", err)
	}
	if len(otp.resetCalls) != 1 || otp.resetCalls[0] != 5 {
		t.Fatalf("should delegate to OTPResetter.Reset(5), got %v", otp.resetCalls)
	}
}
