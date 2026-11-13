package biz

import (
	"context"
	"errors"
	"testing"

	"github.com/go-keel/keel/pkg/pwd"
)

// countingVerifier is a passwordVerify-shaped fake that counts invocations —
// the proxy this test suite uses to prove a "dummy" Argon2id verify actually
// ran on a fail-fast Login branch. Real wall-clock timing is not reliably
// assertable in a unit test, so "verify was called" stands in for "the
// branch paid the same Argon2id cost as a genuine bad-password attempt".
type countingVerifier struct {
	calls int
}

func (v *countingVerifier) verify(_, _ string) (bool, error) {
	v.calls++
	return false, nil
}

// swapCountingPasswordVerify replaces the package-level passwordVerify for
// the duration of one test and restores it via t.Cleanup. Package-level var
// swap is not goroutine-safe, but this package's tests don't run with
// t.Parallel(), so serial execution keeps it safe.
func swapCountingPasswordVerify(t *testing.T) *countingVerifier {
	t.Helper()
	cv := &countingVerifier{}
	orig := passwordVerify
	passwordVerify = cv.verify
	t.Cleanup(func() { passwordVerify = orig })
	return cv
}

// notFoundUserRepo is a UserRepo whose FindByUsername always fails —
// simulates an unknown username. Every other method panics: Login must
// never reach them once FindByUsername has errored.
type notFoundUserRepo struct{}

func (notFoundUserRepo) FindByUsername(context.Context, string) (*User, error) {
	return nil, errors.New("not found")
}
func (notFoundUserRepo) FindByID(context.Context, int64) (*User, error) { panic("unused") }
func (notFoundUserRepo) RolePermCodesByRole(context.Context, int64) ([]string, error) {
	panic("unused")
}
func (notFoundUserRepo) RoleIDs(context.Context, int64) ([]int64, error) { panic("unused") }
func (notFoundUserRepo) SetProfile(context.Context, int64, ProfileEdit) error {
	panic("unused")
}
func (notFoundUserRepo) UpdateAccount(context.Context, int64, AccountChange) error {
	panic("unused")
}
func (notFoundUserRepo) UpdateSettings(context.Context, int64, func(*UserSettings) error) (*UserSettings, error) {
	panic("unused")
}

// TestDummyHashIsValidArgon2id guards the package-level dummyHash init: it
// must be a hash that the real pwd.VerifyPassword can actually run against
// (not a placeholder string), otherwise the dummy-verify calls below would
// error out cheaply instead of paying the genuine Argon2id cost.
func TestDummyHashIsValidArgon2id(t *testing.T) {
	ok, err := pwd.VerifyPassword("constant-time-login-guard", dummyHash)
	if err != nil || !ok {
		t.Fatalf("dummyHash should pass the real pwd.VerifyPassword: ok=%v err=%v", ok, err)
	}
}

// TestLoginUnknownUsernameRunsDummyVerify: the path where FindByUsername fails used to return directly,
// skipping the Argon2id verify, so it answered much faster than "exists + wrong password": an attacker could
// use the response time to probe whether a username exists. After the fix this branch must also run one
// verify (its result discarded).
func TestLoginUnknownUsernameRunsDummyVerify(t *testing.T) {
	cv := swapCountingPasswordVerify(t)
	guard := &fakeLockoutGuard{}
	cfg := AuthConfig{LockoutEnabled: false}
	uc := newLoginUseCase(notFoundUserRepo{}, noopLoginLog{}, guard, cfg)

	_, _, _, err := uc.Login(context.Background(), "ghost", "whatever", "", "", "", "1.2.3.4")
	if err != ErrLoginRejected {
		t.Fatalf("an unknown user should return ErrLoginRejected (revealing nothing), got %v", err)
	}
	if cv.calls < 1 {
		t.Fatalf("an unknown user should run a dummy verify to close the timing side channel, calls=%d", cv.calls)
	}
}

// TestLoginLockedRunsDummyVerify: a lockout hit used to return directly before verify, likewise skipping the
// Argon2id cost and exposing an "account locked" timing signal.
func TestLoginLockedRunsDummyVerify(t *testing.T) {
	cv := swapCountingPasswordVerify(t)
	repo := loginTestUser(t, "alice", "correct-pass")
	guard := &fakeLockoutGuard{failures: map[string]int64{"alice": 5}}
	cfg := AuthConfig{LockoutEnabled: true, LockoutMaxFailures: 5, LockoutLockMinutes: 15}
	uc := newLoginUseCase(repo, noopLoginLog{}, guard, cfg)

	_, _, _, err := uc.Login(context.Background(), "alice", "correct-pass", "", "", "", "1.2.3.4")
	if err != ErrLoginRejected {
		t.Fatalf("a lockout hit should return ErrLoginRejected (revealing nothing), got %v", err)
	}
	if cv.calls < 1 {
		t.Fatalf("a lockout hit should run a dummy verify to close the timing side channel, calls=%d", cv.calls)
	}
}

// TestLoginDisabledUserRunsDummyVerify: a disabled account, like a locked one, is "known username but
// rejected" and also needs the dummy verify to keep constant time; otherwise the response time tells
// "wrong password" from "account disabled".
func TestLoginDisabledUserRunsDummyVerify(t *testing.T) {
	cv := swapCountingPasswordVerify(t)
	repo := loginTestUser(t, "alice", "correct-pass")
	repo.user.Status = 0
	guard := &fakeLockoutGuard{}
	cfg := AuthConfig{LockoutEnabled: false}
	uc := newLoginUseCase(repo, noopLoginLog{}, guard, cfg)

	_, _, _, err := uc.Login(context.Background(), "alice", "correct-pass", "", "", "", "1.2.3.4")
	if err != ErrLoginRejected {
		t.Fatalf("a disabled account should return ErrLoginRejected (revealing nothing), got %v", err)
	}
	if cv.calls < 1 {
		t.Fatalf("a disabled account should run a dummy verify to close the timing side channel, calls=%d", cv.calls)
	}
}

// TestLoginBadPasswordStillCallsVerifyExactlyOnce is a regression guard: the
// pre-existing "user exists, wrong password" path already paid the Argon2id
// cost via a real verify call — it must keep calling exactly once (not the
// dummy hash, not skipped, not double-counted) after the refactor to the
// passwordVerify indirection.
func TestLoginBadPasswordStillCallsVerifyExactlyOnce(t *testing.T) {
	cv := swapCountingPasswordVerify(t)
	repo := loginTestUser(t, "alice", "correct-pass")
	guard := &fakeLockoutGuard{}
	cfg := AuthConfig{LockoutEnabled: false}
	uc := newLoginUseCase(repo, noopLoginLog{}, guard, cfg)

	_, _, _, err := uc.Login(context.Background(), "alice", "wrong-pass", "", "", "", "1.2.3.4")
	if err != ErrLoginRejected {
		t.Fatalf("a wrong password should return ErrLoginRejected, got %v", err)
	}
	if cv.calls != 1 {
		t.Fatalf("an existing user with a wrong password should call verify exactly once (regression), calls=%d", cv.calls)
	}
}
