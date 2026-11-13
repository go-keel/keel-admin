package data

import (
	"context"
	"testing"
	"time"

	"github.com/alicebob/miniredis/v2"
	"github.com/redis/go-redis/v9"

	"github.com/go-keel/keel-admin/internal/core/sys/biz"
	"github.com/go-keel/keel/core/kredis"
)

// replayTestGuard wires a guard onto a miniredis instance through the real
// constructor (so the maxUses normalization is on the tested path), returning
// the miniredis handle for TTL/FastForward assertions.
func replayTestGuard(t *testing.T, maxUses int64) (biz.ReplayGuard, *miniredis.Miniredis) {
	t.Helper()
	mr, err := miniredis.Run()
	if err != nil {
		t.Fatalf("miniredis.Run: %v", err)
	}
	t.Cleanup(mr.Close)

	client := redis.NewClient(&redis.Options{Addr: mr.Addr()})
	t.Cleanup(func() { _ = client.Close() })

	return NewOTPReplayGuard(&kredis.Redis{UniversalClient: client}, maxUses), mr
}

// spend calls TryUse n times, requiring every one of them to be allowed.
func spend(t *testing.T, g biz.ReplayGuard, n int64, uid int64, code string) {
	t.Helper()
	for i := int64(1); i <= n; i++ {
		ok, err := g.TryUse(context.Background(), uid, code)
		if err != nil {
			t.Fatalf("TryUse #%d: %v", i, err)
		}
		if !ok {
			t.Fatalf("use #%d should be allowed (quota %d)", i, n)
		}
	}
}

// The limit comes from configuration, not a constant: the same code must reject at different points under max=1 and
// max=5. Testing only the default would miss "the configuration is not wired at all and DefaultMaxOTPUses is always
// used".
func TestReplayGuardHonorsConfiguredMax(t *testing.T) {
	for _, max := range []int64{1, 2, 5} {
		guard, _ := replayTestGuard(t, max)
		spend(t, guard, max, 7, "123456")

		ok, err := guard.TryUse(context.Background(), 7, "123456")
		if err != nil {
			t.Fatalf("max=%d: TryUse over the limit must not error (a rejection is not a failure): %v", max, err)
		}
		if ok {
			t.Fatalf("max=%d: use #%d should be rejected", max, max+1)
		}
	}
}

// An unset configuration (the proto3 zero value) takes the default rather than a literal 0: 0 literally means "no code
// is usable" and would lock everyone out.
func TestReplayGuardUnsetMaxFallsBackToDefault(t *testing.T) {
	for _, unset := range []int64{0, -1} {
		guard, _ := replayTestGuard(t, unset)
		spend(t, guard, biz.DefaultMaxOTPUses, 7, "123456")

		ok, _ := guard.TryUse(context.Background(), 7, "123456")
		if ok {
			t.Fatalf("max=%d should fall back to the default %d, and use #%d must be rejected",
				unset, biz.DefaultMaxOTPUses, biz.DefaultMaxOTPUses+1)
		}
	}
}

// The TTL is armed only when the count goes from 0 to 1: if every use reset the TTL, the key would outlive the code
// itself (the code is long past the skew window and fails verification, but the count still takes space, pure
// garbage); worse, the "window" would slide with use instead of being a fixed 90s from the first use.
func TestReplayGuardTTLArmedOnceNotExtended(t *testing.T) {
	guard, mr := replayTestGuard(t, 0)
	ctx := context.Background()

	if _, err := guard.TryUse(ctx, 7, "123456"); err != nil {
		t.Fatalf("first TryUse: %v", err)
	}
	keys := mr.Keys()
	if len(keys) != 1 {
		t.Fatalf("exactly 1 count key should be written, got %v", keys)
	}
	if ttl := mr.TTL(keys[0]); ttl != replayTTL {
		t.Fatalf("TTL after first use = %v, want %v", ttl, replayTTL)
	}

	const elapsed = 30 * time.Second
	mr.FastForward(elapsed)
	if _, err := guard.TryUse(ctx, 7, "123456"); err != nil {
		t.Fatalf("second TryUse: %v", err)
	}
	if ttl := mr.TTL(keys[0]); ttl != replayTTL-elapsed {
		t.Fatalf("TTL after second use = %v, want %v (must not be renewed)", ttl, replayTTL-elapsed)
	}
}

// The quota is independent per (userID, code): the key uses the first 16 hex digits of the code hash, short enough to
// look tidy, but short enough to collide would make two different codes share one quota.
func TestReplayGuardBudgetIsPerUserAndCode(t *testing.T) {
	guard, _ := replayTestGuard(t, 0)
	ctx := context.Background()
	spend(t, guard, biz.DefaultMaxOTPUses, 7, "123456")

	ok, err := guard.TryUse(ctx, 7, "654321")
	if err != nil || !ok {
		t.Fatalf("another code of the same user should have its own quota: %v %v", ok, err)
	}
	ok, err = guard.TryUse(ctx, 8, "123456")
	if err != nil || !ok {
		t.Fatalf("the same code of another user should have its own quota: %v %v", ok, err)
	}
}
