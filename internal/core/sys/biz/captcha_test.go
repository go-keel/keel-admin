package biz

import (
	"context"
	"errors"
	"testing"

	"github.com/go-keel/keel-admin/internal/core/captcha"
)

// fakeIssueProvider is a captcha.Provider test double whose Issue result is
// controlled per test case (distinct from config_test.go's fakeCaptchaProvider,
// whose Issue output is fixed — this one needs a nil challenge and a
// controllable error).
type fakeIssueProvider struct {
	challenge *captcha.Challenge
	issueErr  error
}

var _ captcha.Provider = (*fakeIssueProvider)(nil)

func (p *fakeIssueProvider) Issue(context.Context) (*captcha.Challenge, error) {
	return p.challenge, p.issueErr
}
func (p *fakeIssueProvider) Verify(context.Context, string, string, string) (bool, error) {
	return false, nil
}
func (p *fakeIssueProvider) Ready() error { return nil }

// newCaptchaTestUseCase builds an AuthUseCase with only cfg wired — the
// other dependencies are untouched by IssueCaptcha.
func newCaptchaTestUseCase(provider string) *AuthUseCase {
	return NewAuthUseCase(nil, nil, nil, nil, nil, 0, nil, AuthConfig{CaptchaProvider: provider}, nil)
}

func TestIssueCaptchaReturnsConfiguredProviderChallenge(t *testing.T) {
	captcha.ResetForTest()
	defer captcha.ResetForTest()
	want := &captcha.Challenge{ID: "abc", ImageBase64: "img-data"}
	captcha.MustRegister("fake", &fakeIssueProvider{challenge: want})

	uc := newCaptchaTestUseCase("fake")
	got, err := uc.IssueCaptcha(context.Background())
	if err != nil {
		t.Fatalf("IssueCaptcha: unexpected error: %v", err)
	}
	if got != want {
		t.Fatalf("IssueCaptcha: got %v, want %v", got, want)
	}
}

func TestIssueCaptchaProviderNilChallengePassesThrough(t *testing.T) {
	captcha.ResetForTest()
	defer captcha.ResetForTest()
	captcha.MustRegister("widget", &fakeIssueProvider{challenge: nil})

	uc := newCaptchaTestUseCase("widget")
	got, err := uc.IssueCaptcha(context.Background())
	if err != nil {
		t.Fatalf("IssueCaptcha: unexpected error: %v", err)
	}
	if got != nil {
		t.Fatalf("IssueCaptcha: expected nil challenge, got %v", got)
	}
}

func TestIssueCaptchaUnregisteredProviderReturnsNilNoError(t *testing.T) {
	captcha.ResetForTest()
	defer captcha.ResetForTest()
	// Nothing registered under "base64" (default) — must degrade to "no
	// captcha" rather than error, so a misconfiguration can't break login.

	uc := newCaptchaTestUseCase("base64")
	got, err := uc.IssueCaptcha(context.Background())
	if err != nil {
		t.Fatalf("IssueCaptcha: unexpected error: %v", err)
	}
	if got != nil {
		t.Fatalf("IssueCaptcha: expected nil challenge for unregistered provider, got %v", got)
	}
}

func TestIssueCaptchaPropagatesProviderError(t *testing.T) {
	captcha.ResetForTest()
	defer captcha.ResetForTest()
	wantErr := errors.New("boom")
	captcha.MustRegister("fake", &fakeIssueProvider{issueErr: wantErr})

	uc := newCaptchaTestUseCase("fake")
	_, err := uc.IssueCaptcha(context.Background())
	if !errors.Is(err, wantErr) {
		t.Fatalf("IssueCaptcha: got error %v, want %v", err, wantErr)
	}
}
