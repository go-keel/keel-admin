//go:build integration

package data

import (
	"context"
	"fmt"
	"sync"
	"testing"
	"time"

	coredata "github.com/go-keel/keel-admin/internal/core/data"
	"github.com/go-keel/keel-admin/internal/core/sys/biz"
)

// Two people edit the same role at the same time, each submitting a configuration that differs in all six tables: the
// result must be exactly one of them, never a mix, and never a deadlock. sqlite cannot test locking, so this runs on
// real MySQL: make integration points at an empty database that has run the admin complete schema (not the
// development database: the test writes and hard-deletes the rows it creates).
func TestRoleSaveConcurrentUpdatesNeverMix(t *testing.T) {
	db := openTestDB(t)
	repo := NewRoleRepo(&coredata.Data{DB: db})
	ctx := context.Background()

	seed := configA(0)
	seed.Code = fmt.Sprintf("txrace_%d", time.Now().UnixNano())
	if err := repo.Save(ctx, seed); err != nil {
		t.Fatalf("seed: %v", err)
	}
	id := seed.ID
	t.Cleanup(func() {
		for _, table := range []string{"sys_role_permission", "sys_role_assignable", "sys_role_manage_dept", "sys_role_data_scope", "sys_role_resource_grant"} {
			db.Exec("DELETE FROM "+table+" WHERE role_id = ?", id)
		}
		db.Exec(`DELETE FROM sys_role WHERE id = ?`, id)
	})

	wantA, wantB := stateOf(configA(id)).key(), stateOf(configB(id)).key()
	// The round count was measured: the old form (one transaction per table) took tens to hundreds of rounds to hit
	// once on local MySQL 8.4, and 500 rounds caught it in each of 5 runs.
	const rounds = 500
	for round := range rounds {
		// Each round first resets to an empty configuration: with no old rows both deletes take only gap locks and do
		// not block each other, then the inserts wait on each other. That is the likeliest deadlock when role rows are
		// not locked first; with old rows present the row locks happen to keep them apart.
		if err := repo.Save(ctx, &biz.Role{ID: id, Name: "empty", Status: 1}); err != nil {
			t.Fatalf("round %d: reset: %v", round, err)
		}
		start := make(chan struct{})
		var wg sync.WaitGroup
		errs := make([]error, 2)
		for i, cfg := range []func(int64) *biz.Role{configA, configB} {
			wg.Go(func() {
				r := cfg(id)
				<-start
				errs[i] = repo.Save(ctx, r)
			})
		}
		close(start)
		wg.Wait()
		for _, err := range errs {
			if err != nil {
				t.Fatalf("round %d: Save: %v", round, err)
			}
		}
		if got := readRoleState(t, db, id).key(); got != wantA && got != wantB {
			t.Fatalf("round %d: mixed configuration\n got %s\n  A %s\n  B %s", round, got, wantA, wantB)
		}
	}
}
