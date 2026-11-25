//go:build integration

package data

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/go-keel/keel-admin/internal/core/sys/biz"
	"github.com/go-keel/keel/core/kredis"
)

// replayTestGuard wires a guard onto the real Redis through the real constructor (so the maxUses normalization is on
// the tested path). It returns the client for TTL assertions and a user ID unique to this run: the key is
// otp:used:<userID>:<code hash>, so the ID is what keeps runs apart, and the test's keys under it are deleted when it
// ends.
func replayTestGuard(t *testing.T, maxUses int64) (biz.ReplayGuard, *kredis.Redis, int64) {
	t.Helper()
	rdb := testRedis(t)
	uid := time.Now().UnixNano()
	t.Cleanup(func() { deleteReplayKeys(rdb, uid) })
	return NewOTPReplayGuard(rdb, maxUses), rdb, uid
}

// deleteReplayKeys deletes every count key of one user.
func deleteReplayKeys(rdb *kredis.Redis, uid int64) {
	ctx := context.Background()
	if keys, _ := rdb.Keys(ctx, replayKeyPattern(uid)).Result(); len(keys) > 0 {
		_ = rdb.Del(ctx, keys...).Err()
	}
}

// replayKeyPattern matches every count key of one user.
func replayKeyPattern(uid int64) string { return fmt.Sprintf("otp:used:%d:*", uid) }

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
		guard, _, uid := replayTestGuard(t, max)
		spend(t, guard, max, uid, "123456")

		ok, err := guard.TryUse(context.Background(), uid, "123456")
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
		guard, _, uid := replayTestGuard(t, unset)
		spend(t, guard, biz.DefaultMaxOTPUses, uid, "123456")

		ok, _ := guard.TryUse(context.Background(), uid, "123456")
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
	guard, rdb, uid := replayTestGuard(t, 0)
	ctx := context.Background()

	if _, err := guard.TryUse(ctx, uid, "123456"); err != nil {
		t.Fatalf("first TryUse: %v", err)
	}
	keys, err := rdb.Keys(ctx, replayKeyPattern(uid)).Result()
	if err != nil {
		t.Fatalf("KEYS: %v", err)
	}
	if len(keys) != 1 {
		t.Fatalf("exactly 1 count key should be written, got %v", keys)
	}
	if ttl := pttl(t, rdb, keys[0]); ttl <= 0 || ttl > replayTTL {
		t.Fatalf("TTL after first use = %v, want in (0, %v]", ttl, replayTTL)
	}

	// Stand in for time passing without waiting: shorten what is left of the TTL, then use the code again. A renewed
	// TTL would jump back to the whole replayTTL.
	const left = 5 * time.Second
	if err := rdb.PExpire(ctx, keys[0], left).Err(); err != nil {
		t.Fatalf("PEXPIRE: %v", err)
	}
	if _, err := guard.TryUse(ctx, uid, "123456"); err != nil {
		t.Fatalf("second TryUse: %v", err)
	}
	if ttl := pttl(t, rdb, keys[0]); ttl <= 0 || ttl > left {
		t.Fatalf("TTL after second use = %v, want in (0, %v] (must not be renewed)", ttl, left)
	}
}

// The quota is independent per (userID, code): the key uses the first 16 hex digits of the code hash, short enough to
// look tidy, but short enough to collide would make two different codes share one quota.
func TestReplayGuardBudgetIsPerUserAndCode(t *testing.T) {
	guard, rdb, uid := replayTestGuard(t, 0)
	otherUID := uid + 1
	t.Cleanup(func() { deleteReplayKeys(rdb, otherUID) })
	ctx := context.Background()
	spend(t, guard, biz.DefaultMaxOTPUses, uid, "123456")

	ok, err := guard.TryUse(ctx, uid, "654321")
	if err != nil || !ok {
		t.Fatalf("another code of the same user should have its own quota: %v %v", ok, err)
	}
	ok, err = guard.TryUse(ctx, otherUID, "123456")
	if err != nil || !ok {
		t.Fatalf("the same code of another user should have its own quota: %v %v", ok, err)
	}
}
