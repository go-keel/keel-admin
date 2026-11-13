package data

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"time"

	"github.com/redis/go-redis/v9"

	"github.com/go-keel/keel-admin/internal/core/sys/biz"
	"github.com/go-keel/keel/core/kredis"
)

var _ biz.ReplayGuard = (*replayGuard)(nil)

// replayTTL bounds how long a use counter lives in Redis. A code verified
// with Skew=1 stays valid for up to 3 time steps (90s), so the counter must
// outlive that entire window — not just one step — or the budget would reset
// while totp.Verify still accepts the code, handing out a fresh budget.
const replayTTL = 90 * time.Second

// consumeScript atomically increments the use counter and, only on the
// increment that creates the key (INCR result == 1), arms the TTL — one round
// trip, so a connection drop can never leave the key incremented but without
// a TTL (which would burn that code's budget until the key is evicted).
// Arming only on creation also keeps the window fixed from first use instead
// of being pushed back by every subsequent one.
var consumeScript = redis.NewScript(`
local n = redis.call('INCR', KEYS[1])
if n == 1 then redis.call('PEXPIRE', KEYS[1], ARGV[1]) end
return n
`)

type replayGuard struct {
	rdb     *kredis.Redis
	maxUses int64
}

// NewOTPReplayGuard builds the Redis-backed OTP replay guard: an INCR counter
// per (userID, code) capped at maxUses (sys.otp.max_uses), so attempts past the
// cap are rejected regardless of which time step they're checked against.
//
// Normalising happens in the constructor, not at the caller: di.go is the only caller, but a missed normalisation means
// "not configured = limit 0 = nobody can log in", and making that impossible at the type boundary beats relying on the
// caller to remember.
func NewOTPReplayGuard(rdb *kredis.Redis, maxUses int64) biz.ReplayGuard {
	return &replayGuard{rdb: rdb, maxUses: biz.NormalizeMaxOTPUses(maxUses)}
}

func (g *replayGuard) TryUse(ctx context.Context, userID int64, code string) (bool, error) {
	// Key on a hash of the code, not the code itself: the code is a
	// low-entropy secret and must not appear in plaintext in Redis keys
	// (logs, slow-log, monitoring all see keys).
	sum := sha256.Sum256([]byte(code))
	key := fmt.Sprintf("otp:used:%d:%s", userID, hex.EncodeToString(sum[:])[:16])
	n, err := consumeScript.Run(ctx, g.rdb, []string{key}, replayTTL.Milliseconds()).Int64()
	if err != nil {
		return false, err
	}
	return n <= g.maxUses, nil
}
