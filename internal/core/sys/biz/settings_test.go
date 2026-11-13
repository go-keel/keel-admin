package biz

import (
	"context"
	"errors"
	"fmt"
	"slices"
	"sync"
	"testing"
)

func newSettingsUseCase() (*AuthUseCase, *fakeUserRepo) {
	repo := &fakeUserRepo{user: &User{ID: 1, Username: "alice", Status: 1}}
	return newLoginUseCase(repo, noopLoginLog{}, &fakeLockoutGuard{}, AuthConfig{}), repo
}

func TestAddFavoriteIsIdempotent(t *testing.T) {
	uc, _ := newSettingsUseCase()
	ctx := context.Background()

	if _, err := uc.AddFavorite(ctx, 1, "/sys/users"); err != nil {
		t.Fatal(err)
	}
	got, err := uc.AddFavorite(ctx, 1, "/sys/users")
	if err != nil {
		t.Fatalf("adding a duplicate favorite should succeed, not fail: %v", err)
	}
	if want := []string{"/sys/users"}; !slices.Equal(got.FavoriteMenus, want) {
		t.Fatalf("the list should be unchanged after a duplicate add: got %v, want %v", got.FavoriteMenus, want)
	}
}

func TestRemoveFavoriteIsIdempotent(t *testing.T) {
	uc, _ := newSettingsUseCase()
	ctx := context.Background()

	got, err := uc.RemoveFavorite(ctx, 1, "/never-added")
	if err != nil {
		t.Fatalf("removing a missing favorite should succeed, not fail: %v", err)
	}
	if len(got.FavoriteMenus) != 0 {
		t.Fatalf("the list should be empty: %v", got.FavoriteMenus)
	}
}

func TestAddRemoveRoundTrip(t *testing.T) {
	uc, _ := newSettingsUseCase()
	ctx := context.Background()

	for _, p := range []string{"/a", "/b", "/c"} {
		if _, err := uc.AddFavorite(ctx, 1, p); err != nil {
			t.Fatal(err)
		}
	}
	got, err := uc.RemoveFavorite(ctx, 1, "/b")
	if err != nil {
		t.Fatal(err)
	}
	// The order is the favorites order; deleting the middle entry keeps the other two in place.
	if want := []string{"/a", "/c"}; !slices.Equal(got.FavoriteMenus, want) {
		t.Fatalf("got %v, want %v", got.FavoriteMenus, want)
	}
}

func TestAddFavoriteRejectsOverLimit(t *testing.T) {
	uc, _ := newSettingsUseCase()
	ctx := context.Background()

	for i := range MaxFavorites {
		if _, err := uc.AddFavorite(ctx, 1, fmt.Sprintf("/p%d", i)); err != nil {
			t.Fatalf("entry %d should succeed: %v", i, err)
		}
	}
	if _, err := uc.AddFavorite(ctx, 1, "/one-too-many"); !errors.Is(err, ErrFavoriteLimit) {
		t.Fatalf("exceeding the limit should return ErrFavoriteLimit, got %v", err)
	}
	// Once full, removing one should allow adding again: the limit is the current count, not a lifetime quota.
	if _, err := uc.RemoveFavorite(ctx, 1, "/p0"); err != nil {
		t.Fatal(err)
	}
	if _, err := uc.AddFavorite(ctx, 1, "/one-too-many"); err != nil {
		t.Fatalf("adding should work after freeing a slot: %v", err)
	}
}

func TestFavoritePathValidation(t *testing.T) {
	uc, _ := newSettingsUseCase()
	ctx := context.Background()

	long := ""
	for len(long) <= MaxFavoritePathLen {
		long += "x"
	}
	for _, path := range []string{"", "   ", long} {
		if _, err := uc.AddFavorite(ctx, 1, path); !errors.Is(err, ErrInvalidArgument) {
			t.Fatalf("path %q should be rejected: got %v", path, err)
		}
		if _, err := uc.RemoveFavorite(ctx, 1, path); !errors.Is(err, ErrInvalidArgument) {
			t.Fatalf("path %q should be rejected: got %v", path, err)
		}
	}
}

// TestConcurrentAddsBothSurvive is why this API exists: two browsers each add a favorite, and both must remain.
// Only doing the whole read-modify-write under a row lock gives this result; with "the frontend sends back
// the whole list to overwrite", the later request would always erase the earlier one's entry.
func TestConcurrentAddsBothSurvive(t *testing.T) {
	uc, repo := newSettingsUseCase()
	ctx := context.Background()

	var wg sync.WaitGroup
	for _, p := range []string{"/from-chrome", "/from-firefox"} {
		wg.Add(1)
		go func() {
			defer wg.Done()
			if _, err := uc.AddFavorite(ctx, 1, p); err != nil {
				t.Errorf("concurrent add of %s failed: %v", p, err)
			}
		}()
	}
	wg.Wait()

	got := repo.settings.FavoriteMenus
	if len(got) != 2 ||
		!slices.Contains(got, "/from-chrome") ||
		!slices.Contains(got, "/from-firefox") {
		t.Fatalf("both favorites should survive, got %v", got)
	}
}

func TestReorderFavoritesIsFullOrder(t *testing.T) {
	uc, _ := newSettingsUseCase()
	ctx := context.Background()
	for _, p := range []string{"/a", "/b", "/c"} {
		if _, err := uc.AddFavorite(ctx, 1, p); err != nil {
			t.Fatal(err)
		}
	}

	got, err := uc.ReorderFavorites(ctx, 1, []string{"/c", "/a", "/b"})
	if err != nil {
		t.Fatal(err)
	}
	if want := []string{"/c", "/a", "/b"}; !slices.Equal(got.FavoriteMenus, want) {
		t.Fatalf("a drag expresses the whole order: got %v, want %v", got.FavoriteMenus, want)
	}
}

// TestReorderRejectsUnknownPath: rejecting unknown paths is the key difference from "overwrite the whole
// list". Without it, reordering would become a way around the per-item writes and an "add favorite"
// entry that should not exist.
func TestReorderRejectsUnknownPath(t *testing.T) {
	uc, _ := newSettingsUseCase()
	ctx := context.Background()
	if _, err := uc.AddFavorite(ctx, 1, "/a"); err != nil {
		t.Fatal(err)
	}

	if _, err := uc.ReorderFavorites(ctx, 1, []string{"/a", "/not-favorited"}); !errors.Is(err, ErrInvalidArgument) {
		t.Fatalf("a path not among the favorites should be rejected, got %v", err)
	}
	if _, err := uc.ReorderFavorites(ctx, 1, []string{"/a", "/a"}); !errors.Is(err, ErrInvalidArgument) {
		t.Fatalf("a duplicate path should be rejected, got %v", err)
	}
}

// TestReorderKeepsPathsTheClientDidNotKnowAbout: an entry another browser just added is not in this drag's list
// and must not be dropped along the way.
func TestReorderKeepsPathsTheClientDidNotKnowAbout(t *testing.T) {
	uc, repo := newSettingsUseCase()
	ctx := context.Background()
	for _, p := range []string{"/a", "/b"} {
		if _, err := uc.AddFavorite(ctx, 1, p); err != nil {
			t.Fatal(err)
		}
	}
	// The client only has [/a]; at this moment another browser adds /b.
	got, err := uc.ReorderFavorites(ctx, 1, []string{"/a"})
	if err != nil {
		t.Fatal(err)
	}
	if want := []string{"/a", "/b"}; !slices.Equal(got.FavoriteMenus, want) {
		t.Fatalf("entries not listed should stay at the end: got %v, want %v", got.FavoriteMenus, want)
	}
	if !slices.Equal(repo.settings.FavoriteMenus, []string{"/a", "/b"}) {
		t.Fatalf("the stored list should have two entries too: %v", repo.settings.FavoriteMenus)
	}
}
