package data

import (
	"context"
	"testing"

	"gorm.io/gorm"

	coredata "github.com/go-keel/keel-admin/internal/core/data"
	"github.com/go-keel/keel-admin/internal/core/sys/biz"
)

// userListTestDB holds the minimal column set for ListUser's filters (the DDL source of truth is
// migrations/admin/0_schema.sql).
func userListTestDB(t *testing.T) *gorm.DB {
	t.Helper()
	db := openMemoryDB(t, &gorm.Config{})
	ddl := []string{
		`CREATE TABLE sys_user (
			id INTEGER PRIMARY KEY, username TEXT, password TEXT, nickname TEXT,
			email TEXT DEFAULT '', phone TEXT DEFAULT '',
			dept_id INTEGER, is_root NUMERIC, status INTEGER,
			otp_bound NUMERIC, token_version INTEGER, avatar TEXT, ip_whitelist TEXT,
			deleted_at DATETIME
		)`,
		`CREATE TABLE sys_role (id INTEGER PRIMARY KEY, deleted_at DATETIME)`,
		`CREATE TABLE sys_user_role (user_id INTEGER, role_id INTEGER)`,
	}
	for _, s := range ddl {
		if err := db.Exec(s).Error; err != nil {
			t.Fatalf("ddl: %v", err)
		}
	}
	seed := []string{
		// 1 alice: active, bound, department 1, role 9.
		`INSERT INTO sys_user (id,username,nickname,phone,dept_id,status,otp_bound) VALUES (1,'alice','Alice','+86 138-0000-1111',1,1,1)`,
		// 2 bob: disabled, not bound, department 2, role 9.
		`INSERT INTO sys_user (id,username,nickname,dept_id,status,otp_bound) VALUES (2,'bob','Bobby',2,0,0)`,
		// 3 carol: active, not bound, department 2, no role.
		`INSERT INTO sys_user (id,username,nickname,email,dept_id,status,otp_bound) VALUES (3,'carol','Carol','carol@corp.io',2,1,0)`,
		`INSERT INTO sys_role (id) VALUES (9)`,
		`INSERT INTO sys_user_role (user_id, role_id) VALUES (1,9),(2,9)`,
	}
	for _, s := range seed {
		if err := db.Exec(s).Error; err != nil {
			t.Fatalf("seed: %v", err)
		}
	}
	return db
}

func listIDs(t *testing.T, r biz.UserAdminRepo, inDept []int64, f biz.UserFilter) []int64 {
	t.Helper()
	us, _, err := r.List(context.Background(), inDept, f, 0, 50)
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	out := make([]int64, 0, len(us))
	for _, u := range us {
		out = append(out, u.ID)
	}
	return out
}

func eq(a, b []int64) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}

func TestUserListFilters(t *testing.T) {
	repo := NewUserAdminRepo(&coredata.Data{DB: userListTestDB(t)})

	if got := listIDs(t, repo, nil, biz.UserFilter{Status: 1}); !eq(got, []int64{1, 3}) {
		t.Errorf("status=active => %v, want [1 3]", got)
	}
	if got := listIDs(t, repo, nil, biz.UserFilter{Status: 2}); !eq(got, []int64{2}) {
		t.Errorf("status=disabled => %v, want [2]", got)
	}
	if got := listIDs(t, repo, nil, biz.UserFilter{OTPBound: 1}); !eq(got, []int64{1}) {
		t.Errorf("otp=bound => %v, want [1]", got)
	}
	if got := listIDs(t, repo, nil, biz.UserFilter{OTPBound: 2}); !eq(got, []int64{2, 3}) {
		t.Errorf("otp=not bound => %v, want [2 3]", got)
	}
	if got := listIDs(t, repo, nil, biz.UserFilter{RoleID: 9}); !eq(got, []int64{1, 2}) {
		t.Errorf("role=9 => %v, want [1 2]", got)
	}
	// Combined: role 9 and no authenticator bound; this is the query for tracking down force_otp login lockouts.
	if got := listIDs(t, repo, nil, biz.UserFilter{RoleID: 9, OTPBound: 2}); !eq(got, []int64{2}) {
		t.Errorf("role=9 and not bound => %v, want [2]", got)
	}
	if got := listIDs(t, repo, nil, biz.UserFilter{}); !eq(got, []int64{1, 2, 3}) {
		t.Errorf("zero values should not filter => %v, want [1 2 3]", got)
	}
}

func TestUserListRoleFilterIgnoresSoftDeletedRole(t *testing.T) {
	db := userListTestDB(t)
	if err := db.Exec("UPDATE sys_role SET deleted_at = CURRENT_TIMESTAMP WHERE id = 9").Error; err != nil {
		t.Fatalf("soft delete role: %v", err)
	}
	repo := NewUserAdminRepo(&coredata.Data{DB: db})
	if got := listIDs(t, repo, nil, biz.UserFilter{RoleID: 9}); len(got) != 0 {
		t.Fatalf("soft-deleted role still matches users: %v", got)
	}
}

// TestUserListKeywordDoesNotEscapeScope locks how the keyword combines with the data scope.
//
// The keyword condition contains an OR and keeps "department scope AND (username LIKE .. OR nickname LIKE ..)" only
// because GORM adds the parentheses automatically. Without them it collapses into "scope AND a OR b" and the second
// half escapes the scope filter, showing people outside the manageable departments. The current GORM version adds
// them; this test guards against that changing.
func TestUserListKeywordDoesNotEscapeScope(t *testing.T) {
	repo := NewUserAdminRepo(&coredata.Data{DB: userListTestDB(t)})

	// The data scope covers only department 1 (alice). Keyword "o" hits bob/Bobby(2) and Carol(3), both in department
	// 2, which the scope must block, so the result should be empty.
	got := listIDs(t, repo, []int64{1}, biz.UserFilter{Keyword: "o"})
	if len(got) != 0 {
		t.Errorf("keyword brought in people outside the scope => %v, want []", got)
	}
}

// TestUserListKeywordMatchesContact: the keyword also matches email and phone number, still bound by the data scope.
func TestUserListKeywordMatchesContact(t *testing.T) {
	repo := NewUserAdminRepo(&coredata.Data{DB: userListTestDB(t)})

	if got := listIDs(t, repo, nil, biz.UserFilter{Keyword: "corp.io"}); !eq(got, []int64{3}) {
		t.Errorf("search by email => %v, want [3]", got)
	}
	if got := listIDs(t, repo, nil, biz.UserFilter{Keyword: "138-0000"}); !eq(got, []int64{1}) {
		t.Errorf("search by phone => %v, want [1]", got)
	}
	if got := listIDs(t, repo, []int64{1}, biz.UserFilter{Keyword: "corp.io"}); len(got) != 0 {
		t.Errorf("email keyword brought in people outside the scope => %v, want []", got)
	}
}
