package biz

import (
	"context"
	"errors"
	"testing"

	"github.com/go-keel/keel-admin/internal/core/captcha"
)

// fakeIssueProvider is a captcha.Provider test double whose Issue result is
// controlled per test case (a nil challenge and a controllable error).
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

// newCaptchaTestUseCase builds an AuthUseCase with only the provider wired — the
// other dependencies are untouched by IssueCaptcha.
func newCaptchaTestUseCase(p captcha.Provider) *AuthUseCase {
	return NewAuthUseCase(nil, nil, nil, nil, nil, 0, nil, AuthConfig{}, p)
}

func TestIssueCaptchaReturnsConfiguredProviderChallenge(t *testing.T) {
	want := &captcha.Challenge{ID: "abc", ImageBase64: "img-data"}
	uc := newCaptchaTestUseCase(&fakeIssueProvider{challenge: want})
	got, err := uc.IssueCaptcha(context.Background())
	if err != nil {
		t.Fatalf("IssueCaptcha: unexpected error: %v", err)
	}
	if got != want {
		t.Fatalf("IssueCaptcha: got %v, want %v", got, want)
	}
}

func TestIssueCaptchaProviderNilChallengePassesThrough(t *testing.T) {
	uc := newCaptchaTestUseCase(&fakeIssueProvider{challenge: nil})
	got, err := uc.IssueCaptcha(context.Background())
	if err != nil {
		t.Fatalf("IssueCaptcha: unexpected error: %v", err)
	}
	if got != nil {
		t.Fatalf("IssueCaptcha: expected nil challenge, got %v", got)
	}
}

// With captcha off there is no provider: the anonymous endpoint issues nothing and writes nothing.
func TestIssueCaptchaWithoutProviderReturnsNilNoError(t *testing.T) {
	uc := newCaptchaTestUseCase(nil)
	got, err := uc.IssueCaptcha(context.Background())
	if err != nil {
		t.Fatalf("IssueCaptcha: unexpected error: %v", err)
	}
	if got != nil {
		t.Fatalf("IssueCaptcha: expected nil challenge without a provider, got %v", got)
	}
}

func TestIssueCaptchaPropagatesProviderError(t *testing.T) {
	wantErr := errors.New("boom")
	uc := newCaptchaTestUseCase(&fakeIssueProvider{issueErr: wantErr})
	_, err := uc.IssueCaptcha(context.Background())
	if !errors.Is(err, wantErr) {
		t.Fatalf("IssueCaptcha: got error %v, want %v", err, wantErr)
	}
}
