package data

import (
	"context"
	"errors"
	"fmt"
	"slices"
	"testing"

	coredata "github.com/go-keel/keel-admin/internal/core/data"
	"github.com/go-keel/keel-admin/internal/core/sys/biz"
)

// A disabled role grants nothing: the grant queries for permission codes, data scope, resource grants, assignable
// roles, manageable departments and forced authenticator (both the login and the user list entry) all check
// status = 1; the holding relation (RoleIDs) keeps disabled roles, which the pin when editing a user relies on. It first
// asserts every query returns something while enabled; otherwise "empty after disabling" proves nothing.
func TestDisabledRoleGrantsNothing(t *testing.T) {
	db := roleSoftDeleteTestDB(t)
	for _, stmt := range []string{
		`CREATE TABLE sys_dept (id INTEGER PRIMARY KEY, parent_id INTEGER, ancestors TEXT, name TEXT, sort INTEGER, status INTEGER, created_at DATETIME, updated_at DATETIME, deleted_at DATETIME)`,
		`INSERT INTO sys_dept (id, parent_id, ancestors, name, sort, status) VALUES (100, 0, '0,', 'D100', 0, 1)`,
		`INSERT INTO sys_role (id, code, name, sensitive, force_otp, status) VALUES (6, 'r6', 'R6', 0, 0, 1)`,
	} {
		if err := db.Exec(stmt).Error; err != nil {
			t.Fatalf("seed: %v", err)
		}
	}
	ctx := context.Background()
	d := &coredata.Data{DB: db}
	users, scopes, arbac, otp := NewUserAdminRepo(d), NewScopeRepo(d), NewARBACRepo(d), NewOTPRepo(d)

	grants := func() map[string]int {
		codes, err1 := users.RolePermCodesByRole(ctx, 5)
		sc, err2 := scopes.RoleScopes(ctx, []int64{5}, "order")
		res, err3 := scopes.RoleGrantedResourceIDs(ctx, []int64{5}, "notice_category")
		asg, err4 := arbac.AssignableRoleIDs(ctx, []int64{5})
		depts, err5 := arbac.ManageDeptIDs(ctx, []int64{5})
		forced, err6 := otp.UserForceOTP(ctx, 1)
		listForced, err7 := users.ForceOTPUsers(ctx, []int64{1})
		for _, err := range []error{err1, err2, err3, err4, err5, err6, err7} {
			if err != nil {
				t.Fatalf("query: %v", err)
			}
		}
		n := 0
		if forced {
			n = 1
		}
		return map[string]int{"perm": len(codes), "scope": len(sc), "grant": len(res), "assignable": len(asg), "manageDept": len(depts), "forceOTP": n, "forceOTPList": len(listForced)}
	}

	for name, n := range grants() {
		if n == 0 {
			t.Fatalf("enabled role 5 grants no %s; seed does not exercise the query", name)
		}
	}
	if err := db.Exec(`UPDATE sys_role SET status = 0 WHERE id = 5`).Error; err != nil {
		t.Fatalf("disable role: %v", err)
	}
	for name, n := range grants() {
		if n != 0 {
			t.Errorf("disabled role 5 still grants %s (%d)", name, n)
		}
	}
	if ids, err := users.RoleIDs(ctx, 1); err != nil || len(ids) != 1 || ids[0] != 5 {
		t.Fatalf("RoleIDs must keep the disabled role for pinning: ids=%v err=%v", ids, err)
	}
}

// An assigned role that is disabled cannot be assigned either; when editing a user it is outside the operator's range
// and is kept as is by the pin. Before the fix the assignable query checked only that the granting role was enabled:
// disabled 5 was still in 9's assignable range but missing from the dropdown, so a non-root edit of a user holding 5
// submitted without it, saving stripped 5, and re-enabling did not bring it back.
func TestDisabledTargetRoleNotAssignableAndKeptOnEdit(t *testing.T) {
	db := roleSoftDeleteTestDB(t) // 9 enabled and may assign 5; 5 enabled.
	for _, stmt := range []string{
		`CREATE TABLE sys_dept (id INTEGER PRIMARY KEY, parent_id INTEGER, ancestors TEXT, name TEXT, sort INTEGER, status INTEGER, created_at DATETIME, updated_at DATETIME, deleted_at DATETIME)`,
		`INSERT INTO sys_dept (id, parent_id, ancestors, name, sort, status) VALUES (100, 0, '0,', 'D100', 0, 1)`,
		`CREATE TABLE sys_user (
			id INTEGER PRIMARY KEY, username TEXT, password TEXT, nickname TEXT DEFAULT '', email TEXT DEFAULT '',
			phone TEXT DEFAULT '', dept_id INTEGER, is_root INTEGER DEFAULT 0, status INTEGER DEFAULT 1,
			otp_secret_enc BLOB, otp_bound INTEGER DEFAULT 0, otp_bound_at DATETIME, token_version INTEGER DEFAULT 0,
			avatar TEXT DEFAULT '', ip_whitelist TEXT DEFAULT '', settings TEXT DEFAULT '',
			created_at DATETIME, updated_at DATETIME, deleted_at DATETIME
		)`,
		`INSERT INTO sys_role (id, code, name, sensitive, force_otp, status) VALUES (6, 'r6', 'R6', 0, 0, 1)`,
		`INSERT INTO sys_role_assignable (role_id, assignable_role_id) VALUES (9, 6)`,
		`INSERT INTO sys_role_manage_dept (role_id, dept_id) VALUES (9, 100)`,
		`INSERT INTO sys_user (id, username, nickname, dept_id) VALUES (3, 'bob', 'bob', 100)`,
		`INSERT INTO sys_user_role (user_id, role_id) VALUES (3, 5), (3, 6)`,
	} {
		if err := db.Exec(stmt).Error; err != nil {
			t.Fatalf("seed: %v", err)
		}
	}
	ctx := context.Background()
	d := &coredata.Data{DB: db}
	users, arbac := NewUserAdminRepo(d), NewARBACRepo(d)

	assignable := func() []int64 {
		ids, err := arbac.AssignableRoleIDs(ctx, []int64{9})
		if err != nil {
			t.Fatalf("AssignableRoleIDs: %v", err)
		}
		slices.Sort(ids)
		return ids
	}
	if got := assignable(); !slices.Equal(got, []int64{5, 6}) {
		t.Fatalf("enabled: assignable = %v, want [5 6]", got)
	}
	if err := db.Exec(`UPDATE sys_role SET status = 0 WHERE id = 5`).Error; err != nil {
		t.Fatalf("disable role: %v", err)
	}
	if got := assignable(); !slices.Equal(got, []int64{6}) {
		t.Fatalf("disabled target still assignable: %v", got)
	}

	uc := biz.NewUserAdminUseCase(users, biz.NewARBACUseCase(arbac, users), nil)
	actor := &biz.Actor{UserID: -1, RoleIDs: []int64{9}}
	if _, err := uc.Edit(ctx, actor, 3, biz.UserEdit{Nickname: "bob", DeptID: 100, RoleIDs: []int64{6}}); err != nil {
		t.Fatalf("edit without the disabled role: %v", err)
	}
	got, err := users.RoleIDs(ctx, 3)
	if err != nil {
		t.Fatalf("RoleIDs: %v", err)
	}
	slices.Sort(got)
	if !slices.Equal(got, []int64{5, 6}) {
		t.Fatalf("disabled role was stripped on edit: roles = %v, want [5 6]", got)
	}
	if _, err := uc.Edit(ctx, actor, 3, biz.UserEdit{Nickname: "bob", DeptID: 100, RoleIDs: []int64{5, 6}}); !errors.Is(err, biz.ErrRoleOutOfRange) {
		t.Fatalf("assigning the disabled role: err = %v, want ErrRoleOutOfRange", err)
	}
}

// Create stores the status as passed. The column default matches the table script (DEFAULT 1): if GORM treated the
// zero value as "not set" and left it to the column default, creating a disabled role would silently become enabled.
func TestCreateRoleKeepsStatus(t *testing.T) {
	db := roleSaveTestDB(t) // sys_role.status has DEFAULT 1, matching the table script.
	repo := NewRoleRepo(&coredata.Data{DB: db})
	for _, status := range []int8{0, 1} {
		role := &biz.Role{Code: fmt.Sprintf("s%d", status), Name: "R", Status: status}
		if err := repo.Save(context.Background(), role); err != nil {
			t.Fatalf("Save: %v", err)
		}
		var got int8
		if err := db.Raw(`SELECT status FROM sys_role WHERE id = ?`, role.ID).Scan(&got).Error; err != nil {
			t.Fatalf("read back: %v", err)
		}
		if got != status {
			t.Errorf("created with status %d, stored %d", status, got)
		}
	}
}
