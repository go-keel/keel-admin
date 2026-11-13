package data

import (
	"context"
	"testing"

	"gorm.io/gorm"

	coredata "github.com/go-keel/keel-admin/internal/core/data"
)

// RolesOfUsers gets the roles of a whole page of users in one query: grouped by user, ascending by role ID within a
// group, deleted roles absent, and users with no role not in the result; RolesOf shares this query.
func TestUserRepo_RolesOfUsers(t *testing.T) {
	db := openMemoryDB(t, &gorm.Config{})
	for _, stmt := range []string{
		`CREATE TABLE sys_role (
			id INTEGER PRIMARY KEY, code TEXT, name TEXT, sensitive INTEGER,
			force_otp INTEGER, status INTEGER, created_at DATETIME,
			updated_at DATETIME, deleted_at DATETIME
		)`,
		`CREATE TABLE sys_user_role (user_id INTEGER, role_id INTEGER)`,
		`INSERT INTO sys_role (id, code, name, force_otp, status) VALUES (3, 'r3', 'R3', 1, 1)`,
		`INSERT INTO sys_role (id, code, name, force_otp, status) VALUES (7, 'r7', 'R7', 0, 1)`,
		`INSERT INTO sys_role (id, code, name, force_otp, status, deleted_at) VALUES (9, 'r9', 'R9', 0, 1, '2026-01-01 00:00:00')`,
		`INSERT INTO sys_user_role (user_id, role_id) VALUES (1, 7), (1, 3), (2, 7), (2, 9), (3, 9)`,
	} {
		if err := db.Exec(stmt).Error; err != nil {
			t.Fatalf("setup: %v", err)
		}
	}
	repo := NewUserAdminRepo(&coredata.Data{DB: db})
	ctx := context.Background()

	got, err := repo.RolesOfUsers(ctx, []int64{1, 2, 3, 4})
	if err != nil {
		t.Fatalf("RolesOfUsers: %v", err)
	}
	if len(got) != 2 {
		t.Fatalf("want users 1 and 2 only, got %v", got)
	}
	if u1 := got[1]; len(u1) != 2 || u1[0].ID != 3 || u1[0].Name != "R3" || u1[1].ID != 7 {
		t.Fatalf("user 1 roles = %+v", u1)
	}
	if u2 := got[2]; len(u2) != 1 || u2[0].ID != 7 {
		t.Fatalf("user 2 roles = %+v (deleted role 9 must not appear)", u2)
	}

	if empty, err := repo.RolesOfUsers(ctx, nil); err != nil || len(empty) != 0 {
		t.Fatalf("no ids: got %v err %v", empty, err)
	}
	if one, err := repo.RolesOf(ctx, 1); err != nil || len(one) != 2 || one[0].ID != 3 {
		t.Fatalf("RolesOf(1) = %+v err %v", one, err)
	}
}

// A forced authenticator counts only active roles: a user holding a disabled force_otp role gets no "forced but not
// bound" hint in the list and is not forced at login either; both sides use the same definition. Before the fix the
// list counted held roles, disabled ones included, so it showed the hint while login did not force.
func TestForceOTPUsersCountsActiveRolesOnly(t *testing.T) {
	db := openMemoryDB(t, &gorm.Config{})
	for _, stmt := range []string{
		`CREATE TABLE sys_role (
			id INTEGER PRIMARY KEY, code TEXT, name TEXT, sensitive INTEGER,
			force_otp INTEGER, status INTEGER, created_at DATETIME,
			updated_at DATETIME, deleted_at DATETIME
		)`,
		`CREATE TABLE sys_user_role (user_id INTEGER, role_id INTEGER)`,
		`INSERT INTO sys_role (id, code, name, force_otp, status) VALUES (3, 'r3', 'R3', 1, 1)`,
		`INSERT INTO sys_role (id, code, name, force_otp, status) VALUES (4, 'r4', 'R4', 1, 0)`,
		`INSERT INTO sys_role (id, code, name, force_otp, status) VALUES (7, 'r7', 'R7', 0, 1)`,
		`INSERT INTO sys_role (id, code, name, force_otp, status, deleted_at) VALUES (9, 'r9', 'R9', 1, 1, '2026-01-01 00:00:00')`,
		// 1: active forcing role; 2: only a disabled forcing role; 3: only a deleted forcing role; 5: a non-forcing role.
		`INSERT INTO sys_user_role (user_id, role_id) VALUES (1, 3), (1, 7), (2, 4), (2, 7), (3, 9), (5, 7)`,
	} {
		if err := db.Exec(stmt).Error; err != nil {
			t.Fatalf("setup: %v", err)
		}
	}
	d := &coredata.Data{DB: db}
	users, otp := NewUserAdminRepo(d), NewOTPRepo(d)
	ctx := context.Background()

	forced, err := users.ForceOTPUsers(ctx, []int64{1, 2, 3, 5})
	if err != nil {
		t.Fatalf("ForceOTPUsers: %v", err)
	}
	if len(forced) != 1 || !forced[1] {
		t.Fatalf("forced = %v, want only user 1", forced)
	}
	for _, id := range []int64{1, 2, 3, 5} {
		login, err := otp.UserForceOTP(ctx, id)
		if err != nil {
			t.Fatalf("UserForceOTP(%d): %v", id, err)
		}
		if login != forced[id] {
			t.Errorf("user %d: list says forced=%v, login says %v", id, forced[id], login)
		}
	}
	// A disabled role is still held: the role tag shows it as usual.
	if held, err := users.RolesOf(ctx, 2); err != nil || len(held) != 2 {
		t.Fatalf("RolesOf(2) = %+v err %v, want the disabled role kept", held, err)
	}
}
