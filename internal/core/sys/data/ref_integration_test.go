//go:build integration

package data

import (
	"context"
	"testing"

	"gorm.io/gorm"

	coredata "github.com/go-keel/keel-admin/internal/core/data"
)

// User display name: the nickname if set, else the username; deleted users are absent and the caller falls back to #id.
func TestRefRepoUserNames(t *testing.T) {
	forEachDB(t, func(t *testing.T, db *gorm.DB) {
		for _, stmt := range []string{
			`INSERT INTO sys_user (id, username, password, nickname, dept_id) VALUES (1, 'alice', 'x', 'Alice', 1), (2, 'bob', 'x', '', 1)`,
			`INSERT INTO sys_user (id, username, password, nickname, dept_id, deleted_at) VALUES (3, 'gone', 'x', 'Gone', 1, '2026-01-01')`,
		} {
			if err := db.Exec(stmt).Error; err != nil {
				t.Fatalf("seed: %v", err)
			}
		}
		got, err := NewRefRepo(&coredata.Data{DB: db}).UserNames(context.Background(), []int64{1, 2, 3, 1})
		if err != nil {
			t.Fatalf("UserNames: %v", err)
		}
		want := map[int64]string{1: "Alice", 2: "bob"}
		if len(got) != len(want) || got[1] != want[1] || got[2] != want[2] {
			t.Fatalf("UserNames = %v, want %v", got, want)
		}
	})
}
