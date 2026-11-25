//go:build integration

package permdata

import (
	"testing"

	"gorm.io/gorm"

	"github.com/go-keel/keel-admin/internal/core/perm"
	"github.com/go-keel/keel-admin/internal/core/testdb"
)

// item is a throwaway table used only to exercise ApplyScope's generated SQL against a real SQL engine.
type item struct {
	ID          int64 `gorm:"primaryKey"`
	OwnerUserID int64
	OwnerDeptID int64
}

// forEachItemDB runs fn once per dialect on a fresh database holding 3 item rows. AutoMigrate is forbidden for real
// schema (docs/guides/domain-model.md, "Schema") but is fine here: item is a disposable test-only table, never a
// migration.
func forEachItemDB(t *testing.T, fn func(t *testing.T, db *gorm.DB)) {
	t.Helper()
	testdb.ForEachDB(t, []string{"admin"}, func(t *testing.T, db *gorm.DB) {
		if err := db.AutoMigrate(&item{}); err != nil {
			t.Fatalf("automigrate: %v", err)
		}
		rows := []item{
			{ID: 1, OwnerUserID: 1, OwnerDeptID: 10},
			{ID: 2, OwnerUserID: 2, OwnerDeptID: 10},
			{ID: 3, OwnerUserID: 3, OwnerDeptID: 99},
		}
		if err := db.Create(&rows).Error; err != nil {
			t.Fatalf("seed: %v", err)
		}
		fn(t, db)
	})
}

func TestApplyScopeDenyAll(t *testing.T) {
	forEachItemDB(t, func(t *testing.T, db *gorm.DB) {
		var n int64
		if err := db.Model(&item{}).Scopes(ApplyScope(perm.ScopeSpec{})).Count(&n).Error; err != nil || n != 0 {
			t.Fatalf("an empty spec should deny all, got %d (%v)", n, err)
		}
	})
}

func TestApplyScopeOrMerge(t *testing.T) {
	forEachItemDB(t, func(t *testing.T, db *gorm.DB) {
		var n int64
		if err := db.Model(&item{}).Scopes(ApplyScope(perm.ScopeSpec{SelfUserID: 3, DeptIDs: []int64{10}})).Count(&n).Error; err != nil || n != 3 {
			t.Fatalf("SELF(owner=3) OR dept∈{10} should match 3 rows, got %d (%v)", n, err)
		}
	})
}

func TestApplyScopeAll(t *testing.T) {
	forEachItemDB(t, func(t *testing.T, db *gorm.DB) {
		var n int64
		if err := db.Model(&item{}).Scopes(ApplyScope(perm.ScopeSpec{All: true})).Count(&n).Error; err != nil || n != 3 {
			t.Fatalf("ALL adds no filter, got %d (%v)", n, err)
		}
	})
}

// reservedItem has column names that are reserved words on MySQL (sensitive, rank, groups): ApplyScopeWith must quote
// what the caller passes, per dialect, or the query is a syntax error.
type reservedItem struct {
	ID        int64 `gorm:"primaryKey"`
	Sensitive int64
	Rank      int64
	Groups    int64
}

func TestApplyScopeWithReservedColumnNames(t *testing.T) {
	testdb.ForEachDB(t, []string{"admin"}, func(t *testing.T, db *gorm.DB) {
		if err := db.AutoMigrate(&reservedItem{}); err != nil {
			t.Fatalf("automigrate: %v", err)
		}
		if err := db.Create(&[]reservedItem{{ID: 1, Sensitive: 1, Rank: 9, Groups: 9}, {ID: 2, Sensitive: 8, Rank: 2, Groups: 8}}).Error; err != nil {
			t.Fatalf("seed: %v", err)
		}
		spec := perm.ScopeSpec{SelfUserID: 1, DeptIDs: []int64{2}}
		var n int64
		if err := db.Table("reserved_items AS t").Scopes(ApplyScopeWith(spec, Cols{OwnerID: "sensitive", DeptID: "t.rank", PK: "groups"})).Count(&n).Error; err != nil || n != 2 {
			t.Fatalf("scope on reserved column names = %d rows, %v; want 2", n, err)
		}
	})
}
