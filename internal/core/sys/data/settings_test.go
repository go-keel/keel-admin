package data

import (
	"slices"
	"testing"
)

// TestDecodeSettingsIgnoresUnknownFields guards the promise of the field-number ranges: when a derived repo adds a
// field in the range from 100, a version that does not know it must still read the settings after a rollback.
//
// This is tested in the data layer, not in biz: JSON decoding happens only here, and biz's fake repo works on the
// in-memory UserSettings struct throughout, never reaching serialisation.
func TestDecodeSettingsIgnoresUnknownFields(t *testing.T) {
	got := decodeSettings(`{"favoriteMenus":["/sys/users"],"derivedRepoField":{"a":1}}`)
	if want := []string{"/sys/users"}; !slices.Equal(got.FavoriteMenus, want) {
		t.Fatalf("got %v, want %v", got.FavoriteMenus, want)
	}
}

// Corrupt data counts as never set: preferences that cannot be read must not turn the profile into a 500.
func TestDecodeSettingsTolerates(t *testing.T) {
	for _, raw := range []string{"", "not json", "[]"} {
		if got := decodeSettings(raw); len(got.FavoriteMenus) != 0 {
			t.Fatalf("raw %q should decode to empty settings, got %v", raw, got.FavoriteMenus)
		}
	}
}
