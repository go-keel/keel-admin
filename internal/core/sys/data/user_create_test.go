package data

import (
	"context"
	"errors"
	"testing"

	"gorm.io/gorm"
	"gorm.io/gorm/logger"

	coredata "github.com/go-keel/keel-admin/internal/core/data"
	"github.com/go-keel/keel-admin/internal/core/sys/biz"
)

// userCreateTestDB recreates the two tables account creation writes, with unique and primary keys copied from the
// admin complete schema. TranslateError matches how kdb opens the database, so collisions become
// gorm.ErrDuplicatedKey.
func userCreateTestDB(t *testing.T) *gorm.DB {
	t.Helper()
	db := openMemoryDB(t, &gorm.Config{Logger: logger.Discard, TranslateError: true})
	for _, stmt := range []string{
		`CREATE TABLE sys_user (
			id INTEGER PRIMARY KEY AUTOINCREMENT, username TEXT NOT NULL UNIQUE, password TEXT NOT NULL,
			nickname TEXT, email TEXT DEFAULT '', phone TEXT DEFAULT '',
			dept_id INTEGER, is_root INTEGER, status INTEGER, otp_secret_enc BLOB,
			otp_bound INTEGER, otp_bound_at DATETIME, token_version INTEGER,
			avatar TEXT, ip_whitelist TEXT, settings TEXT, created_at DATETIME,
			updated_at DATETIME, deleted_at DATETIME
		)`,
		`CREATE TABLE sys_user_role (user_id INTEGER NOT NULL, role_id INTEGER NOT NULL, PRIMARY KEY (user_id, role_id))`,
	} {
		if err := db.Exec(stmt).Error; err != nil {
			t.Fatalf("create table: %v", err)
		}
	}
	return db
}

// Creating an account that collides on uk_username returns ErrUserExists: biz's pre-check cannot stop concurrent
// creates of the same name, so after the unique key decides, it must become a domain error, not a 500. A role given
// twice collides on sys_user_role's primary key and must not be taken for a duplicate username.
func TestUserCreateDuplicateUsername(t *testing.T) {
	db := userCreateTestDB(t)
	repo := NewUserAdminRepo(&coredata.Data{DB: db})
	ctx := context.Background()

	if err := repo.Create(ctx, &biz.User{Username: "alice", Password: "h", Status: 1}, []int64{5}); err != nil {
		t.Fatalf("Create: %v", err)
	}
	if err := repo.Create(ctx, &biz.User{Username: "alice", Password: "h", Status: 1}, []int64{5}); !errors.Is(err, biz.ErrUserExists) {
		t.Fatalf("create with same name = %v, want ErrUserExists", err)
	}
	err := repo.Create(ctx, &biz.User{Username: "bob", Password: "h", Status: 1}, []int64{5, 5})
	if err == nil || errors.Is(err, biz.ErrUserExists) {
		t.Fatalf("role given twice = %v, want an error other than ErrUserExists", err)
	}
}

// The IP allowlist at account creation is written in the same insert as the account's main row.
func TestUserCreateStoresIPWhitelist(t *testing.T) {
	db := userCreateTestDB(t)
	repo := NewUserAdminRepo(&coredata.Data{DB: db})
	u := &biz.User{Username: "carol", Password: "h", Status: 1, IPWhitelist: "10.0.0.0/8"}
	if err := repo.Create(context.Background(), u, []int64{5}); err != nil {
		t.Fatalf("Create: %v", err)
	}
	var got string
	if err := db.Raw(`SELECT ip_whitelist FROM sys_user WHERE id = ?`, u.ID).Scan(&got).Error; err != nil || got != "10.0.0.0/8" {
		t.Fatalf("ip_whitelist = %q err %v", got, err)
	}
}

// UpdateAccount writes in one statement: the set columns and token_version + 1 in the same UPDATE; without
// invalidation the version stays; with nothing set no statement is sent. Written in two steps, a failed second step
// would leave the password changed while old sessions live on.
func TestUserUpdateAccountOneStatement(t *testing.T) {
	db := userCreateTestDB(t)
	if err := db.Exec(`INSERT INTO sys_user (id, username, password, status, is_root, token_version)
		VALUES (1, 'alice', 'old', 1, 0, 5)`).Error; err != nil {
		t.Fatalf("seed: %v", err)
	}
	updates := 0
	if err := db.Callback().Update().After("gorm:update").Register("count", func(*gorm.DB) { updates++ }); err != nil {
		t.Fatalf("callback: %v", err)
	}
	repo := NewUserAdminRepo(&coredata.Data{DB: db})
	ctx := context.Background()
	read := func() (pw string, status, isRoot, ver int) {
		row := db.Raw(`SELECT password, status, is_root, token_version FROM sys_user WHERE id = 1`).Row()
		if err := row.Scan(&pw, &status, &isRoot, &ver); err != nil {
			t.Fatalf("read: %v", err)
		}
		return
	}
	str := func(s string) *string { return &s }
	i8 := func(v int8) *int8 { return &v }

	if err := repo.UpdateAccount(ctx, 1, biz.AccountChange{PasswordHash: str("new"), RevokeSessions: true}); err != nil {
		t.Fatalf("UpdateAccount: %v", err)
	}
	if pw, _, _, ver := read(); pw != "new" || ver != 6 || updates != 1 {
		t.Fatalf("password+revoke: pw=%q ver=%d updates=%d, want new/6/1", pw, ver, updates)
	}

	if err := repo.UpdateAccount(ctx, 1, biz.AccountChange{Status: i8(0)}); err != nil {
		t.Fatalf("UpdateAccount: %v", err)
	}
	if _, status, _, ver := read(); status != 0 || ver != 6 {
		t.Fatalf("status only: status=%d ver=%d, want 0/6 (no revoke)", status, ver)
	}

	root := true
	if err := repo.UpdateAccount(ctx, 1, biz.AccountChange{IsRoot: &root, RevokeSessions: true}); err != nil {
		t.Fatalf("UpdateAccount: %v", err)
	}
	if pw, _, isRoot, ver := read(); isRoot != 1 || ver != 7 || pw != "new" {
		t.Fatalf("root+revoke: isRoot=%d ver=%d pw=%q", isRoot, ver, pw)
	}

	before := updates
	if err := repo.UpdateAccount(ctx, 1, biz.AccountChange{}); err != nil || updates != before {
		t.Fatalf("empty change should not write: err=%v updates %d→%d", err, before, updates)
	}
}
