package biz

import (
	"context"

	"github.com/go-keel/keel-admin/internal/core/sys/totp"
)

// OTPRepo persists OTP secret state.
type OTPRepo interface {
	GetOTP(ctx context.Context, userID int64) (enc []byte, bound bool, err error)
	SetOTPSecret(ctx context.Context, userID int64, enc []byte) error
	SetOTPBound(ctx context.Context, userID int64) error
	ClearOTP(ctx context.Context, userID int64) error
	UserForceOTP(ctx context.Context, userID int64) (bool, error)
}

// OTPGate is the login/step-up OTP check surface (implemented by OTPUseCase).
type OTPGate interface {
	Required(ctx context.Context, userID int64) (bool, error)
	Bound(ctx context.Context, userID int64) (bool, error)
	Verify(ctx context.Context, userID int64, code string) (bool, error)
}

// DefaultMaxOTPUses is the use limit of one TOTP code when sys.otp.max_uses is not configured.
//
// It deliberately relaxes replay protection. A value of 1 means one use per code (the RFC 6238 §5.2
// recommendation), at the cost that one prompt covers only one protected action and a second one in a row must
// wait for the next code. With N>1 the same code can be submitted N times within its validity window, including
// by an attacker who intercepted it; that is the risk this limit pays for directly, and it grows linearly with N.
//
// Correspondingly, merging consecutive sensitive steps into a single endpoint remains the preferred solution (see
// UpdateRole in api/keel_admin/v1/role.proto): it does not spend this allowance.
const DefaultMaxOTPUses = 3

// NormalizeMaxOTPUses turns the configured value into a usable limit. proto3 cannot tell "not set" from "set to 0",
// and 0 literally means "no code is usable", which would lock everyone out; nobody means that. So 0 and negative
// values fall back to the default. There is no upper bound: how high to go is the deployer's decision, and this
// function does not judge it.
func NormalizeMaxOTPUses(n int64) int64 {
	if n <= 0 {
		return DefaultMaxOTPUses
	}
	return n
}

// ReplayGuard caps how many times one TOTP code may be used (the cap is
// sys.otp.max_uses, held by the implementation), so a code intercepted in
// transit can't be replayed against login or step-up beyond that budget.
// TryUse consumes one use of (userID, code):
//   - (true, nil): budget not exhausted, verification may proceed.
//   - (false, nil): budget exhausted — reject, but not an error.
//   - (false, err): guard unavailable — fail-closed, reject with error.
//
// The guard keys on the code itself, not the wall-clock time step it was
// verified in: with the library's default Skew=1, a single code bound to
// counter V verifies successfully at wall-clock time steps V-1, V and V+1,
// i.e. three different windows. Keying by timestep would hand each window its
// own budget, silently tripling the cap; keying by the code makes the cap
// mean what it says.
type ReplayGuard interface {
	TryUse(ctx context.Context, userID int64, code string) (bool, error)
}

// OTPUseCase manages OTP binding and verification.
type OTPUseCase struct {
	repo   OTPRepo
	cipher *totp.Cipher
	issuer string
	guard  ReplayGuard
}

// NewOTPUseCase builds the OTP use case.
func NewOTPUseCase(repo OTPRepo, cipher *totp.Cipher, issuer string, guard ReplayGuard) *OTPUseCase {
	return &OTPUseCase{repo: repo, cipher: cipher, issuer: issuer, guard: guard}
}

// Bind generates a new secret and returns the provisioning URI. It refuses when already bound: Unbind first.
//
// Why overwriting is not allowed: Bind sets otp_bound to 0 when it stores the secret, so one click on "rebind"
// would kill the authenticator still in the user's hand without them noticing (this happened: the next sensitive
// action only answered "bind first"). Overwriting would also let whoever hijacked the session swap in their own
// authenticator without the password. Splitting it into "unbind (password checked), then bind" makes the
// destructive step an explicit, auditable decision.
func (uc *OTPUseCase) Bind(ctx context.Context, userID int64, account string) (string, error) {
	bound, err := uc.Bound(ctx, userID)
	if err != nil {
		return "", err
	}
	if bound {
		return "", ErrOTPAlreadyBound
	}
	secret, uri, err := totp.GenerateSecret(uc.issuer, account)
	if err != nil {
		return "", err
	}
	enc, err := uc.cipher.Encrypt(secret)
	if err != nil {
		return "", err
	}
	if err := uc.repo.SetOTPSecret(ctx, userID, enc); err != nil {
		return "", err
	}
	return uri, nil
}

// Confirm verifies the first code and marks the secret bound.
func (uc *OTPUseCase) Confirm(ctx context.Context, userID int64, code string) error {
	ok, err := uc.Verify(ctx, userID, code)
	if err != nil {
		return err
	}
	if !ok {
		return ErrOTPInvalid
	}
	return uc.repo.SetOTPBound(ctx, userID)
}

// Verify checks a code against the user's stored secret.
func (uc *OTPUseCase) Verify(ctx context.Context, userID int64, code string) (bool, error) {
	enc, _, err := uc.repo.GetOTP(ctx, userID)
	if err != nil {
		return false, err
	}
	if len(enc) == 0 {
		return false, ErrOTPNotBound
	}
	secret, err := uc.cipher.Decrypt(enc)
	if err != nil {
		return false, err
	}
	if !totp.Verify(secret, code) {
		return false, nil
	}
	// Consume one use of this code (D3, capped at sys.otp.max_uses). Step-up
	// verification and Confirm's bind-activation check share one budget:
	// the code that confirms binding is not a free extra use.
	allowed, err := uc.guard.TryUse(ctx, userID, code)
	if err != nil {
		return false, err // fail-closed: guard unavailable
	}
	if !allowed {
		// The code is right but its uses are spent: return the dedicated error so the caller can give an
		// actionable hint.
		return false, ErrOTPUsed
	}
	return true, nil
}

// Required reports whether any of the user's roles force OTP.
func (uc *OTPUseCase) Required(ctx context.Context, userID int64) (bool, error) {
	return uc.repo.UserForceOTP(ctx, userID)
}

// Bound reports whether the user has a confirmed OTP secret.
func (uc *OTPUseCase) Bound(ctx context.Context, userID int64) (bool, error) {
	_, bound, err := uc.repo.GetOTP(ctx, userID)
	return bound, err
}

// Unbind unbinds the caller's own authenticator. The caller checks the password first (see
// AuthUseCase.VerifyPassword): the password belongs to the account, not to the OTP use case, and the otp repo does
// not hold it.
func (uc *OTPUseCase) Unbind(ctx context.Context, userID int64) error {
	return uc.repo.ClearOTP(ctx, userID)
}

// Reset clears a user's OTP binding (root/high-priv only; high-risk backdoor — audited).
func (uc *OTPUseCase) Reset(ctx context.Context, userID int64) error {
	return uc.repo.ClearOTP(ctx, userID)
}
