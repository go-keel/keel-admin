package biz

import (
	"context"
	"slices"
	"strings"
)

// Limits of favorites. Paths come from the frontend's route table, and the server does not check that the page a
// path points to exists: routes belong to the frontend, and a server-side allowlist would drift and block pages
// that just shipped.
const (
	MaxFavorites       = 30
	MaxFavoritePathLen = 128
	// MaxSettingsBytes is the limit on the whole serialized settings, checked by the data layer before writing. The
	// kernel's fields cannot reach it (30 × 128 is under 4 KB); it stops what a derived repo appends in field numbers
	// from 100 up.
	MaxSettingsBytes = 64 << 10
)

// UserSettings is the personal settings that follow the account (a domain object), one to one with the proto
// api.keel_admin.v1.UserSettings; only the data layer knows the stored form (JSON text).
//
// What belongs here: choices that follow the account and serve only the user's own interface. What does not: data
// the server queries across users, security-related data that needs auditing, and business-system data; each has
// its own table.
type UserSettings struct {
	FavoriteMenus []string
}

// AddFavorite adds a menu path to the caller's favorites and returns the full settings afterwards. It is
// idempotent: adding a path already in favorites succeeds and leaves the list unchanged.
func (uc *AuthUseCase) AddFavorite(ctx context.Context, userID int64, path string) (*UserSettings, error) {
	path, err := normalizeFavorite(path)
	if err != nil {
		return nil, err
	}
	return uc.users.UpdateSettings(ctx, userID, func(s *UserSettings) error {
		if slices.Contains(s.FavoriteMenus, path) {
			return nil
		}
		if len(s.FavoriteMenus) >= MaxFavorites {
			return ErrFavoriteLimit
		}
		// Append at the end: the order is the favorites order, and the sidebar renders in that order.
		s.FavoriteMenus = append(s.FavoriteMenus, path)
		return nil
	})
}

// RemoveFavorite removes a menu path from the caller's favorites and returns the full settings afterwards. It is
// idempotent: removing a path not in favorites succeeds and leaves the list unchanged.
func (uc *AuthUseCase) RemoveFavorite(ctx context.Context, userID int64, path string) (*UserSettings, error) {
	path, err := normalizeFavorite(path)
	if err != nil {
		return nil, err
	}
	return uc.users.UpdateSettings(ctx, userID, func(s *UserSettings) error {
		s.FavoriteMenus = slices.DeleteFunc(s.FavoriteMenus, func(p string) bool { return p == path })
		return nil
	})
}

// ReorderFavorites reorders favorites in the order paths gives and returns the full settings afterwards.
//
// **It accepts only a permutation of the current set**: a path not in favorites is rejected; paths left out stay
// at the end in their existing relative order. This is the key difference from "overwrite the whole list": without
// the limit it would be a whole-list overwrite that bypasses the per-item writes, and when two browsers each add a
// favorite, the later drag would erase the earlier addition without the user noticing what was lost (trade-off in
// docs/adr/admin-0011-user-settings-storage-and-write-granularity.md).
//
// The order itself is overwritten as a total order: a user dragging does mean "this is the whole order".
func (uc *AuthUseCase) ReorderFavorites(
	ctx context.Context,
	userID int64,
	paths []string,
) (*UserSettings, error) {
	want := make([]string, 0, len(paths))
	for _, p := range paths {
		p, err := normalizeFavorite(p)
		if err != nil {
			return nil, err
		}
		if slices.Contains(want, p) {
			return nil, ErrInvalidArgument
		}
		want = append(want, p)
	}
	return uc.users.UpdateSettings(ctx, userID, func(s *UserSettings) error {
		for _, p := range want {
			if !slices.Contains(s.FavoriteMenus, p) {
				// A path not in favorites: the client holds a stale snapshot, or tries to add a favorite through
				// reordering. Both are rejected; adding has its own entry point.
				return ErrInvalidArgument
			}
		}
		// Paths left out stay at the end in their existing relative order: another browser just added them, and
		// this drag must not drop them along the way.
		rest := slices.DeleteFunc(
			slices.Clone(s.FavoriteMenus),
			func(p string) bool { return slices.Contains(want, p) },
		)
		s.FavoriteMenus = append(want, rest...)
		return nil
	})
}

func normalizeFavorite(path string) (string, error) {
	path = strings.TrimSpace(path)
	if path == "" || len(path) > MaxFavoritePathLen {
		return "", ErrInvalidArgument
	}
	return path, nil
}
