package data

import (
	"context"
	"strconv"
	"time"

	"log/slog"

	"github.com/go-keel/keel-admin/internal/core/sys/biz"
	"github.com/go-keel/keel/core/kredis"
	kjson "github.com/go-keel/keel/pkg/json"
)

var (
	_ biz.PermCache        = (*permCache)(nil)
	_ biz.CacheInvalidator = (*permCache)(nil)
)

const permTTL = 5 * time.Minute

type permCache struct {
	rdb *kredis.Redis
}

// NewPermCache builds the Redis-backed permission cache.
func NewPermCache(rdb *kredis.Redis) biz.PermCache { return &permCache{rdb: rdb} }

// NewCacheInvalidator exposes the same cache as the invalidator port.
func NewCacheInvalidator(c biz.PermCache) biz.CacheInvalidator {
	return c.(biz.CacheInvalidator)
}

func permRoleKey(roleID int64) string { return "perm:role:" + strconv.FormatInt(roleID, 10) }

func (c *permCache) GetRoleCodes(ctx context.Context, roleID int64) ([]string, bool) {
	val, err := c.rdb.Get(ctx, permRoleKey(roleID)).Result()
	if err != nil {
		return nil, false
	}
	var codes []string
	if err := kjson.UnmarshalBytes([]byte(val), &codes); err != nil {
		return nil, false
	}
	return codes, true
}

func (c *permCache) SetRoleCodes(ctx context.Context, roleID int64, codes []string) {
	b, err := kjson.MarshalToBytes(codes)
	if err != nil {
		return
	}
	if err := c.rdb.Set(ctx, permRoleKey(roleID), b, permTTL).Err(); err != nil {
		slog.WarnContext(ctx, "perm cache set failed", "error", err)
	}
}

func (c *permCache) InvalidateRole(ctx context.Context, roleID int64) {
	if err := c.rdb.Del(ctx, permRoleKey(roleID)).Err(); err != nil {
		slog.WarnContext(ctx, "perm cache invalidate failed", "error", err)
	}
}
