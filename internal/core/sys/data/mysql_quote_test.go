package data

import (
	"context"
	"regexp"
	"strings"
	"testing"
	"time"

	"gorm.io/driver/mysql"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"

	coredata "github.com/go-keel/keel-admin/internal/core/data"
	"github.com/go-keel/keel-admin/internal/core/sys/biz"
)

// Under the MySQL dialect, reserved-word column names in queries must be quoted.
//
// The rest of the data layer's tests run on in-memory sqlite, and sqlite does not reserve SENSITIVE: a column name
// spliced bare into SQL passes there and is a 1064 syntax error only on MySQL. Filtering the role list by "sensitive"
// and non-root role assignment both once returned 500 because of this. This test uses the MySQL dialect only to
// generate SQL without connecting (DryRun) and checks that the column name is quoted. A new query touching sensitive
// (or another reserved-word column) adds a case here.

// sqlRecorder records every SQL statement DryRun generates.
type sqlRecorder struct {
	logger.Interface
	sqls []string
}

func (r *sqlRecorder) Trace(_ context.Context, _ time.Time, fc func() (string, int64), _ error) {
	sql, _ := fc()
	r.sqls = append(r.sqls, sql)
}

func mysqlDryRun(t *testing.T) (*gorm.DB, *sqlRecorder) {
	t.Helper()
	rec := &sqlRecorder{Interface: logger.Discard}
	// The DSN only lets the dialect initialise; DryRun, skipped version detection and no ping mean no connection at all.
	db, err := gorm.Open(mysql.New(mysql.Config{
		DSN:                       "dryrun:dryrun@tcp(127.0.0.1:1)/dryrun",
		SkipInitializeWithVersion: true,
	}), &gorm.Config{DryRun: true, DisableAutomaticPing: true, Logger: rec})
	if err != nil {
		t.Fatalf("open mysql dialect: %v", err)
	}
	return db, rec
}

// bareSensitive matches sensitive not quoted by backticks (neither side is a backtick or a word character).
var bareSensitive = regexp.MustCompile("(^|[^`\\w])sensitive([^`\\w]|$)")

func assertSensitiveQuoted(t *testing.T, sqls []string, want string) {
	t.Helper()
	joined := strings.Join(sqls, "\n")
	if !strings.Contains(joined, want) {
		t.Fatalf("generated SQL has no %s:\n%s", want, joined)
	}
	if bareSensitive.MatchString(joined) {
		t.Fatalf("sensitive is not quoted (a MySQL reserved word; bare it is a 1064):\n%s", joined)
	}
}

func TestRoleListQuotesSensitiveForMySQL(t *testing.T) {
	for _, tc := range []struct {
		filter int32
		want   string
	}{
		{1, "`sensitive` = 1"},
		{2, "`sensitive` = 0"},
	} {
		db, rec := mysqlDryRun(t)
		repo := NewRoleRepo(&coredata.Data{DB: db})
		if _, _, err := repo.List(context.Background(), biz.RoleFilter{Sensitive: tc.filter}, 0, 20); err != nil {
			t.Fatalf("List(sensitive=%d): %v", tc.filter, err)
		}
		assertSensitiveQuoted(t, rec.sqls, tc.want)
	}
}

func TestSensitiveRoleIDsQuotesColumnForMySQL(t *testing.T) {
	db, rec := mysqlDryRun(t)
	repo := NewARBACRepo(&coredata.Data{DB: db})
	if _, err := repo.SensitiveRoleIDs(context.Background(), []int64{1, 2}); err != nil {
		t.Fatalf("SensitiveRoleIDs: %v", err)
	}
	assertSensitiveQuoted(t, rec.sqls, "`sensitive` = 1")
}
