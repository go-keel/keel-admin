//go:build integration

package data

import (
	"testing"

	"gorm.io/gorm"

	"github.com/go-keel/keel-admin/internal/testdb"
)

// forEachDB runs fn once per dialect kept under migrations/, each on a fresh admin database built from
// migrations/admin/<dialect>/0_schema.sql (as an operator builds a new database) and dropped afterwards. Kernel tests
// touch only sys_* tables, so the business schemas are not loaded. The root seed is deleted first: each test inserts
// exactly the rows it asserts on.
func forEachDB(t *testing.T, fn func(t *testing.T, db *gorm.DB)) {
	t.Helper()
	testdb.ForEachDB(t, []string{"admin"}, func(t *testing.T, db *gorm.DB) {
		for _, table := range []string{"sys_user", "sys_dept"} {
			if err := db.Exec("DELETE FROM " + table).Error; err != nil {
				t.Fatalf("clear seed: %v", err)
			}
		}
		fn(t, db)
	})
}
