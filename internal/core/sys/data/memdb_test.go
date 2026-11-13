package data

import (
	"testing"

	"github.com/glebarez/sqlite"
	"gorm.io/gorm"
)

// openMemoryDB opens an in-memory sqlite database (pure Go driver, no cgo) pinned to one connection: a :memory:
// database belongs only to the connection that opened it, and another connection from the pool would be another empty
// database (reporting no such table). Code inside a transaction that wrongly uses db instead of tx therefore hangs,
// which exposes it. Each test creates its own tables (a minimal test-only DDL).
func openMemoryDB(t *testing.T, cfg *gorm.Config) *gorm.DB {
	t.Helper()
	db, err := gorm.Open(sqlite.Open(":memory:"), cfg)
	if err != nil {
		t.Fatalf("open sqlite: %v", err)
	}
	sqlDB, err := db.DB()
	if err != nil {
		t.Fatalf("sqlite pool: %v", err)
	}
	sqlDB.SetMaxOpenConns(1)
	return db
}
