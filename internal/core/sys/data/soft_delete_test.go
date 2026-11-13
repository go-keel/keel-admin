package data

import (
	"context"
	"errors"
	"testing"

	"gorm.io/gorm"

	coredata "github.com/go-keel/keel-admin/internal/core/data"
	"github.com/go-keel/keel-admin/internal/core/sys/biz"
)

func masterDataSoftDeleteDB(t *testing.T) *gorm.DB {
	t.Helper()
	db := openMemoryDB(t, &gorm.Config{})
	ddl := []string{
		`CREATE TABLE sys_dept (
			id INTEGER PRIMARY KEY, parent_id INTEGER, ancestors TEXT, name TEXT,
			sort INTEGER, status INTEGER, created_at DATETIME, updated_at DATETIME,
			deleted_at DATETIME
		)`,
		`CREATE TABLE sys_user (
			id INTEGER PRIMARY KEY, username TEXT, password TEXT, nickname TEXT,
			email TEXT DEFAULT '', phone TEXT DEFAULT '',
			dept_id INTEGER, is_root INTEGER, status INTEGER, otp_secret_enc BLOB,
			otp_bound INTEGER, otp_bound_at DATETIME, token_version INTEGER,
			avatar TEXT, ip_whitelist TEXT, settings TEXT, created_at DATETIME,
			updated_at DATETIME, deleted_at DATETIME
		)`,
	}
	for _, stmt := range ddl {
		if err := db.Exec(stmt).Error; err != nil {
			t.Fatalf("create table: %v", err)
		}
	}
	return db
}

func TestDeptDeleteIsSoftDelete(t *testing.T) {
	db := masterDataSoftDeleteDB(t)
	if err := db.Create(&sysDept{ID: 1, Name: "root", Status: 1}).Error; err != nil {
		t.Fatalf("seed dept: %v", err)
	}
	repo := NewDeptRepo(&coredata.Data{DB: db})
	if err := repo.Delete(context.Background(), 1); err != nil {
		t.Fatalf("Delete: %v", err)
	}

	if _, err := repo.Get(context.Background(), 1); !errors.Is(err, biz.ErrDeptNotFound) {
		t.Fatalf("active Get after soft delete = %v, want ErrDeptNotFound", err)
	}
	var row sysDept
	if err := db.Unscoped().Where("id = ?", 1).First(&row).Error; err != nil {
		t.Fatalf("unscoped Get: %v", err)
	}
	if !row.DeletedAt.Valid {
		t.Fatal("sys_dept row was physically removed")
	}
}

func TestUserModelHasDatabaseSoftDeleteWithoutDeleteOperation(t *testing.T) {
	db := masterDataSoftDeleteDB(t)
	if err := db.Create(&sysUser{
		ID: 1, Username: "alice", Password: "hash", DeptID: 1, Status: 1,
	}).Error; err != nil {
		t.Fatalf("seed user: %v", err)
	}

	// The user domain has no Delete method; this goes through GORM directly to verify only the PO and database
	// soft-delete support.
	if err := db.Where("id = ?", 1).Delete(&sysUser{}).Error; err != nil {
		t.Fatalf("soft delete user model: %v", err)
	}
	repo := NewUserAdminRepo(&coredata.Data{DB: db})
	if _, err := repo.FindByID(context.Background(), 1); !errors.Is(err, biz.ErrInvalidCredential) {
		t.Fatalf("active FindByID after soft delete = %v, want ErrInvalidCredential", err)
	}
	var row sysUser
	if err := db.Unscoped().Where("id = ?", 1).First(&row).Error; err != nil {
		t.Fatalf("unscoped Get: %v", err)
	}
	if !row.DeletedAt.Valid {
		t.Fatal("sys_user row was physically removed")
	}
}
