//go:build integration

package data

import (
	"testing"

	"gorm.io/gorm"

	"github.com/go-keel/keel-admin/internal/testdb"
)

// forEachDB runs fn once per dialect kept under migrations/, each on a fresh database built from the complete schema
// of every database (migrations/*/<dialect>/0_schema.sql, as an operator builds a new database) and dropped afterwards
// (docs/guides/testing.md, "data tests").
func forEachDB(t *testing.T, fn func(t *testing.T, db *gorm.DB)) {
	t.Helper()
	testdb.ForEachDB(t, nil, fn)
}
