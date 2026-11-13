package biz

import (
	"context"
	"time"
)

// LockoutGuard is the login brute-force counter port: tracks consecutive
// password failures per username and answers whether the account is
// currently over the configured threshold.
//
// It is an enhancement gate, not an admission gate — unlike OTPGate (which
// fails closed on a backend error, since OTP is a hard security boundary),
// a LockoutGuard failure must fail OPEN. Brute-force throttling exists to
// slow down attackers; a Redis outage must never escalate into locking out
// every legitimate user. Callers log a Warn and let the login proceed.
// subject is what is counted; biz builds it from sys.lockout.scope (see AuthUseCase.lockoutSubject). The key
// dimension is policy, so the guard treats subject as an opaque string.
type LockoutGuard interface {
	// Failures returns the current consecutive-failure count for subject
	// (0 if it has no recorded failures, e.g. never failed or the window
	// already expired).
	Failures(ctx context.Context, subject string) (int64, error)
	// RecordFailure increments the failure count for subject, arming (or
	// refreshing, on first increment only) a TTL of window so the count
	// naturally resets once the lockout window elapses.
	RecordFailure(ctx context.Context, subject string, window time.Duration) error
	// Clear resets the failure count for subject, e.g. on a successful login.
	Clear(ctx context.Context, subject string) error
}

// Key dimensions of the lockout counter (sys.lockout.scope).
const (
	// LockoutScopeUsernameIP counts per username plus client IP (the default).
	LockoutScopeUsernameIP = "username_ip"
	// LockoutScopeUsername counts per username across all IPs.
	LockoutScopeUsername = "username"
)
