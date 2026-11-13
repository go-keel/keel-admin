package biz

import (
	"context"
	"testing"
)

// lockoutScopeLogin runs one login with a wrong password and returns the subject that was counted.
func lockoutScopeLogin(t *testing.T, scope, ip string) string {
	t.Helper()
	repo := loginTestUser(t, "alice", "correct-pass")
	guard := &fakeLockoutGuard{}
	uc := newLoginUseCase(repo, noopLoginLog{}, guard, AuthConfig{
		LockoutEnabled: true, LockoutMaxFailures: 5, LockoutLockMinutes: 15,
		LockoutScope: scope,
	})

	_, _, _, _ = uc.Login(context.Background(), "alice", "wrong", "", "", "", ip)
	if len(guard.recordCalls) != 1 {
		t.Fatalf("a wrong password should count once, got %d", len(guard.recordCalls))
	}
	return guard.recordCalls[0]
}

// The default (scope not configured) is username_ip: the lock applies only to the attacker's IP, so nobody can
// lock someone else's account globally just by guessing the username (account-lockout DoS).
func TestLockoutDefaultScopeIsPerIP(t *testing.T) {
	subject := lockoutScopeLogin(t, "", "10.0.0.1")

	if subject == "alice" {
		t.Fatal("the default must not key on the username alone: then anyone could lock out a chosen account")
	}
	if subject != "alice|10.0.0.1" {
		t.Fatalf("the default should key on username+IP, got %q", subject)
	}
}

// Different IPs land in different counting buckets: the account owner is not dragged down by an attacker
// elsewhere.
func TestLockoutPerIPIsolatesAttacker(t *testing.T) {
	attacker := lockoutScopeLogin(t, LockoutScopeUsernameIP, "1.2.3.4")
	victim := lockoutScopeLogin(t, LockoutScopeUsernameIP, "10.0.0.9")

	if attacker == victim {
		t.Fatalf("different source IPs for one account must count in separate buckets, both are %q", attacker)
	}
}

// Explicitly choosing username keeps the global lock (the deployer accepts the DoS risk).
func TestLockoutScopeUsernameIsGlobal(t *testing.T) {
	subject := lockoutScopeLogin(t, LockoutScopeUsername, "10.0.0.1")

	if subject != "alice" {
		t.Fatalf("an explicit username scope should key on the username alone, got %q", subject)
	}
}
