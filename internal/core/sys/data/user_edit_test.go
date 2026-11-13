package data

import (
	"context"
	"errors"
	"testing"

	"gorm.io/gorm"

	coredata "github.com/go-keel/keel-admin/internal/core/data"
	"github.com/go-keel/keel-admin/internal/core/sys/biz"
)

// userEditTestDB recreates the minimal column set Edit touches (the DDL source of truth is
// migrations/admin/0_schema.sql).
func userEditTestDB(t *testing.T) *gorm.DB {
	t.Helper()
	db := openMemoryDB(t, &gorm.Config{})
	ddl := []string{
		`CREATE TABLE sys_user (
			id INTEGER PRIMARY KEY, username TEXT, nickname TEXT, avatar TEXT,
			email TEXT DEFAULT '', phone TEXT DEFAULT '',
			dept_id INTEGER, ip_whitelist TEXT, status INTEGER,
			updated_at DATETIME, deleted_at DATETIME
		)`,
		`CREATE TABLE sys_role (id INTEGER PRIMARY KEY, deleted_at DATETIME)`,
		`CREATE TABLE sys_user_role (user_id INTEGER, role_id INTEGER)`,
		`CREATE TABLE sys_dept (id INTEGER PRIMARY KEY, status INTEGER, deleted_at DATETIME)`,
	}
	for _, s := range ddl {
		if err := db.Exec(s).Error; err != nil {
			t.Fatalf("ddl: %v", err)
		}
	}
	seed := []string{
		`INSERT INTO sys_user (id, username, nickname, avatar, dept_id, ip_whitelist, status)
			VALUES (1, 'alice', 'old-nick', '', 1, '', 1)`,
		`INSERT INTO sys_role (id) VALUES (5), (6)`,
		`INSERT INTO sys_role (id, deleted_at) VALUES (8, CURRENT_TIMESTAMP)`,
		`INSERT INTO sys_user_role (user_id, role_id) VALUES (1, 5), (1, 8)`,
		`INSERT INTO sys_dept (id, status) VALUES (1, 1), (2, 1), (3, 0)`,
	}
	for _, s := range seed {
		if err := db.Exec(s).Error; err != nil {
			t.Fatalf("seed: %v", err)
		}
	}
	return db
}

// TestUserRepoEditPartialFields: a change set with only a nickname writes only the nickname, leaving department, IP
// allowlist and roles alone; fields that are nil in changes do not go into GORM's Updates map.
func TestUserRepoEditPartialFields(t *testing.T) {
	db := userEditTestDB(t)
	repo := NewUserAdminRepo(&coredata.Data{DB: db})
	ctx := context.Background()

	nickname := "new-nick"
	if err := repo.Edit(ctx, 1, biz.UserEditChanges{Nickname: &nickname}); err != nil {
		t.Fatalf("Edit: %v", err)
	}

	u, err := repo.FindByID(ctx, 1)
	if err != nil {
		t.Fatalf("FindByID: %v", err)
	}
	if u.Nickname != "new-nick" {
		t.Errorf("nickname = %q, want new-nick", u.Nickname)
	}
	if u.DeptID != 1 || u.IPWhitelist != "" {
		t.Errorf("unchanged fields were touched: dept=%d ip=%q", u.DeptID, u.IPWhitelist)
	}
	roleIDs, err := repo.RoleIDs(ctx, 1)
	if err != nil {
		t.Fatalf("RoleIDs: %v", err)
	}
	if len(roleIDs) != 1 || roleIDs[0] != 5 {
		t.Errorf("roles changed although not edited: %v", roleIDs)
	}
}

// replaceRoles is a callback that ignores the current roles and replaces them with ids.
func replaceRoles(ids ...int64) biz.RoleResolver {
	return func([]int64) ([]int64, bool, error) { return ids, true, nil }
}

// TestUserRepoEditReplacesRoles: when the callback decides to write, roles are replaced as a whole (old deleted, new
// inserted), the same as Create; old rows on deleted roles are deleted too.
func TestUserRepoEditReplacesRoles(t *testing.T) {
	db := userEditTestDB(t)
	repo := NewUserAdminRepo(&coredata.Data{DB: db})
	ctx := context.Background()

	if err := repo.Edit(ctx, 1, biz.UserEditChanges{Roles: replaceRoles(6)}); err != nil {
		t.Fatalf("Edit: %v", err)
	}

	roleIDs, err := repo.RoleIDs(ctx, 1)
	if err != nil {
		t.Fatalf("RoleIDs: %v", err)
	}
	if len(roleIDs) != 1 || roleIDs[0] != 6 {
		t.Fatalf("roles should be replaced with {6}, got %v", roleIDs)
	}
	var n int64
	if err := db.Raw(`SELECT COUNT(*) FROM sys_user_role WHERE user_id = 1`).Scan(&n).Error; err != nil {
		t.Fatalf("count: %v", err)
	}
	if n != 1 {
		t.Fatalf("holding rows of deleted roles should be deleted too, %d left", n)
	}
}

// TestUserRepoEditResolverSeesCurrentRoles: the callback gets the current roles read in the transaction (deleted roles
// excluded); when it decides nothing changed, roles are not written.
func TestUserRepoEditResolverSeesCurrentRoles(t *testing.T) {
	db := userEditTestDB(t)
	repo := NewUserAdminRepo(&coredata.Data{DB: db})
	ctx := context.Background()

	var seen []int64
	err := repo.Edit(ctx, 1, biz.UserEditChanges{Roles: func(current []int64) ([]int64, bool, error) {
		seen = current
		return nil, false, nil
	}})
	if err != nil {
		t.Fatalf("Edit: %v", err)
	}
	if len(seen) != 1 || seen[0] != 5 {
		t.Fatalf("callback should get {5}, got %v", seen)
	}
	var n int64
	if err := db.Raw(`SELECT COUNT(*) FROM sys_user_role WHERE user_id = 1`).Scan(&n).Error; err != nil {
		t.Fatalf("count: %v", err)
	}
	if n != 2 {
		t.Fatalf("roles must not be written when unchanged, %d rows left", n)
	}
}

// TestUserRepoEditResolverErrorRollsBack: when the callback rejects, the whole edit rolls back, and the nickname in the
// same submit is not stored either.
func TestUserRepoEditResolverErrorRollsBack(t *testing.T) {
	db := userEditTestDB(t)
	repo := NewUserAdminRepo(&coredata.Data{DB: db})
	ctx := context.Background()

	nickname := "never"
	err := repo.Edit(ctx, 1, biz.UserEditChanges{
		Nickname: &nickname,
		Roles:    func([]int64) ([]int64, bool, error) { return nil, false, biz.ErrRoleRequired },
	})
	if !errors.Is(err, biz.ErrRoleRequired) {
		t.Fatalf("Edit error = %v, want ErrRoleRequired", err)
	}
	u, err := repo.FindByID(ctx, 1)
	if err != nil {
		t.Fatalf("FindByID: %v", err)
	}
	if u.Nickname != "old-nick" {
		t.Fatalf("nickname still changed to %q after the callback rejected", u.Nickname)
	}
}

// TestUserRepoEditMissingUser: a missing user is detected inside the lock, with the same error FindByID returns.
func TestUserRepoEditMissingUser(t *testing.T) {
	db := userEditTestDB(t)
	repo := NewUserAdminRepo(&coredata.Data{DB: db})

	nickname := "ghost"
	if err := repo.Edit(context.Background(), 404, biz.UserEditChanges{Nickname: &nickname}); !errors.Is(err, biz.ErrInvalidCredential) {
		t.Fatalf("Edit error = %v, want ErrInvalidCredential", err)
	}
}

// TestUserRepoEditMultipleFieldsOneTransaction: several fields plus roles are written together in one Edit and all take
// effect, verifying it does not rely on separate statements that happen to all succeed.
func TestUserRepoEditMultipleFieldsOneTransaction(t *testing.T) {
	db := userEditTestDB(t)
	repo := NewUserAdminRepo(&coredata.Data{DB: db})
	ctx := context.Background()

	nickname, ip := "combo", "10.0.0.0/8"
	deptID := int64(2)
	if err := repo.Edit(ctx, 1, biz.UserEditChanges{
		Nickname: &nickname, DeptID: &deptID, IPWhitelist: &ip,
		Roles: replaceRoles(5, 6),
	}); err != nil {
		t.Fatalf("Edit: %v", err)
	}

	u, err := repo.FindByID(ctx, 1)
	if err != nil {
		t.Fatalf("FindByID: %v", err)
	}
	if u.Nickname != "combo" || u.DeptID != 2 || u.IPWhitelist != "10.0.0.0/8" {
		t.Fatalf("not all fields took effect: %+v", u)
	}
	roleIDs, err := repo.RoleIDs(ctx, 1)
	if err != nil {
		t.Fatalf("RoleIDs: %v", err)
	}
	if len(roleIDs) != 2 {
		t.Fatalf("roles should be replaced with {5,6}, got %v", roleIDs)
	}
}

// TestUserRepoContactRoundTrip: email and phone number can be written, read back and cleared; clearing relies on the
// map form, since GORM skips a struct's zero values. Both write paths, editing and self-service profile, must hold.
func TestUserRepoContactRoundTrip(t *testing.T) {
	db := userEditTestDB(t)
	repo := NewUserAdminRepo(&coredata.Data{DB: db})
	ctx := context.Background()

	email, phone := "alice@corp.io", "+86 138-0000-1111"
	if err := repo.Edit(ctx, 1, biz.UserEditChanges{Email: &email, Phone: &phone}); err != nil {
		t.Fatalf("Edit: %v", err)
	}
	u, err := repo.FindByID(ctx, 1)
	if err != nil {
		t.Fatalf("FindByID: %v", err)
	}
	if u.Email != email || u.Phone != phone {
		t.Errorf("after edit email=%q phone=%q", u.Email, u.Phone)
	}

	if err := repo.SetProfile(ctx, 1, biz.ProfileEdit{Nickname: "n"}); err != nil {
		t.Fatalf("SetProfile: %v", err)
	}
	u, err = repo.FindByID(ctx, 1)
	if err != nil {
		t.Fatalf("FindByID: %v", err)
	}
	if u.Email != "" || u.Phone != "" {
		t.Errorf("values remain after self-clearing email=%q phone=%q", u.Email, u.Phone)
	}
}

// TestUserRepoEditRejectsDisabledDept: a disabled department cannot become a user's department, and the edit writes
// nothing.
func TestUserRepoEditRejectsDisabledDept(t *testing.T) {
	db := userEditTestDB(t)
	repo := NewUserAdminRepo(&coredata.Data{DB: db})
	ctx := context.Background()

	nickname, deptID := "changed", int64(3)
	err := repo.Edit(ctx, 1, biz.UserEditChanges{Nickname: &nickname, DeptID: &deptID})
	if !errors.Is(err, biz.ErrDeptDisabled) {
		t.Fatalf("err = %v, want ErrDeptDisabled", err)
	}
	u, err := repo.FindByID(ctx, 1)
	if err != nil {
		t.Fatal(err)
	}
	if u.DeptID != 1 || u.Nickname != "old-nick" {
		t.Fatalf("user = %+v, want unchanged", u)
	}
}
