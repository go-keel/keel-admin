package data

import (
	"context"
	"testing"
	"time"

	"github.com/alicebob/miniredis/v2"
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

// lockoutTestGuard starts a miniredis instance, wires it into a
// kredis.Redis (which is just redis.UniversalClient embedded — a plain
// *redis.Client pointed at miniredis satisfies that), and returns a
// lockoutGuard backed by it plus the miniredis handle for TTL/FastForward
// assertions.
func lockoutTestGuard(t *testing.T) (*lockoutGuard, *miniredis.Miniredis) {
	t.Helper()
	mr, err := miniredis.Run()
	if err != nil {
		t.Fatalf("miniredis.Run: %v", err)
	}
	t.Cleanup(mr.Close)

	client := redis.NewClient(&redis.Options{Addr: mr.Addr()})
	t.Cleanup(func() { _ = client.Close() })

	rdb := &kredis.Redis{UniversalClient: client}
	return &lockoutGuard{rdb: rdb}, mr
}

func TestLockoutRecordFailureIncrementsCount(t *testing.T) {
	guard, _ := lockoutTestGuard(t)
	ctx := context.Background()
	const window = time.Minute

	for i := 0; i < 3; i++ {
		if err := guard.RecordFailure(ctx, "alice", window); err != nil {
			t.Fatalf("RecordFailure #%d: %v", i+1, err)
		}
	}

	n, err := guard.Failures(ctx, "alice")
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
	guard, mr := lockoutTestGuard(t)
	ctx := context.Background()

	hook := &cmdRecorderHook{}
	guard.rdb.AddHook(hook)

	if err := guard.RecordFailure(ctx, "erin", time.Minute); err != nil {
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

	// Sanity: miniredis actually armed the TTL as a result of that call.
	if ttl := mr.TTL(lockoutKey("erin")); ttl <= 0 {
		t.Fatalf("TTL after RecordFailure = %v, want > 0", ttl)
	}
}

func TestLockoutRecordFailureSetsTTLAtomically(t *testing.T) {
	guard, mr := lockoutTestGuard(t)
	ctx := context.Background()
	const window = time.Minute

	if err := guard.RecordFailure(ctx, "bob", window); err != nil {
		t.Fatalf("RecordFailure: %v", err)
	}

	ttl := mr.TTL(lockoutKey("bob"))
	if ttl <= 0 {
		t.Fatalf("TTL after RecordFailure = %v, want > 0 (proves atomic INCR+PEXPIRE armed a TTL)", ttl)
	}

	// Fast-forward past the window: the key must expire and the counter
	// resets to 0, proving the TTL was actually set (not silently dropped).
	mr.FastForward(window + time.Second)

	n, err := guard.Failures(ctx, "bob")
	if err != nil {
		t.Fatalf("Failures after expiry: %v", err)
	}
	if n != 0 {
		t.Fatalf("Failures after window expiry = %d, want 0", n)
	}
}

func TestLockoutRecordFailureOnlyArmsTTLOnFirstIncrement(t *testing.T) {
	guard, mr := lockoutTestGuard(t)
	ctx := context.Background()
	const window = 10 * time.Second

	if err := guard.RecordFailure(ctx, "carol", window); err != nil {
		t.Fatalf("RecordFailure #1: %v", err)
	}

	// Advance partway through the window, then record a second failure.
	// Per the sliding-window-from-first-failure semantics, this second
	// call must NOT push the TTL back out.
	mr.FastForward(6 * time.Second)
	if err := guard.RecordFailure(ctx, "carol", window); err != nil {
		t.Fatalf("RecordFailure #2: %v", err)
	}

	// Advance the remaining 4s of the original window (total 10s elapsed
	// since the first failure). If the TTL had been refreshed on the
	// second call, the key would still be alive here.
	mr.FastForward(4 * time.Second)

	n, err := guard.Failures(ctx, "carol")
	if err != nil {
		t.Fatalf("Failures: %v", err)
	}
	if n != 0 {
		t.Fatalf("Failures = %d, want 0 (TTL should count from the FIRST failure, not be refreshed on the second)", n)
	}
}

func TestLockoutClearResetsCount(t *testing.T) {
	guard, _ := lockoutTestGuard(t)
	ctx := context.Background()
	const window = time.Minute

	if err := guard.RecordFailure(ctx, "dave", window); err != nil {
		t.Fatalf("RecordFailure: %v", err)
	}
	if err := guard.RecordFailure(ctx, "dave", window); err != nil {
		t.Fatalf("RecordFailure: %v", err)
	}

	if err := guard.Clear(ctx, "dave"); err != nil {
		t.Fatalf("Clear: %v", err)
	}

	n, err := guard.Failures(ctx, "dave")
	if err != nil {
		t.Fatalf("Failures after Clear: %v", err)
	}
	if n != 0 {
		t.Fatalf("Failures after Clear = %d, want 0", n)
	}
}
