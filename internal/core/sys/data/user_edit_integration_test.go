//go:build integration

package data

import (
	"context"
	"fmt"
	"slices"
	"sync"
	"testing"
	"time"

	coredata "github.com/go-keel/keel-admin/internal/core/data"
	"github.com/go-keel/keel-admin/internal/core/sys/biz"
)

// A changes a user's nickname and swaps the role r1 for r2 while B adds a role rx that A cannot see (outside A's
// assignable range): after both succeed, rx must still be there. Which invisible roles A pins must be computed from the
// roles read after locking the user row; computed from a snapshot outside the transaction, A's save strips rx when B
// happens to commit between A's two reads. A goes through the real UserAdminUseCase (the pre-transaction half is in
// biz), and B calls the repo directly, adding one role to the current roles in the callback.
//
// sqlite cannot test locking, so this runs on real MySQL: make integration points at an empty database that has run
// the admin complete schema (not the development database: the test writes and hard-deletes the rows it creates).
func TestUserEditConcurrentKeepsInvisibleRole(t *testing.T) {
	db := openTestDB(t)
	d := &coredata.Data{DB: db}
	users, roles := NewUserAdminRepo(d), NewRoleRepo(d)
	uc := biz.NewUserAdminUseCase(users, biz.NewARBACUseCase(NewARBACRepo(d), users), nil)
	ctx := context.Background()

	// Roles r1, r2, rx, and the management role A holds: it may assign r1 and r2 and manage root department 1 seeded by
	// migrations/admin/0_schema.sql.
	suffix := time.Now().UnixNano()
	var roleIDs []int64
	newRole := func(name string, assignable []int64, depts []int64) int64 {
		r := &biz.Role{Code: fmt.Sprintf("txuser_%s_%d", name, suffix), Name: name, Status: 1, AssignableRoleIDs: assignable, ManageDepts: depts}
		if err := roles.Save(ctx, r); err != nil {
			t.Fatalf("seed role %s: %v", name, err)
		}
		roleIDs = append(roleIDs, r.ID)
		return r.ID
	}
	var userID int64
	t.Cleanup(func() {
		db.Exec(`DELETE FROM sys_user_role WHERE user_id = ?`, userID)
		db.Exec(`DELETE FROM sys_user WHERE id = ?`, userID)
		for _, table := range []string{"sys_role_permission", "sys_role_assignable", "sys_role_manage_dept", "sys_role_data_scope", "sys_role_resource_grant"} {
			db.Exec("DELETE FROM "+table+" WHERE role_id IN ?", roleIDs)
		}
		db.Exec(`DELETE FROM sys_role WHERE id IN ?`, roleIDs)
	})
	r1, r2, rx := newRole("r1", nil, nil), newRole("r2", nil, nil), newRole("rx", nil, nil)
	manager := newRole("manager", []int64{r1, r2}, []int64{1})

	target := &biz.User{Username: fmt.Sprintf("txuser_%d", suffix), Password: "x", Nickname: "old", DeptID: 1, Status: 1}
	if err := users.Create(ctx, target, []int64{r1}); err != nil {
		t.Fatalf("seed user: %v", err)
	}
	userID = target.ID
	actorA := &biz.Actor{UserID: -1, RoleIDs: []int64{manager}}
	addRX := func(current []int64) ([]int64, bool, error) {
		if slices.Contains(current, rx) {
			return nil, false, nil
		}
		return append(slices.Clone(current), rx), true, nil
	}
	want := fmt.Sprint(slices.Sorted(slices.Values([]int64{r2, rx})))

	// The round count was measured: with A's pin switched back to the out-of-transaction snapshot, or the user-row lock
	// removed, a few runs on local MySQL 8.4 hit rx being stripped in rounds 6 to 24.
	const rounds = 300
	for round := range rounds {
		old := "old"
		if err := users.Edit(ctx, userID, biz.UserEditChanges{Nickname: &old, Roles: replaceRoles(r1)}); err != nil {
			t.Fatalf("round %d: reset: %v", round, err)
		}
		start := make(chan struct{})
		var wg sync.WaitGroup
		var errA, errB error
		wg.Go(func() {
			<-start
			_, errA = uc.Edit(ctx, actorA, userID, biz.UserEdit{Nickname: "by-A", DeptID: 1, RoleIDs: []int64{r2}})
		})
		wg.Go(func() {
			<-start
			errB = users.Edit(ctx, userID, biz.UserEditChanges{Roles: addRX})
		})
		close(start)
		wg.Wait()
		if errA != nil || errB != nil {
			t.Fatalf("round %d: A err = %v, B err = %v", round, errA, errB)
		}
		got, err := users.RoleIDs(ctx, userID)
		if err != nil {
			t.Fatalf("round %d: RoleIDs: %v", round, err)
		}
		if s := fmt.Sprint(slices.Sorted(slices.Values(got))); s != want {
			t.Fatalf("round %d: roles should be %s, got %s", round, want, s)
		}
		u, err := users.FindByID(ctx, userID)
		if err != nil {
			t.Fatalf("round %d: FindByID: %v", round, err)
		}
		if u.Nickname != "by-A" {
			t.Fatalf("round %d: nickname should be by-A, got %q", round, u.Nickname)
		}
	}
}
