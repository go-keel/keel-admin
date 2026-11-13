// Package base64captcha implements captcha.Provider on top of
// github.com/mojocn/base64Captcha's digit driver: it draws a numeric image
// captcha and persists the answer in Redis under "captcha:ans:<id>".
package base64captcha

import (
	"context"
	"strings"
	"time"

	driver "github.com/mojocn/base64Captcha"

	"github.com/go-keel/keel-admin/internal/core/captcha"
	"github.com/go-keel/keel/core/kredis"
)

// Digit image tuning — fixed rendering knobs. length/ttl come from startup
// config (app.yaml sys.captcha.base64_*).
const (
	imgHeight = 80
	imgWidth  = 240
	maxSkew   = 0.7
	dotCount  = 80
)

// ansKeyPrefix namespaces answers in Redis: "captcha:ans:<id>".
const ansKeyPrefix = "captcha:ans:"

func ansKey(id string) string { return ansKeyPrefix + id }

// provider is the base64 image captcha.Provider. newStore builds the
// base64Captcha Store for one call, bound to that call's context —
// base64Captcha.Store predates context.Context, so ctx can't live on a
// struct field. Production wires redisStore (see New); tests inject
// base64Captcha's own memory store (see base64_test.go).
type provider struct {
	newStore func(ctx context.Context) driver.Store
	length   int
}

var _ captcha.Provider = (*provider)(nil)

// New builds the base64 image captcha provider, answers persisted in Redis.
// length + ttl are fixed at startup from app.yaml sys.captcha.
func New(rdb *kredis.Redis, length int, ttl time.Duration) captcha.Provider {
	return newProvider(func(ctx context.Context) driver.Store {
		return &redisStore{ctx: ctx, rdb: rdb, ttl: ttl}
	}, length)
}

func newProvider(newStore func(ctx context.Context) driver.Store, length int) *provider {
	return &provider{newStore: newStore, length: length}
}

// Issue draws a fresh numeric image captcha and stores its answer.
func (p *provider) Issue(ctx context.Context) (*captcha.Challenge, error) {
	d := driver.NewDriverDigit(imgHeight, imgWidth, p.length, maxSkew, dotCount)
	c := driver.NewCaptcha(d, p.newStore(ctx))

	id, b64, _, err := c.Generate()
	if err != nil {
		return nil, err
	}
	return &captcha.Challenge{ID: id, ImageBase64: b64}, nil
}

// Verify checks id/answer against the stored answer and deletes it
// regardless of outcome (base64Captcha Store.Verify's clear=true semantics),
// so a challenge — right or wrong — can be redeemed at most once.
func (p *provider) Verify(ctx context.Context, id, answer, _ string) (bool, error) {
	return p.newStore(ctx).Verify(id, answer, true), nil
}

// Ready always returns nil: base64 image captcha has no external dependency
// (no secret, no third-party service to reach) — it is unconditionally
// selectable as the active provider.
func (p *provider) Ready() error { return nil }

// redisStore adapts kredis.Redis to base64Captcha's Store interface
// (Set/Get/Verify). ctx is bound per call by provider.newStore, since the
// upstream interface carries none.
type redisStore struct {
	ctx context.Context
	rdb *kredis.Redis
	ttl time.Duration
}

var _ driver.Store = (*redisStore)(nil)

// Set stores the answer with the configured TTL.
func (s *redisStore) Set(id string, value string) error {
	return s.rdb.Set(s.ctx, ansKey(id), value, s.ttl).Err()
}

// Get returns the stored answer, or "" if absent/expired. clear=true GETDELs
// it (one-shot read); a Redis error is treated the same as a miss — the
// caller (Verify) can only distinguish "no valid answer" either way.
func (s *redisStore) Get(id string, clear bool) string {
	if clear {
		v, _ := s.rdb.GetDel(s.ctx, ansKey(id)).Result()
		return v
	}
	v, _ := s.rdb.Get(s.ctx, ansKey(id)).Result()
	return v
}

// Verify compares id's stored answer against answer, case-insensitively
// (base64Captcha's own semantics; the digit driver never emits letters, so
// this is a no-op in practice), consuming the stored answer via Get's
// clear=true GETDEL either way.
func (s *redisStore) Verify(id, answer string, clear bool) bool {
	if id == "" || answer == "" {
		return false
	}
	v := s.Get(id, clear)
	if v == "" {
		return false
	}
	return strings.EqualFold(strings.TrimSpace(v), strings.TrimSpace(answer))
}
