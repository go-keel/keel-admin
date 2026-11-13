package data

import (
	"context"
	"encoding/json"
	"errors"

	"gorm.io/gorm"
	"gorm.io/gorm/clause"

	"github.com/go-keel/keel-admin/internal/core/sys/biz"
)

// settingsJSON is the storage shape of the sys_user.settings column. Field names match the proto's JSON names
// (protojson's lowerCamelCase), and fields a derived repo appends from field number 100 go here too. encoding/json
// ignores unknown fields by default: after a rollback to an older version the new fields cannot be read, but the
// whole profile request does not fail.
type settingsJSON struct {
	FavoriteMenus []string `json:"favoriteMenus,omitempty"`
}

func decodeSettings(raw string) biz.UserSettings {
	if raw == "" {
		return biz.UserSettings{}
	}
	var j settingsJSON
	if err := json.Unmarshal([]byte(raw), &j); err != nil {
		// Corrupt data counts as never set: preferences that cannot be read must not turn the profile into a 500.
		return biz.UserSettings{}
	}
	return biz.UserSettings{FavoriteMenus: j.FavoriteMenus}
}

// UpdateSettings reads, modifies and writes sys_user.settings, all inside a row lock.
//
// FOR UPDATE is the whole point here: without it, two browsers adding a favorite at the same time both read the same
// old list, and the later write erases the earlier entry. The lock covers one row and favorites change rarely, so it
// is not a hot spot.
func (r *userRepo) UpdateSettings(ctx context.Context, id int64, mutate func(*biz.UserSettings) error) (*biz.UserSettings, error) {
	var out biz.UserSettings
	err := r.data.DB.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		var m sysUser
		err := tx.Clauses(clause.Locking{Strength: clause.LockingStrengthUpdate}).
			Where("id = ?", id).First(&m).Error
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return biz.ErrInvalidCredential
		}
		if err != nil {
			return err
		}
		s := decodeSettings(m.Settings)
		if err := mutate(&s); err != nil {
			return err
		}
		raw, err := json.Marshal(settingsJSON{FavoriteMenus: s.FavoriteMenus})
		if err != nil {
			return err
		}
		if len(raw) > biz.MaxSettingsBytes {
			return biz.ErrInvalidArgument
		}
		if err := tx.Model(&sysUser{}).Where("id = ?", id).
			Update("settings", string(raw)).Error; err != nil {
			return err
		}
		out = s
		return nil
	})
	if err != nil {
		return nil, err
	}
	return &out, nil
}
