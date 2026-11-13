package biz

import (
	"context"
	"testing"
)

// rootOTPLogin runs one login with the existing login test scaffolding: OTPGate reports "no role forces it,
// not bound", so whether OTP is required depends entirely on Login's own system-level rule forcing it for root.
func rootOTPLogin(t *testing.T, isRoot bool) (token, bindToken string) {
	t.Helper()
	repo := loginTestUser(t, "u", "pw")
	repo.user.IsRoot = isRoot

	uc := newLoginUseCase(repo, noopLoginLog{}, &fakeLockoutGuard{}, AuthConfig{})
	token, bindToken, _, err := uc.Login(context.Background(), "u", "pw", "", "", "", "1.2.3.4")
	if err != nil {
		t.Fatalf("Login: unexpected error: %v", err)
	}
	return token, bindToken
}

// Root gets its permissions by bypass and usually holds no role, so it would slip past the role-driven force_otp
// gate (otp.Required only JOINs sys_user_role). The system-level rule closes this gap, and when not bound the
// first login must go through the binding bootstrap (issuing a bindToken instead of an access token).
func TestLoginRootIsForcedOTPWithoutForceRole(t *testing.T) {
	token, bindToken := rootOTPLogin(t, true)

	if token != "" {
		t.Fatal("root without OTP bound must not get an access token directly")
	}
	if bindToken == "" {
		t.Fatal("root should be forced to use OTP system-wide; when not bound it must get a binding bootstrap token")
	}
}

// Non-root without a force_otp role: unchanged, not affected by the root rule.
func TestLoginNonRootWithoutForceRoleSkipsOTP(t *testing.T) {
	token, bindToken := rootOTPLogin(t, false)

	if bindToken != "" {
		t.Fatal("non-root with no forcing role must not enter the OTP binding flow")
	}
	if token == "" {
		t.Fatal("non-root with no forcing role should log in directly")
	}
}
