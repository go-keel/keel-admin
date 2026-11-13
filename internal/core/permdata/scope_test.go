package permdata

import (
	"strings"
	"testing"

	"github.com/glebarez/sqlite"
	"gorm.io/driver/mysql"
	"gorm.io/gorm"

	"github.com/go-keel/keel-admin/internal/core/perm"
)

// item is a throwaway table used only to exercise ApplyScope's generated SQL
// against a real (in-memory) SQL engine.
type item struct {
	ID          int64 `gorm:"primaryKey"`
	OwnerUserID int64
	OwnerDeptID int64
}

// testDB opens an in-memory sqlite DB and seeds 3 rows. AutoMigrate is
// forbidden for business schema (see AGENTS.md) but is fine here: this is a
// disposable test-only table, never a real migration.
func testDB(t *testing.T) *gorm.DB {
	t.Helper()
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	if err != nil {
		t.Fatalf("open sqlite: %v", err)
	}
	// A :memory: database belongs only to the connection that opened it; another pooled connection is another empty
	// database (no such table). Pin it to one connection. Code that wrongly uses db instead of tx inside a
	// transaction then blocks, which exposes it.
	sqlDB, err := db.DB()
	if err != nil {
		t.Fatalf("sqlite pool: %v", err)
	}
	sqlDB.SetMaxOpenConns(1)
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
	return db
}

func TestApplyScopeDenyAll(t *testing.T) {
	db := testDB(t)
	var n int64
	db.Model(&item{}).Scopes(ApplyScope(perm.ScopeSpec{})).Count(&n)
	if n != 0 {
		t.Fatalf("an empty spec should deny all, got %d", n)
	}
}

func TestApplyScopeOrMerge(t *testing.T) {
	db := testDB(t)
	var n int64
	db.Model(&item{}).Scopes(ApplyScope(perm.ScopeSpec{SelfUserID: 3, DeptIDs: []int64{10}})).Count(&n)
	if n != 3 {
		t.Fatalf("SELF(owner=3) OR dept∈{10} should match 3 rows, got %d", n)
	}
}

func TestApplyScopeAll(t *testing.T) {
	db := testDB(t)
	var n int64
	db.Model(&item{}).Scopes(ApplyScope(perm.ScopeSpec{All: true})).Count(&n)
	if n != 3 {
		t.Fatalf("ALL adds no filter, got %d", n)
	}
}

// The column names of ApplyScopeWith come from the caller (the derived repo's tables), and a MySQL reserved word
// spliced into SQL raw is a syntax error. The tests above run on sqlite, which does not reserve these words, so they
// cannot catch it; this one uses the MySQL dialect to generate SQL only, without a database, and checks that all
// three column names are quoted and a table prefix is quoted segment by segment.
func TestApplyScopeQuotesColumnsForMySQL(t *testing.T) {
	db, err := gorm.Open(mysql.New(mysql.Config{
		DSN:                       "dryrun:dryrun@tcp(127.0.0.1:1)/dryrun",
		SkipInitializeWithVersion: true,
	}), &gorm.Config{DryRun: true, DisableAutomaticPing: true})
	if err != nil {
		t.Fatalf("open mysql dialect: %v", err)
	}
	spec := perm.ScopeSpec{SelfUserID: 1, DeptIDs: []int64{2}, ResourceIDs: []int64{3}}
	cols := Cols{OwnerID: "sensitive", DeptID: "rank", PK: "t.groups"}
	sql := db.ToSQL(func(tx *gorm.DB) *gorm.DB {
		var rows []item
		return tx.Model(&item{}).Scopes(ApplyScopeWith(spec, cols)).Find(&rows)
	})
	for _, want := range []string{"`sensitive` = 1", "`rank` IN (2)", "`t`.`groups` IN (3)"} {
		if !strings.Contains(sql, want) {
			t.Errorf("generated SQL lacks %s:\n%s", want, sql)
		}
	}
}
