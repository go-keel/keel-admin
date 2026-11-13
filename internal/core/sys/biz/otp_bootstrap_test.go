package biz

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"strings"
	"testing"
	"time"
)

func bootstrapUseCase(t *testing.T, mutate func(*User)) *AuthUseCase {
	t.Helper()
	repo := loginTestUser(t, "u", "pw")
	if mutate != nil {
		mutate(repo.user)
	}
	return newLoginUseCase(repo, noopLoginLog{}, &fakeLockoutGuard{}, AuthConfig{})
}

// Finishing the bootstrap issues a token directly: the password was checked when the bindToken was issued and
// the OTP was just checked in Confirm, so both factors are in place, equivalent to a full login. The user must
// not be forced to log in again (logging in again needs another code, spending one more of that code's
// MaxOTPUses allowance for nothing).
func TestCompleteOTPBootstrapIssuesToken(t *testing.T) {
	uc := bootstrapUseCase(t, nil)

	token, err := uc.CompleteOTPBootstrap(context.Background(), 1, "1.2.3.4")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if token == "" {
		t.Fatal("a finished bootstrap should issue an access token directly")
	}
}

// A bindToken is valid for 10 minutes, and the account may be disabled meanwhile: finishing must run admission
// again, or a disabled account could get a valid token with an old bindToken.
func TestCompleteOTPBootstrapRejectsDisabledAccount(t *testing.T) {
	uc := bootstrapUseCase(t, func(u *User) { u.Status = 0 })

	_, err := uc.CompleteOTPBootstrap(context.Background(), 1, "1.2.3.4")
	if !errors.Is(err, ErrUserDisabled) {
		t.Fatalf("an account disabled meanwhile must not get a token, got %v", err)
	}
}

// Likewise, the whitelist may have been narrowed meanwhile.
func TestCompleteOTPBootstrapRejectsIPOutOfWhitelist(t *testing.T) {
	uc := bootstrapUseCase(t, func(u *User) { u.IPWhitelist = "10.0.0.0/8" })

	_, err := uc.CompleteOTPBootstrap(context.Background(), 1, "66.66.66.66")
	if !errors.Is(err, ErrIPNotAllowed) {
		t.Fatalf("outside the whitelist no token should be issued, got %v", err)
	}
	if _, err := uc.CompleteOTPBootstrap(context.Background(), 1, "10.1.2.3"); err != nil {
		t.Fatalf("inside the whitelist should pass: %v", err)
	}
}

// "Remember me" does not affect the token lifetime: it only decides whether the session cookie the server
// sends is persistent or session-scoped (see service/cookie.go). The token is always sys.session_ttl: it is
// the same token, whether it is stored on disk is the browser's business, and the server must not sign two
// lifetimes.
func TestCompleteOTPBootstrapUsesSessionTTL(t *testing.T) {
	uc := bootstrapUseCase(t, nil)
	uc.sessionTTL = 30 * 24 * time.Hour

	token, err := uc.CompleteOTPBootstrap(context.Background(), 1, "1.2.3.4")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if ttl := ttlOf(t, token); ttl != 30*24*time.Hour {
		t.Fatalf("the bootstrap token should use session_ttl, got %v", ttl)
	}
}

// ttlOf extracts a token's exp-iat (for tests; the signature is not verified).
func ttlOf(t *testing.T, token string) time.Duration {
	t.Helper()
	parts := strings.Split(token, ".")
	if len(parts) != 3 {
		t.Fatalf("not a JWT: %q", token)
	}
	raw, err := base64.RawURLEncoding.DecodeString(parts[1])
	if err != nil {
		t.Fatal(err)
	}
	var c struct {
		Exp int64 `json:"exp"`
		Iat int64 `json:"iat"`
	}
	if err := json.Unmarshal(raw, &c); err != nil {
		t.Fatal(err)
	}
	return time.Duration(c.Exp-c.Iat) * time.Second
}
