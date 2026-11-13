package data

import (
	"context"
	"errors"
	"fmt"
	"slices"
	"testing"

	"gorm.io/gorm"
	"gorm.io/gorm/logger"

	coredata "github.com/go-keel/keel-admin/internal/core/data"
	"github.com/go-keel/keel-admin/internal/core/sys/biz"
)

// roleSaveTestDB recreates the role main table and its five configuration tables, with primary and unique keys copied
// from migrations/admin/0_schema.sql: the rollback tests trigger failure by a primary-key collision, so wrong keys
// would test nothing. TranslateError matches how kdb opens the database, so collisions become gorm.ErrDuplicatedKey
// (glebarez/sqlite implements that translation).
func roleSaveTestDB(t *testing.T) *gorm.DB {
	t.Helper()
	db := openMemoryDB(t, &gorm.Config{Logger: logger.Discard, TranslateError: true})
	for _, stmt := range []string{
		`CREATE TABLE sys_role (
			id INTEGER PRIMARY KEY AUTOINCREMENT, code TEXT NOT NULL UNIQUE, name TEXT NOT NULL,
			sensitive INTEGER NOT NULL DEFAULT 0, force_otp INTEGER NOT NULL DEFAULT 0,
			status INTEGER NOT NULL DEFAULT 1, created_at DATETIME, updated_at DATETIME, deleted_at DATETIME
		)`,
		`CREATE TABLE sys_role_permission (role_id INTEGER NOT NULL, perm_code TEXT NOT NULL, PRIMARY KEY (role_id, perm_code))`,
		`CREATE TABLE sys_role_assignable (role_id INTEGER NOT NULL, assignable_role_id INTEGER NOT NULL, PRIMARY KEY (role_id, assignable_role_id))`,
		`CREATE TABLE sys_role_manage_dept (role_id INTEGER NOT NULL, dept_id INTEGER NOT NULL, PRIMARY KEY (role_id, dept_id))`,
		`CREATE TABLE sys_role_data_scope (
			id INTEGER PRIMARY KEY AUTOINCREMENT, role_id INTEGER NOT NULL, resource_type TEXT NOT NULL,
			scope_type TEXT NOT NULL, created_at DATETIME, UNIQUE (role_id, resource_type)
		)`,
		`CREATE TABLE sys_role_resource_grant (
			role_id INTEGER NOT NULL, resource_type TEXT NOT NULL, resource_id INTEGER NOT NULL,
			created_at DATETIME, PRIMARY KEY (role_id, resource_type, resource_id)
		)`,
	} {
		if err := db.Exec(stmt).Error; err != nil {
			t.Fatalf("create table: %v", err)
		}
	}
	return db
}

// roleState is everything a role has in the six tables, each item sorted so states compare directly.
type roleState struct {
	Name                        string
	Status, Sensitive, ForceOTP int8
	Perms                       []string
	Assignable, Depts           []int64
	Scopes, Grants              []string // "resource:policy", "resource:instance id"
}

// key prints the state as one line for comparison: nil and an empty slice print the same, saving per-item nil checks.
func (s roleState) key() string { return fmt.Sprintf("%+v", s) }

// readRoleState reads a role's six tables back from the database. Name is empty when the row does not exist.
func readRoleState(t *testing.T, db *gorm.DB, roleID int64) roleState {
	t.Helper()
	var s roleState
	var role sysRole
	var scopes []sysRoleDataScope
	var grants []sysRoleResourceGrant
	steps := []error{
		db.Unscoped().Where("id = ?", roleID).Limit(1).Find(&role).Error,
		db.Model(&sysRolePermission{}).Where("role_id = ?", roleID).Order("perm_code").Pluck("perm_code", &s.Perms).Error,
		db.Model(&sysRoleAssignable{}).Where("role_id = ?", roleID).Order("assignable_role_id").Pluck("assignable_role_id", &s.Assignable).Error,
		db.Model(&sysRoleManageDept{}).Where("role_id = ?", roleID).Order("dept_id").Pluck("dept_id", &s.Depts).Error,
		db.Where("role_id = ?", roleID).Order("resource_type").Find(&scopes).Error,
		db.Where("role_id = ?", roleID).Order("resource_type, resource_id").Find(&grants).Error,
	}
	for _, err := range steps {
		if err != nil {
			t.Fatalf("read role %d: %v", roleID, err)
		}
	}
	s.Name, s.Status, s.Sensitive, s.ForceOTP = role.Name, role.Status, role.Sensitive, role.ForceOTP
	for _, sc := range scopes {
		s.Scopes = append(s.Scopes, sc.ResourceType+":"+sc.ScopeType)
	}
	for _, g := range grants {
		s.Grants = append(s.Grants, fmt.Sprintf("%s:%d", g.ResourceType, g.ResourceID))
	}
	return s
}

// stateOf is what one submitted configuration should look like once stored.
func stateOf(r *biz.Role) roleState {
	s := roleState{
		Name: r.Name, Status: r.Status, Sensitive: r.Sensitive, ForceOTP: r.ForceOTP,
		Perms: slices.Sorted(slices.Values(r.Perms)), Assignable: slices.Sorted(slices.Values(r.AssignableRoleIDs)),
		Depts: slices.Sorted(slices.Values(r.ManageDepts)),
	}
	for _, sc := range r.Scopes {
		s.Scopes = append(s.Scopes, sc.ResourceType+":"+string(sc.ScopeType))
		for _, id := range sc.ResourceIDs {
			s.Grants = append(s.Grants, fmt.Sprintf("%s:%d", sc.ResourceType, id))
		}
	}
	slices.Sort(s.Scopes)
	slices.Sort(s.Grants)
	return s
}

// configA / configB are two complete configurations that differ in every table.
func configA(id int64) *biz.Role {
	return &biz.Role{
		ID: id, Code: "txrole", Name: "A", Status: 1, Sensitive: 0, ForceOTP: 0,
		Perms:             []string{"a:p1", "a:p2", "a:p3"},
		AssignableRoleIDs: []int64{1001, 1002},
		ManageDepts:       []int64{11},
		Scopes:            []biz.DataScopeInput{{ResourceType: "notice", ScopeType: biz.ScopeAll}},
	}
}

func configB(id int64) *biz.Role {
	return &biz.Role{
		ID: id, Code: "txrole", Name: "B", Status: 0, Sensitive: 1, ForceOTP: 1,
		Perms:             []string{"b:p1", "b:p2"},
		AssignableRoleIDs: []int64{2001},
		ManageDepts:       []int64{21, 22},
		Scopes: []biz.DataScopeInput{
			{ResourceType: "notice_category", ScopeType: biz.ScopeGrant, ResourceIDs: []int64{7, 8}},
			{ResourceType: "order", ScopeType: biz.ScopeSelf},
		},
	}
}

func TestRoleSaveCreatesThenReplacesEverything(t *testing.T) {
	db := roleSaveTestDB(t)
	repo := NewRoleRepo(&coredata.Data{DB: db})
	ctx := context.Background()

	a := configA(0)
	if err := repo.Save(ctx, a); err != nil {
		t.Fatalf("Save(create): %v", err)
	}
	if a.ID == 0 {
		t.Fatal("Save(create) did not fill in the ID")
	}
	if got, want := readRoleState(t, db, a.ID), stateOf(a); got.key() != want.key() {
		t.Fatalf("after create\n got %v\nwant %v", got, want)
	}

	b := configB(a.ID)
	if err := repo.Save(ctx, b); err != nil {
		t.Fatalf("Save(update): %v", err)
	}
	if got, want := readRoleState(t, db, a.ID), stateOf(b); got.key() != want.key() {
		t.Fatalf("after whole replacement\n got %v\nwant %v", got, want)
	}

	if err := repo.Save(ctx, configA(404)); err != biz.ErrRoleNotFound {
		t.Fatalf("updating a missing role: expected ErrRoleNotFound, got %v", err)
	}
}

// When two configurations overlap, only changed rows are touched: a resource whose policy changed is updated in place,
// and switching explicit assignment from an instance set to all clears the instance rows; the result is still all of
// the second configuration.
func TestRoleSaveReplacesOverlappingConfig(t *testing.T) {
	db := roleSaveTestDB(t)
	repo := NewRoleRepo(&coredata.Data{DB: db})
	ctx := context.Background()

	first := &biz.Role{
		Code: "overlap", Name: "1", Status: 1,
		Perms:             []string{"p:1", "p:2", "p:3"},
		AssignableRoleIDs: []int64{1, 2},
		ManageDepts:       []int64{10, 20},
		Scopes: []biz.DataScopeInput{
			{ResourceType: "notice_category", ScopeType: biz.ScopeGrant, ResourceIDs: []int64{7, 8}},
			{ResourceType: "order", ScopeType: biz.ScopeSelf},
			{ResourceType: "brand", ScopeType: biz.ScopeGrant, ResourceIDs: []int64{1, 2}},
		},
	}
	if err := repo.Save(ctx, first); err != nil {
		t.Fatalf("Save(create): %v", err)
	}
	second := &biz.Role{
		ID: first.ID, Code: "overlap", Name: "2", Status: 1,
		Perms:             []string{"p:2", "p:4"},
		AssignableRoleIDs: []int64{2, 3},
		ManageDepts:       []int64{20},
		Scopes: []biz.DataScopeInput{
			{ResourceType: "notice_category", ScopeType: biz.ScopeGrant, ResourceIDs: []int64{8, 9}},
			{ResourceType: "order", ScopeType: biz.ScopeAll},
			{ResourceType: "brand", ScopeType: biz.ScopeAll},
			{ResourceType: "shop", ScopeType: biz.ScopeDept},
		},
	}
	if err := repo.Save(ctx, second); err != nil {
		t.Fatalf("Save(update): %v", err)
	}
	if got, want := readRoleState(t, db, first.ID), stateOf(second); got.key() != want.key() {
		t.Fatalf("after overlapping replacement\n got %v\nwant %v", got, want)
	}
}

// DeleteGrantsByResource deletes only the grant rows of the given instance, across roles (deleted roles included), and
// leaves other instances and other resource types alone.
func TestDeleteGrantsByResource(t *testing.T) {
	db := roleSaveTestDB(t)
	for _, stmt := range []string{
		`INSERT INTO sys_role (id, code, name) VALUES (1, 'r1', 'R1'), (3, 'r3', 'R3')`,
		`INSERT INTO sys_role (id, code, name, deleted_at) VALUES (2, 'r2', 'R2', CURRENT_TIMESTAMP)`,
		`INSERT INTO sys_role_resource_grant (role_id, resource_type, resource_id) VALUES
			(1, 'nc', 5), (1, 'nc', 6), (2, 'nc', 5), (2, 'nc', 6), (3, 'other', 5)`,
	} {
		if err := db.Exec(stmt).Error; err != nil {
			t.Fatalf("seed: %v", err)
		}
	}
	if err := NewScopeRepo(&coredata.Data{DB: db}).DeleteGrantsByResource(context.Background(), "nc", 5); err != nil {
		t.Fatalf("DeleteGrantsByResource: %v", err)
	}
	var left []string
	if err := db.Raw(`SELECT role_id || ':' || resource_type || ':' || resource_id FROM sys_role_resource_grant
		ORDER BY role_id, resource_type, resource_id`).Scan(&left).Error; err != nil {
		t.Fatalf("read back: %v", err)
	}
	if got, want := fmt.Sprint(left), "[1:nc:6 2:nc:6 3:other:5]"; got != want {
		t.Fatalf("left %s, want %s", got, want)
	}
}

// A failure halfway through the configuration must roll back as a whole: two identical permission codes make the
// second table, sys_role_permission, collide on the primary key after the main row was written. Create must not leave
// half a role, and edit must not leave "name changed, configuration not".
func TestRoleSaveRollsBackWhenConfigFailsHalfway(t *testing.T) {
	db := roleSaveTestDB(t)
	repo := NewRoleRepo(&coredata.Data{DB: db})
	ctx := context.Background()

	bad := configA(0)
	bad.Code = "half"
	bad.Perms = []string{"dup", "dup"}
	if err := repo.Save(ctx, bad); err == nil {
		t.Fatal("Save(create) collided on the primary key but returned no error")
	} else if errors.Is(err, biz.ErrDuplicateRole) {
		t.Fatal("a configuration-table collision was reported as a duplicate role code: only the main-row insert maps collisions")
	}
	var n int64
	if err := db.Raw(`SELECT COUNT(*) FROM sys_role WHERE code = 'half'`).Scan(&n).Error; err != nil {
		t.Fatalf("count: %v", err)
	}
	if n != 0 {
		t.Fatalf("failed create left %d rows of a half-written role", n)
	}

	good := configA(0)
	if err := repo.Save(ctx, good); err != nil {
		t.Fatalf("Save(create): %v", err)
	}
	before := readRoleState(t, db, good.ID)

	bad = configB(good.ID)
	bad.Perms = []string{"dup", "dup"}
	if err := repo.Save(ctx, bad); err == nil {
		t.Fatal("Save(update) collided on the primary key but returned no error")
	}
	if after := readRoleState(t, db, good.ID); after.key() != before.key() {
		t.Fatalf("configuration changed after failed edit\nbefore %v\n after %v", before, after)
	}
}

// Creating a role that collides on uk_code returns ErrDuplicateRole: the pre-check cannot stop concurrent creates of
// the same code, so after the unique key decides, it must become a domain error, not a 500. uk_code does not include
// deleted_at, so a soft-deleted role's code may not be reused either.
func TestRoleSaveCreateDuplicateCode(t *testing.T) {
	db := roleSaveTestDB(t)
	repo := NewRoleRepo(&coredata.Data{DB: db})
	ctx := context.Background()

	first := configA(0)
	if err := repo.Save(ctx, first); err != nil {
		t.Fatalf("Save(create): %v", err)
	}
	if err := repo.Save(ctx, configA(0)); !errors.Is(err, biz.ErrDuplicateRole) {
		t.Fatalf("create with same code = %v, want ErrDuplicateRole", err)
	}
	if err := repo.Delete(ctx, first.ID); err != nil {
		t.Fatalf("Delete: %v", err)
	}
	if err := repo.Save(ctx, configA(0)); !errors.Is(err, biz.ErrDuplicateRole) {
		t.Fatalf("reuse soft-deleted role's code = %v, want ErrDuplicateRole", err)
	}
}
