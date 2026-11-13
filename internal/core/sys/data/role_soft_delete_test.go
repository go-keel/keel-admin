package data

import (
	"context"
	"testing"

	"gorm.io/gorm"

	coredata "github.com/go-keel/keel-admin/internal/core/data"
)

// roleSoftDeleteTestDB creates the minimal role/association schema. The main
// row is soft-deleted; associations deliberately remain as historical
// configuration and every runtime query must anchor itself to active sys_role.
func roleSoftDeleteTestDB(t *testing.T) *gorm.DB {
	t.Helper()
	db := openMemoryDB(t, &gorm.Config{})
	ddl := []string{
		`CREATE TABLE sys_role (
			id INTEGER PRIMARY KEY, code TEXT, name TEXT, sensitive INTEGER,
			force_otp INTEGER, status INTEGER, created_at DATETIME,
			updated_at DATETIME, deleted_at DATETIME
		)`,
		`CREATE TABLE sys_user_role (user_id INTEGER, role_id INTEGER)`,
		`CREATE TABLE sys_role_permission (role_id INTEGER, perm_code TEXT)`,
		`CREATE TABLE sys_role_data_scope (
			id INTEGER PRIMARY KEY, role_id INTEGER, resource_type TEXT, scope_type TEXT,
			created_at DATETIME
		)`,
		`CREATE TABLE sys_role_resource_grant (role_id INTEGER, resource_type TEXT, resource_id INTEGER)`,
		`CREATE TABLE sys_role_manage_dept (role_id INTEGER, dept_id INTEGER)`,
		`CREATE TABLE sys_role_assignable (role_id INTEGER, assignable_role_id INTEGER)`,
	}
	for _, stmt := range ddl {
		if err := db.Exec(stmt).Error; err != nil {
			t.Fatalf("create table: %v", err)
		}
	}

	// Role 5: one row in every association table. Soft delete must retain them.
	seed := []string{
		`INSERT INTO sys_role (id, code, name, sensitive, force_otp, status) VALUES (5, 'r5', 'R5', 0, 1, 1)`,
		`INSERT INTO sys_user_role (user_id, role_id) VALUES (1, 5)`,
		`INSERT INTO sys_role_permission (role_id, perm_code) VALUES (5, 'p:read')`,
		`INSERT INTO sys_role_data_scope (id, role_id, resource_type, scope_type) VALUES (1, 5, 'order', 'ALL')`,
		`INSERT INTO sys_role_resource_grant (role_id, resource_type, resource_id) VALUES (5, 'notice_category', 3)`,
		`INSERT INTO sys_role_manage_dept (role_id, dept_id) VALUES (5, 100)`,
		`INSERT INTO sys_role_assignable (role_id, assignable_role_id) VALUES (5, 6)`,
		// Role 9 references role 5 as assignable. The row remains for history,
		// but active ARBAC queries must not expose deleted target 5.
		`INSERT INTO sys_role (id, code, name, sensitive, force_otp, status) VALUES (9, 'r9', 'R9', 0, 0, 1)`,
		`INSERT INTO sys_role_assignable (role_id, assignable_role_id) VALUES (9, 5)`,
		`INSERT INTO sys_user_role (user_id, role_id) VALUES (2, 9)`,
		`INSERT INTO sys_role_permission (role_id, perm_code) VALUES (9, 'p:write')`,
	}
	for _, stmt := range seed {
		if err := db.Exec(stmt).Error; err != nil {
			t.Fatalf("seed: %v", err)
		}
	}
	return db
}

func TestRoleDeleteSoftDeletesAndRevokesAtReadBoundary(t *testing.T) {
	db := roleSoftDeleteTestDB(t)
	repo := NewRoleRepo(&coredata.Data{DB: db})

	if err := repo.Delete(context.Background(), 5); err != nil {
		t.Fatalf("Delete: %v", err)
	}

	var deleted sysRole
	if err := db.Unscoped().Where("id = ?", 5).First(&deleted).Error; err != nil {
		t.Fatalf("read tombstone: %v", err)
	}
	if !deleted.DeletedAt.Valid {
		t.Fatal("sys_role 5 should be retained with deleted_at")
	}

	// Every association survives. These rows are historical configuration,
	// not active authority; active reads below must filter through sys_role.
	for _, q := range []string{
		"SELECT COUNT(*) FROM sys_user_role WHERE role_id=5",
		"SELECT COUNT(*) FROM sys_role_permission WHERE role_id=5",
		"SELECT COUNT(*) FROM sys_role_data_scope WHERE role_id=5",
		"SELECT COUNT(*) FROM sys_role_resource_grant WHERE role_id=5",
		"SELECT COUNT(*) FROM sys_role_manage_dept WHERE role_id=5",
		"SELECT COUNT(*) FROM sys_role_assignable WHERE role_id=5",
	} {
		var n int64
		if err := db.Raw(q).Scan(&n).Error; err != nil {
			t.Fatalf("query %q: %v", q, err)
		}
		if n != 1 {
			t.Fatalf("%s = %d, want retained 1", q, n)
		}
	}

	var active int64
	if err := db.Model(&sysRole{}).Where("id = ?", 5).Count(&active).Error; err != nil {
		t.Fatalf("active role count: %v", err)
	}
	if active != 0 {
		t.Fatalf("soft-deleted role remains active: %d", active)
	}

	userRepo := NewUserAdminRepo(&coredata.Data{DB: db})
	if ids, err := userRepo.RoleIDs(context.Background(), 1); err != nil || len(ids) != 0 {
		t.Fatalf("deleted role still effective for user 1: ids=%v err=%v", ids, err)
	}
	if ids, err := userRepo.RoleIDs(context.Background(), 2); err != nil || len(ids) != 1 || ids[0] != 9 {
		t.Fatalf("active role 9 should survive: ids=%v err=%v", ids, err)
	}
	if codes, err := userRepo.RolePermCodesByRole(context.Background(), 5); err != nil || len(codes) != 0 {
		t.Fatalf("deleted role permissions still active: codes=%v err=%v", codes, err)
	}
	if roles, err := userRepo.RolesOf(context.Background(), 1); err != nil || len(roles) != 0 {
		t.Fatalf("deleted role still visible on user: roles=%v err=%v", roles, err)
	}
	otpRepo := NewOTPRepo(&coredata.Data{DB: db})
	if forced, err := otpRepo.UserForceOTP(context.Background(), 1); err != nil || forced {
		t.Fatalf("deleted role still forces OTP: forced=%v err=%v", forced, err)
	}

	arbacRepo := NewARBACRepo(&coredata.Data{DB: db})
	if ids, err := arbacRepo.AssignableRoleIDs(context.Background(), []int64{9}); err != nil || len(ids) != 0 {
		t.Fatalf("deleted assignable target still active: ids=%v err=%v", ids, err)
	}

	scopeRepo := NewScopeRepo(&coredata.Data{DB: db})
	if scopes, err := scopeRepo.RoleScopes(context.Background(), []int64{5}, "order"); err != nil || len(scopes) != 0 {
		t.Fatalf("deleted role scope still active: scopes=%v err=%v", scopes, err)
	}
}
