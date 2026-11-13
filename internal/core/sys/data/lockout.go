package data

import (
	"context"
	"errors"
	"strconv"
	"time"

	"github.com/redis/go-redis/v9"

	"github.com/go-keel/keel-admin/internal/core/sys/biz"
	"github.com/go-keel/keel/core/kredis"
)

var _ biz.LockoutGuard = (*lockoutGuard)(nil)

// recordFailureScript atomically increments the failure counter and, only
// on the increment that creates the key (INCR result == 1), arms the
// window TTL — a single round trip so a connection drop can never leave
// the key incremented but without a TTL (the two-step INCR-then-EXPIRE
// this replaces could do exactly that).
var recordFailureScript = redis.NewScript(`
local n = redis.call('INCR', KEYS[1])
if n == 1 then redis.call('PEXPIRE', KEYS[1], ARGV[1]) end
return n
`)

// lockoutGuard is the Redis-backed biz.LockoutGuard: a single INCR counter
// per subject under key "lockout:<subject>", TTL'd to the configured lockout
// window on first increment. What a subject is (username, or username+IP) is
// biz's call — see AuthUseCase.lockoutSubject; this layer never parses it.
type lockoutGuard struct {
	rdb *kredis.Redis
}

// NewLockoutGuard builds the Redis-backed login brute-force guard.
func NewLockoutGuard(rdb *kredis.Redis) biz.LockoutGuard {
	return &lockoutGuard{rdb: rdb}
}

func lockoutKey(subject string) string { return "lockout:" + subject }

// Failures reads the current counter; a missing key (never failed, or the
// window already expired) reports 0, not an error.
func (g *lockoutGuard) Failures(ctx context.Context, subject string) (int64, error) {
	val, err := g.rdb.Get(ctx, lockoutKey(subject)).Result()
	if errors.Is(err, redis.Nil) {
		return 0, nil
	}
	if err != nil {
		return 0, err
	}
	n, err := strconv.ParseInt(val, 10, 64)
	if err != nil {
		return 0, err
	}
	return n, nil
}

// RecordFailure increments the counter and, only on the increment that
// creates the key (INCR result == 1), arms the window TTL — so the counter
// keeps counting within one window instead of having its expiry pushed back
// on every failure. The increment and the first-time TTL arm run as a
// single Lua script so they commit atomically: a connection drop can never
// leave the key incremented but permanently without a TTL.
func (g *lockoutGuard) RecordFailure(ctx context.Context, subject string, window time.Duration) error {
	key := lockoutKey(subject)
	return recordFailureScript.Run(ctx, g.rdb, []string{key}, window.Milliseconds()).Err()
}

// Clear removes the counter, e.g. after a successful login.
func (g *lockoutGuard) Clear(ctx context.Context, subject string) error {
	return g.rdb.Del(ctx, lockoutKey(subject)).Err()
}
