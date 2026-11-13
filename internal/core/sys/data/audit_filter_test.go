package data

import (
	"context"
	"errors"
	"slices"
	"testing"

	"gorm.io/gorm"

	coredata "github.com/go-keel/keel-admin/internal/core/data"
	"github.com/go-keel/keel-admin/internal/core/sys/biz"
)

// auditFilterTestDB holds the minimal column set for the audit list filters (the DDL source of truth is
// migrations/admin/0_schema.sql).
func auditFilterTestDB(t *testing.T) *gorm.DB {
	t.Helper()
	db := openMemoryDB(t, &gorm.Config{})
	stmts := []string{
		`CREATE TABLE sys_user (
			id INTEGER PRIMARY KEY, username TEXT, nickname TEXT, deleted_at DATETIME
		)`,
		`CREATE TABLE sys_audit_log (
			id INTEGER PRIMARY KEY, operator_id INTEGER, action TEXT, target_type TEXT,
			target_id TEXT, request_payload TEXT, reason TEXT, ip TEXT, created_at DATETIME
		)`,
		`INSERT INTO sys_user (id,username,nickname) VALUES (1,'alice','Alice'),(2,'bob','Bobby')`,
		// 3 is soft-deleted: the UI falls back to showing its ID in the operator column, and the keyword must not find it.
		`INSERT INTO sys_user (id,username,nickname,deleted_at) VALUES (3,'zoe','Zoe',CURRENT_TIMESTAMP)`,
		`INSERT INTO sys_audit_log (id,operator_id,action) VALUES
			(1,1,'user.update'),(2,2,'user.update'),(3,2,'role.update'),(4,3,'user.update')`,
	}
	for _, s := range stmts {
		if err := db.Exec(s).Error; err != nil {
			t.Fatalf("setup: %v", err)
		}
	}
	return db
}

func auditIDs(t *testing.T, f biz.AuditFilter) []int64 {
	t.Helper()
	repo := NewAuditRepo(&coredata.Data{DB: auditFilterTestDB(t)})
	logs, total, err := repo.List(context.Background(), f, 0, 50)
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	ids := make([]int64, 0, len(logs))
	for _, l := range logs {
		ids = append(ids, l.ID)
	}
	slices.Sort(ids)
	if total != int64(len(ids)) {
		t.Fatalf("total = %d, rows = %d", total, len(ids))
	}
	return ids
}

func TestAuditListOperatorKeyword(t *testing.T) {
	cases := []struct {
		name string
		f    biz.AuditFilter
		want []int64
	}{
		{"matches username", biz.AuditFilter{OperatorKeyword: "ali"}, []int64{1}},
		{"matches nickname", biz.AuditFilter{OperatorKeyword: "bby"}, []int64{2, 3}},
		// Matching nobody means no results; it must not degrade into "no filter" and return every record.
		{"matches nobody", biz.AuditFilter{OperatorKeyword: "nobody"}, []int64{}},
		{"soft-deleted account does not match", biz.AuditFilter{OperatorKeyword: "zoe"}, []int64{}},
		// The keyword condition contains an OR (username or nickname) and must be wrapped whole in the subquery;
		// otherwise it collapses into "action AND a OR b" and the second half escapes the other filters.
		{"applies together with other filters", biz.AuditFilter{OperatorKeyword: "b", Action: "role.update"}, []int64{3}},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if got := auditIDs(t, c.f); !slices.Equal(got, c.want) {
				t.Errorf("got %v, want %v", got, c.want)
			}
		})
	}
}

// The list does not read the request body or the reason (the list does not show them, and the body is large text);
// the detail view gets every field by ID, with the operator name, and reports ErrAuditLogNotFound when missing.
func TestAuditListOmitsPayloadAndGetReturnsIt(t *testing.T) {
	db := auditFilterTestDB(t)
	if err := db.Exec(`UPDATE sys_audit_log SET request_payload = '{"name":"x"}', reason = 'fix' WHERE id = 1`).Error; err != nil {
		t.Fatalf("seed: %v", err)
	}
	repo := NewAuditRepo(&coredata.Data{DB: db})
	ctx := context.Background()

	logs, _, err := repo.List(ctx, biz.AuditFilter{}, 0, 50)
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	for _, l := range logs {
		if l.RequestPayload != "" || l.Reason != "" {
			t.Fatalf("list row %d carries payload/reason: %+v", l.ID, l)
		}
	}

	got, err := repo.Get(ctx, 1)
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	if got.RequestPayload != `{"name":"x"}` || got.Reason != "fix" || got.OperatorName != "alice" {
		t.Fatalf("Get(1) = %+v", got)
	}
	if _, err := repo.Get(ctx, 999); !errors.Is(err, biz.ErrAuditLogNotFound) {
		t.Fatalf("Get(999) err = %v, want ErrAuditLogNotFound", err)
	}
}
