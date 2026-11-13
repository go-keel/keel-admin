package biz

import (
	"context"
	"errors"
	"testing"
)

// fakeOTPGate controls three states: whether the role forces OTP, whether it is bound, whether verification
// passes; verifyErr simulates Verify failing (for example, uses exhausted), and verifyCalls counts how many
// times Verify was called.
type fakeOTPGate struct {
	required    bool
	bound       bool
	valid       bool
	verifyErr   error
	verifyCalls int
}

var _ OTPGate = (*fakeOTPGate)(nil)

func (g *fakeOTPGate) Required(context.Context, int64) (bool, error) { return g.required, nil }
func (g *fakeOTPGate) Bound(context.Context, int64) (bool, error)    { return g.bound, nil }
func (g *fakeOTPGate) Verify(context.Context, int64, string) (bool, error) {
	g.verifyCalls++
	return g.valid, g.verifyErr
}

// admissionLogin runs one login and returns the result and the lockout counter's RecordFailure calls.
func admissionLogin(t *testing.T, gate *fakeOTPGate, otpCode string) (
	token, bindToken string, guard *fakeLockoutGuard, err error,
) {
	t.Helper()
	repo := loginTestUser(t, "u", "pw")
	guard = &fakeLockoutGuard{}
	uc := newLoginUseCase(repo, noopLoginLog{}, guard,
		AuthConfig{LockoutEnabled: true, LockoutMaxFailures: 5, LockoutLockMinutes: 15})
	uc.otp = gate

	token, bindToken, _, err = uc.Login(context.Background(), "u", "pw", otpCode, "", "", "1.2.3.4")
	return token, bindToken, guard, err
}

// Bound but not forced by a role: the profile page lets everyone bind OTP themselves; if OTP were checked only
// when forced, a self-bound OTP would be ignored entirely at login, a binding for nothing. Bound means checked.
func TestLoginBoundWithoutForceRoleStillRequiresOTP(t *testing.T) {
	_, _, _, err := admissionLogin(t, &fakeOTPGate{required: false, bound: true}, "")

	if !errors.Is(err, ErrLoginRejected) {
		t.Fatalf("an account with OTP bound must verify OTP even when no role forces it, got err=%v", err)
	}
}

func TestLoginBoundWithoutForceRoleAcceptsValidOTP(t *testing.T) {
	token, _, _, err := admissionLogin(t,
		&fakeOTPGate{required: false, bound: true, valid: true}, "123456")

	if err != nil {
		t.Fatalf("Login: unexpected error: %v", err)
	}
	if token == "" {
		t.Fatal("a correct OTP should log in")
	}
}

// A wrong OTP must count toward the lockout just like a wrong password; otherwise whoever has the password can
// brute-force the 6-digit code forever.
func TestLoginBadOTPRecordsLockoutFailure(t *testing.T) {
	_, _, guard, err := admissionLogin(t,
		&fakeOTPGate{required: true, bound: true, valid: false}, "000000")

	if !errors.Is(err, ErrLoginRejected) {
		t.Fatalf("want ErrLoginRejected, got %v", err)
	}
	if len(guard.recordCalls) != 1 {
		t.Fatalf("a wrong OTP must count toward the lockout (6-digit brute force), RecordFailure calls=%d",
			len(guard.recordCalls))
	}
}

// The one-time code comes before the password: when the code is wrong the real password hash is never touched,
// only one dummy check runs (constant time). Otherwise someone without the code could still use this entry
// to try passwords one by one.
func TestLoginOTPCheckedBeforePassword(t *testing.T) {
	repo := loginTestUser(t, "u", "pw")
	var hashes []string
	orig := passwordVerify
	passwordVerify = func(p, h string) (bool, error) {
		hashes = append(hashes, h)
		return orig(p, h)
	}
	t.Cleanup(func() { passwordVerify = orig })
	uc := newLoginUseCase(repo, noopLoginLog{}, &fakeLockoutGuard{}, AuthConfig{})
	uc.otp = &fakeOTPGate{bound: true, valid: false}

	_, _, _, err := uc.Login(context.Background(), "u", "pw", "000000", "", "", "1.2.3.4")
	if !errors.Is(err, ErrLoginRejected) {
		t.Fatalf("a wrong code should be rejected, got %v", err)
	}
	if len(hashes) != 1 || hashes[0] != dummyHash {
		t.Fatalf("a wrong code should run one dummy check and not touch the real password hash, verified=%d", len(hashes))
	}
}

// Bound but no code entered: the same error as the other rejections, with no "please enter your one-time
// code" hint, which would tell someone without the password that this account has an authenticator bound.
func TestLoginBoundMissingOTPIsIndistinguishable(t *testing.T) {
	_, _, guard, err := admissionLogin(t, &fakeOTPGate{bound: true}, "")
	if !errors.Is(err, ErrLoginRejected) {
		t.Fatalf("a missing code should give ErrLoginRejected, got %v", err)
	}
	if len(guard.recordCalls) != 0 {
		t.Fatalf("a missing code is not a wrong guess and must not count, RecordFailure=%d", len(guard.recordCalls))
	}
}

// Correct code but uses exhausted: likewise not revealed (revealing it would confirm "the code is right"), and
// not counted toward the lockout.
func TestLoginOTPUsedUpIsIndistinguishable(t *testing.T) {
	_, _, guard, err := admissionLogin(t, &fakeOTPGate{bound: true, verifyErr: ErrOTPUsed}, "123456")
	if !errors.Is(err, ErrLoginRejected) {
		t.Fatalf("exhausted uses should give ErrLoginRejected, got %v", err)
	}
	if len(guard.recordCalls) != 0 {
		t.Fatalf("the code is correct and must not count toward the lockout, RecordFailure=%d", len(guard.recordCalls))
	}
}

// Correct code, wrong password: rejected as usual and counted as one failure.
func TestLoginValidOTPWrongPasswordRejected(t *testing.T) {
	repo := loginTestUser(t, "u", "pw")
	guard := &fakeLockoutGuard{}
	uc := newLoginUseCase(repo, noopLoginLog{}, guard,
		AuthConfig{LockoutEnabled: true, LockoutMaxFailures: 5, LockoutLockMinutes: 15})
	uc.otp = &fakeOTPGate{bound: true, valid: true}

	_, _, _, err := uc.Login(context.Background(), "u", "wrong", "123456", "", "", "1.2.3.4")
	if !errors.Is(err, ErrLoginRejected) {
		t.Fatalf("a wrong password should be rejected, got %v", err)
	}
	if len(guard.recordCalls) != 1 {
		t.Fatalf("a wrong password should count one failure, RecordFailure=%d", len(guard.recordCalls))
	}
}

// The IP whitelist comes before the one-time code and is the only rejection reported explicitly: a request from
// the wrong source does not use up the code's uses.
func TestLoginIPWhitelistCheckedBeforeOTP(t *testing.T) {
	repo := loginTestUser(t, "u", "pw")
	repo.user.IPWhitelist = "10.0.0.0/8"
	gate := &fakeOTPGate{bound: true, valid: true}
	uc := newLoginUseCase(repo, noopLoginLog{}, &fakeLockoutGuard{}, AuthConfig{})
	uc.otp = gate

	_, _, _, err := uc.Login(context.Background(), "u", "wrong", "123456", "", "", "66.66.66.66")
	if !errors.Is(err, ErrIPNotAllowed) {
		t.Fatalf("outside the whitelist should give ErrIPNotAllowed (even with a wrong password), got %v", err)
	}
	if gate.verifyCalls != 0 {
		t.Fatalf("outside the whitelist the one-time code must not be verified, Verify=%d", gate.verifyCalls)
	}
}

// Forced but not bound: there is no code to submit, so the binding-guide token is issued only after the password
// passes; a wrong password is rejected as usual.
func TestLoginForcedUnboundNeedsPasswordForBindToken(t *testing.T) {
	_, bindToken, _, err := admissionLogin(t, &fakeOTPGate{required: true}, "")
	if err != nil || bindToken == "" {
		t.Fatalf("a correct password should get the binding-guide token, err=%v", err)
	}

	repo := loginTestUser(t, "u", "pw")
	uc := newLoginUseCase(repo, noopLoginLog{}, &fakeLockoutGuard{}, AuthConfig{})
	uc.otp = &fakeOTPGate{required: true}
	if _, bt, _, err := uc.Login(context.Background(), "u", "wrong", "", "", "", "1.2.3.4"); !errors.Is(err, ErrLoginRejected) || bt != "" {
		t.Fatalf("a wrong password must not get the binding-guide token, err=%v", err)
	}
}

// Not bound and not forced by a role: unaffected, logs in directly.
func TestLoginUnboundNotForcedSkipsOTP(t *testing.T) {
	token, bindToken, _, err := admissionLogin(t, &fakeOTPGate{}, "")

	if err != nil {
		t.Fatalf("Login: unexpected error: %v", err)
	}
	if bindToken != "" || token == "" {
		t.Fatal("not bound and not forced should log in directly")
	}
}

// Changing the password must not affect the OTP binding: they are independent factors, and changing one must
// not quietly void the other. Otherwise the user would rescan the code after every password change, which
// brings no security benefit and is easily mistaken for "changing the password lost my OTP".
// (Changing the password does bump token_version to end old sessions; that is the session layer and
// unrelated to the binding.)
func TestChangePasswordKeepsOTPBinding(t *testing.T) {
	repo := loginTestUser(t, "u", "old-pass")
	repo.user.OTPBound = true
	uc := newLoginUseCase(repo, noopLoginLog{}, &fakeLockoutGuard{}, AuthConfig{})

	if err := uc.ChangeMyPassword(context.Background(), 1, "old-pass", "new-pass-123"); err != nil {
		t.Fatalf("changing the password should succeed: %v", err)
	}
	if !repo.user.OTPBound {
		t.Fatal("changing the password must not clear the OTP binding")
	}
}
