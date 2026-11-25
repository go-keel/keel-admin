package biz

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/go-keel/keel-admin/internal/core/captcha"
)

// fakeCaptcha is a controllable captcha.Provider test double: the Verify result is fixed per test case, and the call
// count proves Verify ran.
type fakeCaptcha struct {
	verifyOK  bool
	verifyErr error

	verifyCalls int
}

var _ captcha.Provider = (*fakeCaptcha)(nil)

func (g *fakeCaptcha) Issue(context.Context) (*captcha.Challenge, error) { return nil, nil }
func (g *fakeCaptcha) Ready() error                                      { return nil }

func (g *fakeCaptcha) Verify(context.Context, string, string, string) (bool, error) {
	g.verifyCalls++
	return g.verifyOK, g.verifyErr
}

// panicUserRepo is a UserRepo whose every method panics — used to prove
// captcha step 0 short-circuits Login before FindByUsername (or anything
// else account-related) is ever reached.
type panicUserRepo struct{}

func (panicUserRepo) FindByUsername(context.Context, string) (*User, error) {
	panic("FindByUsername must not be called when captcha step 0 rejects login")
}
func (panicUserRepo) FindByID(context.Context, int64) (*User, error)               { panic("unused") }
func (panicUserRepo) RolePermCodesByRole(context.Context, int64) ([]string, error) { panic("unused") }
func (panicUserRepo) RoleIDs(context.Context, int64) ([]int64, error)              { panic("unused") }
func (panicUserRepo) SetProfile(context.Context, int64, ProfileEdit) error         { panic("unused") }
func (panicUserRepo) UpdateAccount(context.Context, int64, AccountChange) error    { panic("unused") }
func (panicUserRepo) UpdateSettings(context.Context, int64, func(*UserSettings) error) (*UserSettings, error) {
	panic("unused")
}

func TestLoginCaptchaVerifyPassContinuesToUserLookup(t *testing.T) {
	repo := loginTestUser(t, "alice", "correct-pass")
	guard := &fakeLockoutGuard{}
	store := AuthConfig{LockoutEnabled: false}
	gate := &fakeCaptcha{verifyOK: true}
	signer := testSigner()
	uc := NewAuthUseCase(repo, noopLoginLog{}, &fakePermCache{}, fakeOTPGateNoRequire{}, signer, time.Hour, guard, store, gate)

	token, _, _, err := uc.Login(context.Background(), "alice", "correct-pass", "", "cid", "answer", "1.2.3.4")
	if err != nil {
		t.Fatalf("a passed captcha check should continue the login: %v", err)
	}
	if token == "" {
		t.Fatal("a token should be issued")
	}
	if gate.verifyCalls != 1 {
		t.Fatalf("Verify should be called once, got %d", gate.verifyCalls)
	}
}

func TestLoginCaptchaVerifyFailReturnsErrCaptchaInvalidNotRecordedAsLockoutFailure(t *testing.T) {
	guard := &fakeLockoutGuard{}
	store := AuthConfig{LockoutEnabled: true, LockoutMaxFailures: 5, LockoutLockMinutes: 15}
	gate := &fakeCaptcha{verifyOK: false}
	signer := testSigner()
	// panicUserRepo proves captcha runs before FindByUsername.
	uc := NewAuthUseCase(panicUserRepo{}, noopLoginLog{}, &fakePermCache{}, fakeOTPGateNoRequire{}, signer, time.Hour, guard, store, gate)

	_, _, _, err := uc.Login(context.Background(), "alice", "irrelevant", "", "cid", "bad-answer", "1.2.3.4")
	if !errors.Is(err, ErrCaptchaInvalid) {
		t.Fatalf("a failed captcha check should return ErrCaptchaInvalid, got %v", err)
	}
	if guard.calls() != 0 {
		t.Fatalf("a failed captcha check must not count toward brute-force protection: guard calls=%d", guard.calls())
	}
}

// With captcha off there is no provider (nil): login skips step 0.
func TestLoginCaptchaDisabledSkipsCaptcha(t *testing.T) {
	repo := loginTestUser(t, "alice", "correct-pass")
	guard := &fakeLockoutGuard{}
	store := AuthConfig{LockoutEnabled: false}
	signer := testSigner()
	uc := NewAuthUseCase(repo, noopLoginLog{}, &fakePermCache{}, fakeOTPGateNoRequire{}, signer, time.Hour, guard, store, nil)

	token, _, _, err := uc.Login(context.Background(), "alice", "correct-pass", "", "", "", "1.2.3.4")
	if err != nil {
		t.Fatalf("with captcha off, login should work normally: %v", err)
	}
	if token == "" {
		t.Fatal("a token should be issued")
	}
}

func TestLoginMetaReadsConfig(t *testing.T) {
	cfg := AuthConfig{CaptchaProvider: "base64"}
	uc := NewAuthUseCase(nil, nil, nil, nil, nil, 0, nil, cfg, &fakeCaptcha{})

	got := uc.LoginMeta(context.Background())
	if !got.CaptchaEnabled || got.CaptchaProvider != "base64" {
		t.Fatalf("LoginMeta should pass through the current config: %+v", got)
	}
	if NewAuthUseCase(nil, nil, nil, nil, nil, 0, nil, cfg, nil).LoginMeta(context.Background()).CaptchaEnabled {
		t.Fatal("without a provider, LoginMeta should report captcha off")
	}
}
