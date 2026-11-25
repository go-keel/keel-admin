package permdata

import (
	"strings"
	"testing"

	"gorm.io/driver/mysql"
	"gorm.io/gorm"

	"github.com/go-keel/keel-admin/internal/core/perm"
)

// dryRunItem stands for a business table in the dry run below.
type dryRunItem struct {
	ID          int64 `gorm:"primaryKey"`
	OwnerUserID int64
	OwnerDeptID int64
}

// The column names of ApplyScopeWith come from the caller (the derived repo's tables), and a MySQL reserved word
// spliced into SQL raw is a syntax error. TestApplyScopeWithReservedColumnNames runs such a query on every dialect
// under make integration; this one keeps the check in make check: it uses the MySQL dialect to generate SQL only,
// without a database, and checks that all three column names are quoted and a table prefix is quoted segment by
// segment.
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
		var rows []dryRunItem
		return tx.Model(&dryRunItem{}).Scopes(ApplyScopeWith(spec, cols)).Find(&rows)
	})
	for _, want := range []string{"`sensitive` = 1", "`rank` IN (2)", "`t`.`groups` IN (3)"} {
		if !strings.Contains(sql, want) {
			t.Errorf("generated SQL lacks %s:\n%s", want, sql)
		}
	}
}
