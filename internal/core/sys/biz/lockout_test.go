package biz

import (
	"context"
	"testing"
	"time"

	"github.com/go-keel/keel/pkg/jwt"
	"github.com/go-keel/keel/pkg/pwd"
)

// fakeLockoutGuard is an in-memory LockoutGuard fake. err, when set, makes
// every method fail — used to exercise the fail-open path.
type fakeLockoutGuard struct {
	failures              map[string]int64
	failureCallsUsernames []string
	recordCalls           []string
	clearCalls            []string
	failureCalls          int
	err                   error
}

var _ LockoutGuard = (*fakeLockoutGuard)(nil)

func (g *fakeLockoutGuard) Failures(_ context.Context, username string) (int64, error) {
	g.failureCalls++
	g.failureCallsUsernames = append(g.failureCallsUsernames, username)
	if g.err != nil {
		return 0, g.err
	}
	return g.failures[username], nil
}

func (g *fakeLockoutGuard) RecordFailure(_ context.Context, username string, _ time.Duration) error {
	g.recordCalls = append(g.recordCalls, username)
	if g.err != nil {
		return g.err
	}
	if g.failures == nil {
		g.failures = map[string]int64{}
	}
	g.failures[username]++
	return nil
}

func (g *fakeLockoutGuard) Clear(_ context.Context, username string) error {
	g.clearCalls = append(g.clearCalls, username)
	if g.err != nil {
		return g.err
	}
	delete(g.failures, username)
	return nil
}

func (g *fakeLockoutGuard) calls() int {
	return g.failureCalls + len(g.recordCalls) + len(g.clearCalls)
}

// fakeOTPGateNoRequire always reports OTP not required, for Login tests that
// don't exercise the OTP branch.
type fakeOTPGateNoRequire struct{}

func (fakeOTPGateNoRequire) Required(context.Context, int64) (bool, error) { return false, nil }
func (fakeOTPGateNoRequire) Bound(context.Context, int64) (bool, error)    { return false, nil }
func (fakeOTPGateNoRequire) Verify(context.Context, int64, string) (bool, error) {
	return false, nil
}

// loginTestUser builds a fakeUserRepo carrying one enabled user with the
// given plaintext password hashed for real (pwd.VerifyPassword must pass).
func loginTestUser(t *testing.T, username, password string) *fakeUserRepo {
	t.Helper()
	hash, err := pwd.HashPassword(password)
	if err != nil {
		t.Fatal(err)
	}
	return &fakeUserRepo{user: &User{ID: 1, Username: username, Password: hash, Status: 1}}
}

// testSigner builds an HS256 signer for tests. The key is non-empty, so construction cannot fail.
func testSigner() jwt.Signer {
	s, err := jwt.NewHS256Signer([]byte("test-secret"))
	if err != nil {
		panic(err)
	}
	return s
}

func newLoginUseCase(repo UserRepo, log LoginLogger, guard LockoutGuard, cfg AuthConfig) *AuthUseCase {
	signer := testSigner()
	// captcha disabled: these tests exercise the lockout guard, not captcha —
	// see auth_captcha_test.go for captcha step 0 coverage.
	return NewAuthUseCase(repo, log, &fakePermCache{}, fakeOTPGateNoRequire{}, signer, time.Hour, guard, cfg, &fakeCaptchaGate{enabled: false})
}

func TestLoginWithinThresholdRecordsFailureOnBadPassword(t *testing.T) {
	repo := loginTestUser(t, "alice", "correct-pass")
	guard := &fakeLockoutGuard{failures: map[string]int64{}}
	store := AuthConfig{LockoutEnabled: true, LockoutMaxFailures: 5, LockoutLockMinutes: 15}
	uc := newLoginUseCase(repo, noopLoginLog{}, guard, store)

	_, _, _, err := uc.Login(context.Background(), "alice", "wrong-pass", "", "", "", "1.2.3.4")
	if err != ErrLoginRejected {
		t.Fatalf("a bad password should return ErrLoginRejected, got %v", err)
	}
	if len(guard.recordCalls) != 1 || guard.recordCalls[0] != "alice|1.2.3.4" {
		t.Fatalf("under the threshold a wrong password should call RecordFailure once (see lockoutSubject): %v", guard.recordCalls)
	}
}

func TestLoginLockedAfterMaxFailuresBlocksEvenCorrectPassword(t *testing.T) {
	repo := loginTestUser(t, "alice", "correct-pass")
	guard := &fakeLockoutGuard{failures: map[string]int64{"alice|1.2.3.4": 5}}
	store := AuthConfig{LockoutEnabled: true, LockoutMaxFailures: 5, LockoutLockMinutes: 15}
	uc := newLoginUseCase(repo, noopLoginLog{}, guard, store)

	_, _, _, err := uc.Login(context.Background(), "alice", "correct-pass", "", "", "", "1.2.3.4")
	if err != ErrLoginRejected {
		t.Fatalf("at the threshold even a correct password should get ErrLoginRejected (lockout hidden), got %v", err)
	}
	// Locked out before password verification: RecordFailure must not fire
	// (the correct-password attempt never reaches that branch), and the
	// counter must not have grown from what the guard already reported.
	if len(guard.recordCalls) != 0 {
		t.Fatalf("at the threshold login should stop before the password check, no RecordFailure: %v", guard.recordCalls)
	}
}

func TestLoginSuccessClearsLockout(t *testing.T) {
	repo := loginTestUser(t, "alice", "correct-pass")
	guard := &fakeLockoutGuard{failures: map[string]int64{"alice|1.2.3.4": 3}}
	store := AuthConfig{LockoutEnabled: true, LockoutMaxFailures: 5, LockoutLockMinutes: 15}
	uc := newLoginUseCase(repo, noopLoginLog{}, guard, store)

	token, _, _, err := uc.Login(context.Background(), "alice", "correct-pass", "", "", "", "1.2.3.4")
	if err != nil {
		t.Fatalf("a correct password should log in: %v", err)
	}
	if token == "" {
		t.Fatal("a token should be issued")
	}
	if len(guard.clearCalls) != 1 || guard.clearCalls[0] != "alice|1.2.3.4" {
		t.Fatalf("a successful login should call Clear once (subject: see lockoutSubject): %v", guard.clearCalls)
	}
}

func TestLoginLockoutDisabledBypassesGuardEntirely(t *testing.T) {
	repo := loginTestUser(t, "alice", "correct-pass")
	guard := &fakeLockoutGuard{failures: map[string]int64{"alice|1.2.3.4": 99}}
	store := AuthConfig{LockoutEnabled: false}
	uc := newLoginUseCase(repo, noopLoginLog{}, guard, store)

	// Even with a huge recorded failure count, disabled means the guard is
	// never consulted — bad password just fails normally...
	if _, _, _, err := uc.Login(context.Background(), "alice", "wrong-pass", "", "", "", "1.2.3.4"); err != ErrLoginRejected {
		t.Fatalf("a bad password should return ErrLoginRejected, got %v", err)
	}
	// ...and correct password succeeds despite the (ignored) failure count.
	if _, _, _, err := uc.Login(context.Background(), "alice", "correct-pass", "", "", "", "1.2.3.4"); err != nil {
		t.Fatalf("enabled=false must not lock out: %v", err)
	}
	if guard.calls() != 0 {
		t.Fatalf("enabled=false should bypass the guard entirely, zero calls: failures=%d record=%v clear=%v",
			guard.failureCalls, guard.recordCalls, guard.clearCalls)
	}
}

func TestLoginGuardErrorFailsOpen(t *testing.T) {
	repo := loginTestUser(t, "alice", "correct-pass")
	guard := &fakeLockoutGuard{err: context.DeadlineExceeded}
	store := AuthConfig{LockoutEnabled: true, LockoutMaxFailures: 5, LockoutLockMinutes: 15}
	uc := newLoginUseCase(repo, noopLoginLog{}, guard, store)

	token, _, _, err := uc.Login(context.Background(), "alice", "correct-pass", "", "", "", "1.2.3.4")
	if err != nil {
		t.Fatalf("a guard error should fail open, a correct password should still log in: %v", err)
	}
	if token == "" {
		t.Fatal("a token should be issued when failing open too")
	}
}

// TestLoginLockoutKeyUsesCanonicalUsername reproduces and guards against the bug of bypassing the lockout by
// rotating letter case: MySQL FindByUsername is case-insensitive under the utf8mb4_*_ci collation ("Admin"
// matches the "admin" row), but if the lockout guard received the username as typed in the request,
// "lockout:Admin" and "lockout:admin" would be two separate Redis keys, and an attacker rotating case could
// keep every key's count below the threshold. The guard must receive the canonical form u.Username from the
// database (here the fake FindByUsername returns Username:"admin" for any case, simulating the CI
// collation), not the request's "Admin".
func TestLoginLockoutKeyUsesCanonicalUsername(t *testing.T) {
	repo := loginTestUser(t, "admin", "correct-pass")
	guard := &fakeLockoutGuard{failures: map[string]int64{}}
	store := AuthConfig{LockoutEnabled: true, LockoutMaxFailures: 5, LockoutLockMinutes: 15}
	uc := newLoginUseCase(repo, noopLoginLog{}, guard, store)

	_, _, _, err := uc.Login(context.Background(), "Admin", "wrong-pass", "", "", "", "1.2.3.4")
	if err != ErrLoginRejected {
		t.Fatalf("a bad password should return ErrLoginRejected, got %v", err)
	}

	if len(guard.failureCallsUsernames) != 1 || guard.failureCallsUsernames[0] != "admin|1.2.3.4" {
		t.Fatalf("Failures should get a subject built from the canonical username: %v", guard.failureCallsUsernames)
	}
	if len(guard.recordCalls) != 1 || guard.recordCalls[0] != "admin|1.2.3.4" {
		t.Fatalf("RecordFailure should get the canonical username, not the request's \"Admin\": %v", guard.recordCalls)
	}
}
