//go:build integration

package data

import (
	"context"
	"fmt"
	"os"
	"testing"
	"time"

	"github.com/redis/go-redis/v9"

	"github.com/go-keel/keel/core/kredis"
)

// cmdRecorderHook records the top-level command name of every Redis call
// issued through the hooked client, so tests can assert on wire-level
// round-trip shape (e.g. "one atomic EVAL", not "INCR then a separate
// EXPIRE").
type cmdRecorderHook struct {
	names []string
}

func (h *cmdRecorderHook) DialHook(next redis.DialHook) redis.DialHook { return next }

func (h *cmdRecorderHook) ProcessHook(next redis.ProcessHook) redis.ProcessHook {
	return func(ctx context.Context, cmd redis.Cmder) error {
		h.names = append(h.names, cmd.Name())
		return next(ctx, cmd)
	}
}

func (h *cmdRecorderHook) ProcessPipelineHook(next redis.ProcessPipelineHook) redis.ProcessPipelineHook {
	return func(ctx context.Context, cmds []redis.Cmder) error {
		for _, cmd := range cmds {
			h.names = append(h.names, cmd.Name())
		}
		return next(ctx, cmds)
	}
}

// testRedis connects to the real Redis in TEST_REDIS_ADDR, database 0. Tests share that server, so every key a test
// writes carries the test name and a nanosecond timestamp, and the test deletes its own keys when it ends.
func testRedis(t *testing.T) *kredis.Redis {
	t.Helper()
	addr := os.Getenv("TEST_REDIS_ADDR")
	if addr == "" {
		t.Fatal("TEST_REDIS_ADDR is not set. make integration needs a real Redis; see docs/guides/testing.md")
	}
	client := redis.NewClient(&redis.Options{Addr: addr})
	t.Cleanup(func() { _ = client.Close() })
	return &kredis.Redis{UniversalClient: client}
}

// lockoutTestGuard returns a lockoutGuard on the real Redis and a subject unique to this test and run, whose counter
// key is deleted when the test ends.
func lockoutTestGuard(t *testing.T) (*lockoutGuard, string) {
	t.Helper()
	rdb := testRedis(t)
	subject := fmt.Sprintf("it:%s:%d", t.Name(), time.Now().UnixNano())
	t.Cleanup(func() { _ = rdb.Del(context.Background(), lockoutKey(subject)).Err() })
	return &lockoutGuard{rdb: rdb}, subject
}

// pttl reads a key's remaining time to live from Redis.
func pttl(t *testing.T, rdb *kredis.Redis, key string) time.Duration {
	t.Helper()
	ttl, err := rdb.PTTL(context.Background(), key).Result()
	if err != nil {
		t.Fatalf("PTTL %s: %v", key, err)
	}
	return ttl
}

func TestLockoutRecordFailureIncrementsCount(t *testing.T) {
	guard, subject := lockoutTestGuard(t)
	ctx := context.Background()
	const window = time.Minute

	for i := 0; i < 3; i++ {
		if err := guard.RecordFailure(ctx, subject, window); err != nil {
			t.Fatalf("RecordFailure #%d: %v", i+1, err)
		}
	}

	n, err := guard.Failures(ctx, subject)
	if err != nil {
		t.Fatalf("Failures: %v", err)
	}
	if n != 3 {
		t.Fatalf("Failures = %d, want 3", n)
	}
}

// TestLockoutRecordFailureIsSingleAtomicCall proves the fix for the
// INCR-then-conditional-EXPIRE race: RecordFailure must arm the TTL as part
// of the SAME Redis command that increments the counter (a Lua script), not
// via a separate top-level INCR followed by a separate top-level
// EXPIRE/PEXPIRE — the latter can leave a permanently-TTL-less key if the
// connection drops between the two round trips.
func TestLockoutRecordFailureIsSingleAtomicCall(t *testing.T) {
	guard, subject := lockoutTestGuard(t)
	ctx := context.Background()

	hook := &cmdRecorderHook{}
	guard.rdb.AddHook(hook)

	if err := guard.RecordFailure(ctx, subject, time.Minute); err != nil {
		t.Fatalf("RecordFailure: %v", err)
	}

	for _, name := range hook.names {
		if name == "incr" || name == "expire" || name == "pexpire" {
			t.Fatalf("RecordFailure issued top-level command %q (commands: %v); TTL must be armed inside the same atomic script call as the increment, not a separate round trip", name, hook.names)
		}
	}
	if len(hook.names) == 0 {
		t.Fatalf("RecordFailure issued no commands at all")
	}

	// Sanity: Redis actually armed the TTL as a result of that call.
	if ttl := pttl(t, guard.rdb, lockoutKey(subject)); ttl <= 0 {
		t.Fatalf("TTL after RecordFailure = %v, want > 0", ttl)
	}
}

func TestLockoutRecordFailureSetsTTLAtomically(t *testing.T) {
	guard, subject := lockoutTestGuard(t)
	ctx := context.Background()
	const window = time.Minute

	if err := guard.RecordFailure(ctx, subject, window); err != nil {
		t.Fatalf("RecordFailure: %v", err)
	}

	// The key expiring at the end of the window is Redis's own behavior; what this code owns is arming a TTL of the
	// window, so that is what is asserted.
	if ttl := pttl(t, guard.rdb, lockoutKey(subject)); ttl <= 0 || ttl > window {
		t.Fatalf("TTL after RecordFailure = %v, want in (0, %v] (proves atomic INCR+PEXPIRE armed the window)", ttl, window)
	}
}

func TestLockoutRecordFailureOnlyArmsTTLOnFirstIncrement(t *testing.T) {
	guard, subject := lockoutTestGuard(t)
	ctx := context.Background()
	const window = time.Minute
	key := lockoutKey(subject)

	if err := guard.RecordFailure(ctx, subject, window); err != nil {
		t.Fatalf("RecordFailure #1: %v", err)
	}

	// Stand in for time passing without waiting: shorten what is left of the window, then record a second failure.
	// Per the sliding-window-from-first-failure semantics, this second call must NOT push the TTL back out to the
	// whole window.
	const left = 5 * time.Second
	if err := guard.rdb.PExpire(ctx, key, left).Err(); err != nil {
		t.Fatalf("PEXPIRE: %v", err)
	}
	if err := guard.RecordFailure(ctx, subject, window); err != nil {
		t.Fatalf("RecordFailure #2: %v", err)
	}

	if ttl := pttl(t, guard.rdb, key); ttl <= 0 || ttl > left {
		t.Fatalf("TTL after the second failure = %v, want in (0, %v] (TTL should count from the FIRST failure, not be refreshed on the second)", ttl, left)
	}
}

func TestLockoutClearResetsCount(t *testing.T) {
	guard, subject := lockoutTestGuard(t)
	ctx := context.Background()
	const window = time.Minute

	if err := guard.RecordFailure(ctx, subject, window); err != nil {
		t.Fatalf("RecordFailure: %v", err)
	}
	if err := guard.RecordFailure(ctx, subject, window); err != nil {
		t.Fatalf("RecordFailure: %v", err)
	}

	if err := guard.Clear(ctx, subject); err != nil {
		t.Fatalf("Clear: %v", err)
	}

	n, err := guard.Failures(ctx, subject)
	if err != nil {
		t.Fatalf("Failures after Clear: %v", err)
	}
	if n != 0 {
		t.Fatalf("Failures after Clear = %d, want 0", n)
	}
}
