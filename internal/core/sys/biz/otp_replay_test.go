package biz

import (
	"context"
	"errors"
	"fmt"
	"testing"
	"time"

	pquerna "github.com/pquerna/otp/totp"

	"github.com/go-keel/keel-admin/internal/core/sys/totp"
)

const testSecret = "JBSWY3DPEHPK3PXP"

// fakeOTPRepo is a minimal OTPRepo fake with a single pre-bound secret.
type fakeOTPRepo struct {
	enc   []byte
	bound bool
}

var _ OTPRepo = (*fakeOTPRepo)(nil)

func (f *fakeOTPRepo) GetOTP(context.Context, int64) ([]byte, bool, error) {
	return f.enc, f.bound, nil
}
func (f *fakeOTPRepo) SetOTPSecret(_ context.Context, _ int64, enc []byte) error {
	f.enc = enc
	return nil
}
func (f *fakeOTPRepo) SetOTPBound(context.Context, int64) error {
	f.bound = true
	return nil
}
func (f *fakeOTPRepo) ClearOTP(context.Context, int64) error {
	f.enc, f.bound = nil, false
	return nil
}
func (f *fakeOTPRepo) UserForceOTP(context.Context, int64) (bool, error) { return false, nil }

// fakeGuard is an in-memory ReplayGuard fake keyed by "uid:code" — mirrors
// the real guard's key shape (code, not wall-clock time step) and its cap
// semantics (a use counter checked against a normalized cap; zero max means
// DefaultMaxOTPUses, same as an unset sys.otp.max_uses).
type fakeGuard struct {
	used map[string]int
	max  int64
	err  error
}

var _ ReplayGuard = (*fakeGuard)(nil)

func (g *fakeGuard) TryUse(_ context.Context, userID int64, code string) (bool, error) {
	if g.err != nil {
		return false, g.err
	}
	key := keyFor(userID, code)
	g.used[key]++
	return int64(g.used[key]) <= NormalizeMaxOTPUses(g.max), nil
}

func keyFor(userID int64, code string) string {
	return fmt.Sprintf("%d:%s", userID, code)
}

func newTestOTPUseCase(t *testing.T, guard ReplayGuard) (*OTPUseCase, *fakeOTPRepo) {
	t.Helper()
	cipher, err := totp.NewCipher([]byte("0123456789abcdef0123456789abcdef"))
	if err != nil {
		t.Fatal(err)
	}
	enc, err := cipher.Encrypt(testSecret)
	if err != nil {
		t.Fatal(err)
	}
	repo := &fakeOTPRepo{enc: enc, bound: true}
	return NewOTPUseCase(repo, cipher, "keel-admin-test", guard), repo
}

func totpCodeNow(t *testing.T) string {
	t.Helper()
	code, err := pquerna.GenerateCode(testSecret, time.Now())
	if err != nil {
		t.Fatal(err)
	}
	return code
}

// Assert both sides of the boundary: the first N uses must all pass and use N+1 must be rejected. Testing only
// one side lets one of two bugs slip through: "the limit was quietly lowered" (a smaller allowance, the user
// waits for the next code for nothing) or "the limit stopped working" (the count has no effect, degrading to
// unlimited replay).
func TestVerifyAllowsUpToMaxUsesThenRejects(t *testing.T) {
	guard := &fakeGuard{used: map[string]int{}}
	uc, _ := newTestOTPUseCase(t, guard)
	ctx := context.Background()
	code := totpCodeNow(t)

	for i := 1; i <= DefaultMaxOTPUses; i++ {
		ok, err := uc.Verify(ctx, 1, code)
		if err != nil || !ok {
			t.Fatalf("use %d (limit %d) should pass: %v %v", i, DefaultMaxOTPUses, ok, err)
		}
	}

	ok, err := uc.Verify(ctx, 1, code)
	if ok {
		t.Fatalf("use %d exceeds the limit %d and should be rejected", DefaultMaxOTPUses+1, DefaultMaxOTPUses)
	}
	// Over the limit returns ErrOTPUsed rather than ErrOTPInvalid: the code itself is correct, only its uses are
	// exhausted. The layer above uses this to say "wait for the next code"; answering "incorrect code" would
	// lie to the user and only make them think the system is broken.
	if !errors.Is(err, ErrOTPUsed) {
		t.Fatalf("over the limit should return ErrOTPUsed (distinguishable from the fail-closed guard failure): %v", err)
	}
}

func TestVerifyGuardErrorFailsClosed(t *testing.T) {
	guard := &fakeGuard{used: map[string]int{}, err: context.DeadlineExceeded}
	uc, _ := newTestOTPUseCase(t, guard)
	code := totpCodeNow(t)

	ok, err := uc.Verify(context.Background(), 1, code)
	if err == nil {
		t.Fatal("a guard error should fail closed and return an error")
	}
	if ok {
		t.Fatal("a guard error must not report a pass")
	}
}

func TestVerifyDifferentUsersIndependentReplayState(t *testing.T) {
	guard := &fakeGuard{used: map[string]int{}}
	uc, _ := newTestOTPUseCase(t, guard)
	code := totpCodeNow(t)
	ctx := context.Background()

	ok, err := uc.Verify(ctx, 1, code)
	if err != nil || !ok {
		t.Fatalf("user 1's first use should pass: %v %v", ok, err)
	}
	ok, err = uc.Verify(ctx, 2, code)
	if err != nil || !ok {
		t.Fatalf("user 2's first use should pass (independent replay state): %v %v", ok, err)
	}
}

// TestVerifyBudgetIsSharedAcrossTimesteps: the guard must key on the code, not on
// the wall-clock timestep at verification time (totp.Timestep(time.Now())).
// pquerna/otp defaults to Skew=1, so a single code bound to counter V verifies
// successfully at wall-clock timesteps V-1, V and V+1 — keyed by the wall clock
// that would be three distinct guard keys, each handing out a full budget of its
// own (a 3x silent multiplier on the cap). Keying on the code itself collapses all
// three windows onto one guard key, so ONE budget of DefaultMaxOTPUses covers the
// code's whole lifetime: once it's spent, further calls must be rejected no
// matter which timestep they land in. The test spends the budget, then calls
// Verify again — guard state carries across the call, independent of any
// clock movement — to pin that down.
func TestVerifyBudgetIsSharedAcrossTimesteps(t *testing.T) {
	guard := &fakeGuard{used: map[string]int{}}
	uc, _ := newTestOTPUseCase(t, guard)
	ctx := context.Background()
	code := totpCodeNow(t)

	for i := 1; i <= DefaultMaxOTPUses; i++ {
		ok, err := uc.Verify(ctx, 1, code)
		if err != nil || !ok {
			t.Fatalf("use %d should pass: %v %v", i, ok, err)
		}
	}

	// Simulate "replay in a different timestep window": the guard key is
	// derived from the code, not the current time, so a further call with
	// the same code — regardless of any wall-clock movement — must still
	// hit the same guard key, and find the budget already spent.
	ok, err := uc.Verify(ctx, 1, code)
	if ok {
		t.Fatal("one code across time steps (skew=1, three windows) should share one allowance, " +
			"not get a fresh one because the key is bound to the time step")
	}
	if !errors.Is(err, ErrOTPUsed) {
		t.Fatalf("over the limit across time steps should also return ErrOTPUsed: %v", err)
	}
}

// Bind must be rejected while already bound: overwriting directly would let "click to rebind" instantly void
// the authenticator the user is still using (without their knowledge), and someone who hijacked the session
// could swap in their own authenticator without a password. Unbind (which checks the password) must come
// first.
func TestBindRejectsWhenAlreadyBound(t *testing.T) {
	guard := &fakeGuard{used: map[string]int{}}
	uc, repo := newTestOTPUseCase(t, guard)
	repo.bound = true

	if _, err := uc.Bind(context.Background(), 1, "u"); !errors.Is(err, ErrOTPAlreadyBound) {
		t.Fatalf("Bind while bound should return ErrOTPAlreadyBound, got %v", err)
	}
	// And the existing secret must not have been touched.
	if len(repo.enc) == 0 || !repo.bound {
		t.Fatal("a rejected Bind must not change the existing binding")
	}
}

// After unbinding, binding again works: the other half of the two-step flow must go through.
func TestBindAllowedAfterUnbind(t *testing.T) {
	guard := &fakeGuard{used: map[string]int{}}
	uc, repo := newTestOTPUseCase(t, guard)
	repo.bound = true

	if err := uc.Unbind(context.Background(), 1); err != nil {
		t.Fatalf("Unbind: %v", err)
	}
	if repo.bound || len(repo.enc) != 0 {
		t.Fatal("Unbind should clear the secret and the bound flag")
	}
	if _, err := uc.Bind(context.Background(), 1, "u"); err != nil {
		t.Fatalf("binding again after unbinding should work: %v", err)
	}
}
