package biz

import (
	"context"
	"errors"
	"os"
	"testing"

	"github.com/go-keel/keel-admin/internal/core/perm"
)

// TestMain registers a small, self-contained perm resource set once for the
// whole package so validateScopes' registry/anchor checks have something
// to check against — this test package doesn't need sys.Resources, it just
// needs *a* sealed registry.
//
// fake_dept_resource is dept_test.go's fixture for exercising
// biz.DeptUseCase.Delete's occupancy loop (perm.ResourcesWithDeptOccupancy):
// since perm's registry is sealed exactly once for this whole test binary,
// individual tests can't perm.ResetForTest()/MustRegister a resource of
// their own without unsealing (and breaking) this shared registry — they
// instead flip deptOccupancyTestFlag (see setDeptOccupancy in dept_test.go).
func TestMain(m *testing.M) {
	perm.MustRegisterGroups([]perm.Group{
		{Domain: "sys"},
		{Domain: "sys", Module: "dept"},
		{Domain: "sys", Module: "account"},
	})
	perm.MustRegisterKernel(
		[]perm.Permission{
			{Code: "sys:dept:view"},
			{Code: "sys:account:view"},
		},
		nil,
		[]perm.ResourceDef{
			// notice declares ListGrantable, so RESOURCE_GRANT is configurable (candidate instances 100/101);
			// self_only declares none, so it is not configurable.
			{
				Res:     "notice",
				Anchors: perm.AnchorSelf | perm.AnchorDept,
				ListGrantable: func(context.Context) ([]perm.GrantOption, error) {
					return []perm.GrantOption{{ID: 100, Label: "Group A"}, {ID: 101, Label: "Group B"}}, nil
				},
			},
			{Res: "self_only", Anchors: perm.AnchorSelf},
			{
				Res:     "fake_dept_resource",
				Anchors: perm.AnchorDept,
				DeptOccupancy: func(_ context.Context, _ int64) (bool, error) {
					return deptOccupancyTestFlag, nil
				},
			},
		},
	)
	if err := perm.Seal(nil); err != nil {
		panic(err)
	}
	os.Exit(m.Run())
}

type fakeRoleRepo struct {
	roles  map[int64]*Role
	byCode map[string]*Role
	getErr error // when non-nil, Get returns it directly (simulates a database error)
	nextID int64
	scopes map[int64][]DataScopeInput

	// records the arguments List received: whether the filter and paging are passed down unchanged is
	// the only thing this layer can test (the actual filtering lives in the data layer's SQL, which is not tested
	// here: we test neither upstream nor SQL).
	lastFilter RoleFilter
	lastOffset int
	lastLimit  int

	saveErr      error
	getByCodeErr error // simulates the pre-check query itself failing (a dropped connection, say)
}

var _ RoleRepo = (*fakeRoleRepo)(nil)

func newFakeRoleRepo() *fakeRoleRepo {
	return &fakeRoleRepo{roles: map[int64]*Role{}, byCode: map[string]*Role{}, nextID: 1}
}

func (f *fakeRoleRepo) all() []*Role {
	out := make([]*Role, 0, len(f.roles))
	for id := int64(1); id < f.nextID; id++ {
		if r, ok := f.roles[id]; ok {
			out = append(out, r)
		}
	}
	return out
}

func (f *fakeRoleRepo) List(_ context.Context, filter RoleFilter, offset, limit int) ([]*RoleListRow, int64, error) {
	f.lastFilter, f.lastOffset, f.lastLimit = filter, offset, limit
	all := f.all()
	total := int64(len(all))
	if offset >= len(all) {
		return []*RoleListRow{}, total, nil
	}
	page := all[offset:min(offset+limit, len(all))]
	out := make([]*RoleListRow, 0, len(page))
	for _, r := range page {
		out = append(out, &RoleListRow{
			ID: r.ID, Code: r.Code, Name: r.Name,
			Sensitive: r.Sensitive, ForceOTP: r.ForceOTP, Status: r.Status,
			PermCount: int32(len(r.Perms)),
		})
	}
	return out, total, nil
}
func (f *fakeRoleRepo) ListBrief(context.Context) ([]*Role, error) {
	return f.all(), nil
}
func (f *fakeRoleRepo) Get(_ context.Context, id int64) (*Role, error) {
	if f.getErr != nil {
		return nil, f.getErr
	}
	r, ok := f.roles[id]
	if !ok {
		return nil, ErrRoleNotFound // matches the real implementation
	}
	return r, nil
}
func (f *fakeRoleRepo) GetByCode(_ context.Context, code string) (*Role, error) {
	if f.getByCodeErr != nil {
		return nil, f.getByCodeErr
	}
	r, ok := f.byCode[code]
	if !ok {
		return nil, ErrRoleNotFound // matches the real implementation: only this sentinel means "absent"
	}
	return r, nil
}

// Save follows the real implementation's contract: ID 0 creates and backfills the ID, otherwise the role must
// exist. saveErr simulates a failed transaction (the real implementation rolls back entirely; here nothing
// is written).
func (f *fakeRoleRepo) Save(_ context.Context, r *Role) error {
	if f.saveErr != nil {
		return f.saveErr
	}
	if r.ID == 0 {
		r.ID = f.nextID
		f.nextID++
		f.byCode[r.Code] = r
	} else if _, ok := f.roles[r.ID]; !ok {
		return ErrRoleNotFound
	}
	f.roles[r.ID] = r
	return nil
}
func (f *fakeRoleRepo) Delete(_ context.Context, id int64) error {
	delete(f.roles, id)
	return nil
}
func (f *fakeRoleRepo) DataScopes(context.Context, []int64) (map[int64][]DataScopeInput, error) {
	return f.scopes, nil
}

type fakeCacheInvalidator struct{ invalidated []int64 }

var _ CacheInvalidator = (*fakeCacheInvalidator)(nil)

func (f *fakeCacheInvalidator) InvalidateRole(_ context.Context, roleID int64) {
	f.invalidated = append(f.invalidated, roleID)
}

type fakeARBAC struct {
	// assignable holds only enabled roles, per the repo contract (the query enforces enabled; tested in the data layer).
	assignable []int64
	sensitive  []int64
	// The read-back side (used by List), kept apart from the assignable union used for the checks above.
	assignableCfg map[int64][]int64
	mdeptCfg      map[int64][]int64
}

var _ ARBACRepo = (*fakeARBAC)(nil)

func (f fakeARBAC) AssignableRoleIDs(context.Context, []int64) ([]int64, error) {
	return f.assignable, nil
}
func (fakeARBAC) ManageDeptIDs(context.Context, []int64) ([]int64, error) { return nil, nil }
func (f fakeARBAC) SensitiveRoleIDs(_ context.Context, ids []int64) ([]int64, error) {
	set := toSet(f.sensitive)
	var out []int64
	for _, id := range ids {
		if set[id] {
			out = append(out, id)
		}
	}
	return out, nil
}
func (f fakeARBAC) AssignableConfigs(context.Context, []int64) (map[int64][]int64, error) {
	return f.assignableCfg, nil
}
func (f fakeARBAC) ManageDeptConfigs(context.Context, []int64) (map[int64][]int64, error) {
	return f.mdeptCfg, nil
}

func TestRoleUseCase_Create_DuplicateCode(t *testing.T) {
	uc := NewRoleUseCase(newFakeRoleRepo(), fakeARBAC{}, &fakeCacheInvalidator{})
	ctx := context.Background()

	if _, err := uc.Create(ctx, "ops", RoleUpdate{Name: "Ops", ForceOTP: 0, Sensitive: 0, Status: 1}); err != nil {
		t.Fatalf("first create: %v", err)
	}
	if _, err := uc.Create(ctx, "ops", RoleUpdate{Name: "Ops2", ForceOTP: 0, Sensitive: 0, Status: 1}); !errors.Is(err, ErrDuplicateRole) {
		t.Fatalf("expected ErrDuplicateRole, got %v", err)
	}
}

// A failing pre-check query must not be treated as "does not exist" and go on creating: return that error
// unchanged and write nothing.
func TestRoleUseCase_Create_LookupErrorStops(t *testing.T) {
	repo := newFakeRoleRepo()
	repo.getByCodeErr = errors.New("connection lost")
	uc := NewRoleUseCase(repo, fakeARBAC{}, &fakeCacheInvalidator{})

	_, err := uc.Create(context.Background(), "ops", RoleUpdate{Name: "Ops", Status: 1})
	if !errors.Is(err, repo.getByCodeErr) {
		t.Fatalf("Create = %v, want the pre-check query error", err)
	}
	if len(repo.roles) != 0 {
		t.Fatalf("created %d roles after the pre-check failed", len(repo.roles))
	}
}

// Sensitive only means "non-root users cannot assign it"; it does not block root from updating or deleting
// (rejecting Update/Delete for everyone including root was a second meaning that was rejected, see ADR
// admin-0015). This test locks that in; do not add it back.
func TestRoleUseCase_Sensitive_DoesNotBlockUpdateOrDelete(t *testing.T) {
	repo := newFakeRoleRepo()
	uc := NewRoleUseCase(repo, fakeARBAC{}, &fakeCacheInvalidator{})
	ctx := context.Background()

	r, _ := uc.Create(ctx, "tech", RoleUpdate{Name: "Tech", ForceOTP: 0, Sensitive: 1, Status: 1}) // sensitive role
	if r.Sensitive != 1 {
		t.Fatalf("create: expected sensitive=1, got %d", r.Sensitive)
	}
	got, err := uc.Update(ctx, r.ID, RoleUpdate{Name: "X", Status: 1, Sensitive: 1})
	if err != nil {
		t.Fatalf("update sensitive role: %v", err)
	}
	if got.Name != "X" || got.Sensitive != 1 {
		t.Fatalf("expected name=X sensitive=1, got name=%s sensitive=%d", got.Name, got.Sensitive)
	}
	if err := uc.Delete(ctx, r.ID); err != nil {
		t.Fatalf("delete sensitive role: %v", err)
	}
}

// PermissionsOf lets a non-root user see only roles within their assignable range; otherwise it would be a hole
// for enumerating the permissions of any role. Root is unrestricted.
func TestRoleUseCase_PermissionsOf_RangeGuard(t *testing.T) {
	repo := newFakeRoleRepo()
	ctx := context.Background()
	seeder := NewRoleUseCase(repo, fakeARBAC{}, &fakeCacheInvalidator{})
	inRange, _ := seeder.Create(ctx, "ops", RoleUpdate{Name: "Ops", ForceOTP: 0, Sensitive: 0, Status: 1})
	repo.roles[inRange.ID].Perms = []string{"sys:dept:view"}
	outRange, _ := seeder.Create(ctx, "fin", RoleUpdate{Name: "Fin", ForceOTP: 0, Sensitive: 0, Status: 1})
	repo.roles[outRange.ID].Perms = []string{"sys:account:view"}
	priv, _ := seeder.Create(ctx, "tech", RoleUpdate{Name: "Tech", ForceOTP: 0, Sensitive: 1, Status: 1}) // sensitive role
	repo.roles[priv.ID].Perms = []string{"sys:account:kick"}

	// actor can only assign inRange.
	uc := NewRoleUseCase(repo, fakeARBAC{assignable: []int64{inRange.ID}}, &fakeCacheInvalidator{})
	actor := &Actor{UserID: 9, IsRoot: false, RoleIDs: []int64{7}}

	got, err := uc.PermissionsOf(ctx, actor, inRange.ID)
	if err != nil || len(got) != 1 || got[0] != "sys:dept:view" {
		t.Fatalf("in-range: expected [sys:dept:view], got %v (err %v)", got, err)
	}
	if _, err := uc.PermissionsOf(ctx, actor, outRange.ID); !errors.Is(err, ErrRoleOutOfRange) {
		t.Fatalf("out-of-range: expected ErrRoleOutOfRange, got %v", err)
	}
	// A sensitive role stays hidden even if it was mistakenly put into assignable.
	uc2 := NewRoleUseCase(repo, fakeARBAC{assignable: []int64{priv.ID}, sensitive: []int64{priv.ID}}, &fakeCacheInvalidator{})
	if _, err := uc2.PermissionsOf(ctx, actor, priv.ID); !errors.Is(err, ErrRoleOutOfRange) {
		t.Fatalf("sensitive: expected ErrRoleOutOfRange, got %v", err)
	}
	// Root is unrestricted.
	rootUC := NewRoleUseCase(repo, fakeARBAC{}, &fakeCacheInvalidator{})
	if got, err := rootUC.PermissionsOf(ctx, &Actor{IsRoot: true}, outRange.ID); err != nil || len(got) != 1 {
		t.Fatalf("root: expected 1 code, got %v (err %v)", got, err)
	}
}

// Update is the only write entry for a role: basic info, permissions and the three boundaries change in one go,
// sensitive included (it is an ordinary field and needs no separate toggle endpoint).
func TestRoleUseCase_Update_ReplacesEverything(t *testing.T) {
	repo := newFakeRoleRepo()
	uc := NewRoleUseCase(repo, fakeARBAC{}, &fakeCacheInvalidator{})
	ctx := context.Background()

	r, _ := uc.Create(ctx, "ops", RoleUpdate{Name: "Ops", ForceOTP: 0, Sensitive: 0, Status: 1})
	got, err := uc.Update(ctx, r.ID, RoleUpdate{
		Name:              "Ops2",
		ForceOTP:          1,
		Status:            0,
		Sensitive:         1,
		AssignableRoleIDs: []int64{9},
		ManageDepts:       []int64{3},
	})
	if err != nil {
		t.Fatalf("update: %v", err)
	}
	if got.Name != "Ops2" || got.ForceOTP != 1 || got.Status != 0 || got.Sensitive != 1 {
		t.Fatalf("basic fields not applied: %+v", got)
	}
	if len(got.AssignableRoleIDs) != 1 || got.AssignableRoleIDs[0] != 9 {
		t.Fatalf("assignable not applied: %v", got.AssignableRoleIDs)
	}
	if len(got.ManageDepts) != 1 || got.ManageDepts[0] != 3 {
		t.Fatalf("manage depts not applied: %v", got.ManageDepts)
	}
	if _, err := uc.Update(ctx, 404, RoleUpdate{Name: "X", Status: 1}); !errors.Is(err, ErrRoleNotFound) {
		t.Fatalf("expected ErrRoleNotFound, got %v", err)
	}
}

// Retired permission codes (old roles in the database may still hold them) must not make an edit fail: the edit
// silently drops dead codes, keeps the codes that still exist and saves as usual.
func TestRoleUseCase_Update_DropsUnknownPerms(t *testing.T) {
	repo := newFakeRoleRepo()
	uc := NewRoleUseCase(repo, fakeARBAC{}, &fakeCacheInvalidator{})
	ctx := context.Background()

	r, _ := uc.Create(ctx, "ops", RoleUpdate{Name: "Ops", ForceOTP: 0, Sensitive: 0, Status: 1})
	got, err := uc.Update(ctx, r.ID, RoleUpdate{
		Name: "Renamed", Status: 1,
		Perms: []string{"sys:account:view", "sys:nope:nope"},
	})
	if err != nil {
		t.Fatalf("expected unknown perm to be dropped, not error: %v", err)
	}
	if len(got.Perms) != 1 || got.Perms[0] != "sys:account:view" {
		t.Fatalf("expected only known perm kept, got %v", got.Perms)
	}
	if repo.roles[r.ID].Name != "Renamed" {
		t.Fatalf("write should proceed after dropping dead code: %s", repo.roles[r.ID].Name)
	}
}

// Create and Update both hand the basic info and the whole configuration to a single Save: the repo must receive
// the complete set so that one transaction covers every write. When Save fails the use case returns the error
// unchanged, and Update does not invalidate the cache (no commit, no change).
func TestRoleUseCase_SaveCarriesWholeConfig(t *testing.T) {
	repo := newFakeRoleRepo()
	cache := &fakeCacheInvalidator{}
	uc := NewRoleUseCase(repo, fakeARBAC{}, cache)
	ctx := context.Background()
	want := RoleUpdate{
		Name: "Ops", Status: 1,
		Perms:             []string{"sys:dept:view"},
		AssignableRoleIDs: []int64{9},
		ManageDepts:       []int64{3},
		Scopes:            []DataScopeInput{{ResourceType: "notice", ScopeType: ScopeDept}},
	}

	r, err := uc.Create(ctx, "ops", want)
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	saved := repo.roles[r.ID]
	if len(saved.Perms) != 1 || len(saved.AssignableRoleIDs) != 1 || len(saved.ManageDepts) != 1 || len(saved.Scopes) != 1 {
		t.Fatalf("Create did not hand Save the complete configuration: %+v", saved)
	}

	want.Perms = []string{"sys:account:view"}
	if _, err := uc.Update(ctx, r.ID, want); err != nil {
		t.Fatalf("Update: %v", err)
	}
	if got := repo.roles[r.ID].Perms; len(got) != 1 || got[0] != "sys:account:view" {
		t.Fatalf("permission codes Update handed to Save = %v", got)
	}

	boom := errors.New("tx failed")
	repo.saveErr = boom
	cache.invalidated = nil
	if _, err := uc.Update(ctx, r.ID, want); !errors.Is(err, boom) {
		t.Fatalf("Update: expected Save error, got %v", err)
	}
	if len(cache.invalidated) != 0 {
		t.Fatalf("the cache must not be invalidated after Save fails, got %v", cache.invalidated)
	}
	if _, err := uc.Create(ctx, "fin", want); !errors.Is(err, boom) {
		t.Fatalf("Create: expected Save error, got %v", err)
	}
}

// The assignment dropdown and viewing permissions use the same range (assignableRange): only roles in the
// dropdown can have their permissions viewed, and none outside it (sensitive, disabled, not in assignable)
// can. The repo query filters out disabled roles; the fake repo here honours that contract and does not
// return them.
func TestRoleUseCase_ListAssignable(t *testing.T) {
	repo := newFakeRoleRepo()
	ctx := context.Background()

	seed := NewRoleUseCase(repo, fakeARBAC{}, &fakeCacheInvalidator{})
	a, _ := seed.Create(ctx, "a", RoleUpdate{Name: "A", Status: 1})               // assignable
	b, _ := seed.Create(ctx, "b", RoleUpdate{Name: "B", Sensitive: 1, Status: 1}) // sensitive, must be excluded
	c, _ := seed.Create(ctx, "c", RoleUpdate{Name: "C", Status: 0})               // disabled, the repo does not return it
	d, _ := seed.Create(ctx, "d", RoleUpdate{Name: "D", Status: 1})               // not in assignable

	uc := NewRoleUseCase(repo, fakeARBAC{assignable: []int64{a.ID, b.ID}, sensitive: []int64{b.ID}}, &fakeCacheInvalidator{})
	actor := &Actor{UserID: 9, RoleIDs: []int64{1}}

	got, err := uc.ListAssignable(ctx, actor)
	if err != nil {
		t.Fatalf("ListAssignable: %v", err)
	}
	if len(got) != 1 || got[0].ID != a.ID {
		t.Fatalf("non-root: expected only role a(%d), got %+v", a.ID, got)
	}
	for _, r := range []*Role{a, b, c, d} {
		_, err := uc.PermissionsOf(ctx, actor, r.ID)
		if inList := r.ID == a.ID; inList != (err == nil) {
			t.Errorf("role %s: in dropdown=%v but PermissionsOf err=%v", r.Code, inList, err)
		}
	}

	all, err := uc.ListAssignable(ctx, &Actor{UserID: 1, IsRoot: true})
	if err != nil {
		t.Fatalf("ListAssignable root: %v", err)
	}
	if len(all) != 4 {
		t.Fatalf("root: expected all 4 roles, got %d", len(all))
	}
}

func TestRoleUseCase_Delete_NotFound(t *testing.T) {
	uc := NewRoleUseCase(newFakeRoleRepo(), fakeARBAC{}, &fakeCacheInvalidator{})
	if err := uc.Delete(context.Background(), 404); !errors.Is(err, ErrRoleNotFound) {
		t.Fatalf("expected ErrRoleNotFound, got %v", err)
	}
}

func TestRoleUseCase_Update_DataScope_UnknownResource(t *testing.T) {
	repo := newFakeRoleRepo()
	uc := NewRoleUseCase(repo, fakeARBAC{}, &fakeCacheInvalidator{})
	ctx := context.Background()
	r, _ := uc.Create(ctx, "ops", RoleUpdate{Name: "Ops", ForceOTP: 0, Sensitive: 0, Status: 1})

	_, err := uc.Update(ctx, r.ID, RoleUpdate{
		Name: "Ops", Status: 1,
		Scopes: []DataScopeInput{{ResourceType: "ghost", ScopeType: ScopeSelf}},
	})
	if !errors.Is(err, ErrInvalidArgument) {
		t.Fatalf("expected ErrInvalidArgument for unregistered resource, got %v", err)
	}
}

func TestRoleUseCase_Update_DataScope_DuplicateResource(t *testing.T) {
	repo := newFakeRoleRepo()
	uc := NewRoleUseCase(repo, fakeARBAC{}, &fakeCacheInvalidator{})
	ctx := context.Background()
	r, _ := uc.Create(ctx, "ops", RoleUpdate{Name: "Ops", ForceOTP: 0, Sensitive: 0, Status: 1})

	// The same resource given twice: each entry is valid on its own; if validation does not reject it, it hits
	// uk_role_res and becomes a 500.
	_, err := uc.Update(ctx, r.ID, RoleUpdate{
		Name: "Ops", Status: 1,
		Scopes: []DataScopeInput{
			{ResourceType: "self_only", ScopeType: ScopeSelf},
			{ResourceType: "self_only", ScopeType: ScopeAll},
		},
	})
	if !errors.Is(err, ErrInvalidArgument) {
		t.Fatalf("expected ErrInvalidArgument for duplicate resource, got %v", err)
	}
}

func TestRoleUseCase_Update_DataScope_AnchorMismatch(t *testing.T) {
	repo := newFakeRoleRepo()
	uc := NewRoleUseCase(repo, fakeARBAC{}, &fakeCacheInvalidator{})
	ctx := context.Background()
	r, _ := uc.Create(ctx, "ops", RoleUpdate{Name: "Ops", ForceOTP: 0, Sensitive: 0, Status: 1})

	// self_only has no AnchorDept — DEPT scope must be rejected.
	_, err := uc.Update(ctx, r.ID, RoleUpdate{
		Name: "Ops", Status: 1,
		Scopes: []DataScopeInput{{ResourceType: "self_only", ScopeType: ScopeDept}},
	})
	if !errors.Is(err, ErrInvalidArgument) {
		t.Fatalf("expected ErrInvalidArgument for anchor mismatch, got %v", err)
	}
}

func TestRoleUseCase_Update_InvalidatesRoleCache(t *testing.T) {
	repo := newFakeRoleRepo()
	cache := &fakeCacheInvalidator{}
	uc := NewRoleUseCase(repo, fakeARBAC{}, cache)
	ctx := context.Background()
	r, _ := uc.Create(ctx, "ops", RoleUpdate{Name: "Ops", ForceOTP: 0, Sensitive: 0, Status: 1})

	if _, err := uc.Update(ctx, r.ID, RoleUpdate{Name: "Ops", Status: 1}); err != nil {
		t.Fatalf("Update: %v", err)
	}
	if len(cache.invalidated) != 1 || cache.invalidated[0] != r.ID {
		t.Fatalf("expected InvalidateRole(%d) once, got %v", r.ID, cache.invalidated)
	}
}

func TestRoleUseCase_Delete_InvalidatesRoleCache(t *testing.T) {
	repo := newFakeRoleRepo()
	cache := &fakeCacheInvalidator{}
	uc := NewRoleUseCase(repo, fakeARBAC{}, cache)
	ctx := context.Background()
	r, _ := uc.Create(ctx, "ops", RoleUpdate{Name: "Ops", ForceOTP: 0, Sensitive: 0, Status: 1})

	if err := uc.Delete(ctx, r.ID); err != nil {
		t.Fatalf("Delete: %v", err)
	}
	if len(cache.invalidated) != 1 || cache.invalidated[0] != r.ID {
		t.Fatalf("expected InvalidateRole(%d) once, got %v", r.ID, cache.invalidated)
	}
}

func TestRoleUseCase_Update_DataScope_OK(t *testing.T) {
	repo := newFakeRoleRepo()
	uc := NewRoleUseCase(repo, fakeARBAC{}, &fakeCacheInvalidator{})
	ctx := context.Background()
	r, _ := uc.Create(ctx, "ops", RoleUpdate{Name: "Ops", ForceOTP: 0, Sensitive: 0, Status: 1})

	// A resource can have only one policy at a time, so notice's three scopes are submitted in three calls.
	for _, notice := range []DataScopeInput{
		{ResourceType: "notice", ScopeType: ScopeSelf},
		{ResourceType: "notice", ScopeType: ScopeDept},
		{ResourceType: "notice", ScopeType: ScopeGrant, ResourceIDs: []int64{100, 101}}, // an instance among the candidates
	} {
		items := []DataScopeInput{notice, {ResourceType: "self_only", ScopeType: ScopeAll}}
		if _, err := uc.Update(ctx, r.ID, RoleUpdate{Name: "Ops", Status: 1, Scopes: items}); err != nil {
			t.Fatalf("expected compatible scope %s to pass, got %v", notice.ScopeType, err)
		}
	}
}

// The gate for RESOURCE_GRANT: a resource that does not declare ListGrantable cannot be configured at all; the
// instance set must not be empty (empty = a dead policy that always denies all); every id must be among the
// candidates; a non-GRANT policy must not carry an instance set. The frontend disables these too, but this is
// the boundary.
func TestRoleUseCase_Update_GrantScope_Guards(t *testing.T) {
	repo := newFakeRoleRepo()
	uc := NewRoleUseCase(repo, fakeARBAC{}, &fakeCacheInvalidator{})
	ctx := context.Background()
	r, _ := uc.Create(ctx, "ops", RoleUpdate{Name: "Ops", ForceOTP: 0, Sensitive: 0, Status: 1})

	// self_only declares no ListGrantable, so RESOURCE_GRANT is not configurable.
	err := scopeUpdate(ctx, uc, r.ID, DataScopeInput{ResourceType: "self_only", ScopeType: ScopeGrant, ResourceIDs: []int64{1}})
	if !errors.Is(err, ErrInvalidArgument) {
		t.Fatalf("grant on non-grantable resource: expected ErrInvalidArgument, got %v", err)
	}
	// An empty instance set is rejected (dead policy).
	err = scopeUpdate(ctx, uc, r.ID, DataScopeInput{ResourceType: "notice", ScopeType: ScopeGrant})
	if !errors.Is(err, ErrInvalidArgument) {
		t.Fatalf("grant with no instances: expected ErrInvalidArgument, got %v", err)
	}
	// An instance id outside the candidates is rejected.
	err = scopeUpdate(ctx, uc, r.ID, DataScopeInput{ResourceType: "notice", ScopeType: ScopeGrant, ResourceIDs: []int64{100, 999}})
	if !errors.Is(err, ErrInvalidArgument) {
		t.Fatalf("grant with unknown instance: expected ErrInvalidArgument, got %v", err)
	}
	// A non-GRANT policy carrying an instance set is rejected.
	err = scopeUpdate(ctx, uc, r.ID, DataScopeInput{ResourceType: "notice", ScopeType: ScopeDept, ResourceIDs: []int64{100}})
	if !errors.Is(err, ErrInvalidArgument) {
		t.Fatalf("non-grant scope with instances: expected ErrInvalidArgument, got %v", err)
	}
	// Instances among the candidates pass.
	if err := scopeUpdate(ctx, uc, r.ID, DataScopeInput{ResourceType: "notice", ScopeType: ScopeGrant, ResourceIDs: []int64{101}}); err != nil {
		t.Fatalf("grant with known instances: expected pass, got %v", err)
	}
}

func scopeUpdate(ctx context.Context, uc *RoleUseCase, id int64, s DataScopeInput) error {
	_, err := uc.Update(ctx, id, RoleUpdate{Name: "Ops", Status: 1, Scopes: []DataScopeInput{s}})
	return err
}

// TestRoleListFillsBoundaries locks in reading back the three boundaries. All three Set* calls replace the
// whole set, so a configuration UI that cannot get the current values can only submit an empty set: "open
// the dialog and click OK" clears it. This assertion is the regression point for that behaviour.
// The boundaries are read back by Get (the source of the edit form's initial values), not by List: list rows
// are slim and structurally cannot carry boundaries. Failing to read them back raises no error; it only makes
// the edit form submit empty values, and all of these replace the whole set: every open of the edit form
// would wipe someone else's configuration.
func TestRoleGetFillsBoundaries(t *testing.T) {
	repo := newFakeRoleRepo()
	ctx := context.Background()
	seeded, _ := NewRoleUseCase(repo, fakeARBAC{}, &fakeCacheInvalidator{}).
		Create(ctx, "ops", RoleUpdate{Name: "Ops", ForceOTP: 0, Sensitive: 0, Status: 1})

	repo.scopes = map[int64][]DataScopeInput{
		seeded.ID: {{ResourceType: "notice", ScopeType: ScopeDept}},
	}
	arbac := fakeARBAC{
		assignableCfg: map[int64][]int64{seeded.ID: {2, 3}},
		mdeptCfg:      map[int64][]int64{seeded.ID: {7}},
	}
	uc := NewRoleUseCase(repo, arbac, &fakeCacheInvalidator{})

	r, err := uc.Get(ctx, seeded.ID)
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	if len(r.AssignableRoleIDs) != 2 {
		t.Errorf("AssignableRoleIDs = %v, want 2 items", r.AssignableRoleIDs)
	}
	if len(r.ManageDepts) != 1 || r.ManageDepts[0] != 7 {
		t.Errorf("ManageDepts = %+v, want [7]", r.ManageDepts)
	}
	if len(r.Scopes) != 1 || r.Scopes[0].ScopeType != ScopeDept {
		t.Errorf("Scopes = %+v, want [notice/DEPT]", r.Scopes)
	}
}

// A filter dropped halfway raises no error; it only makes "filtered" look the same as "unfiltered": the
// request goes out, the page renders, only the counts are wrong. A limit not greater than 0 gets the default
// page size: a negative value passed down unchanged would mean "no limit" to gorm.
func TestRoleListPassesFilterAndDefaultsLimit(t *testing.T) {
	repo := newFakeRoleRepo()
	uc := NewRoleUseCase(repo, fakeARBAC{}, &fakeCacheInvalidator{})
	want := RoleFilter{Keyword: "ops", Status: 1, Sensitive: 2, ForceOTP: 1}

	if _, _, err := uc.List(context.Background(), want, 40, 0); err != nil {
		t.Fatalf("List: %v", err)
	}
	if repo.lastFilter != want {
		t.Errorf("repo got filter = %+v, want %+v", repo.lastFilter, want)
	}
	if repo.lastOffset != 40 {
		t.Errorf("repo got offset = %d, want 40", repo.lastOffset)
	}
	if repo.lastLimit != defaultPageSize { // limit=0 gets the default page size, not "no limit"
		t.Errorf("repo got limit = %d, want %d", repo.lastLimit, defaultPageSize)
	}
	if _, _, err := uc.List(context.Background(), RoleFilter{}, 0, -1); err != nil {
		t.Fatalf("List: %v", err)
	}
	if repo.lastLimit != defaultPageSize {
		t.Errorf("with limit=-1 repo got %d, want %d", repo.lastLimit, defaultPageSize)
	}
	// The upper bound belongs to the proto's lte; it is passed through unchanged here, not clamped again.
	if _, _, err := uc.List(context.Background(), RoleFilter{}, 0, 150); err != nil {
		t.Fatalf("List: %v", err)
	}
	if repo.lastLimit != 150 {
		t.Errorf("with limit=150 repo got %d, want 150", repo.lastLimit)
	}
}

// A database error must not be reported as "role not found": Get, Update, Delete and PermissionsOf return the
// repo's error unchanged; a role that really does not exist is still ErrRoleNotFound.
func TestRoleUseCase_RepoErrorNotRewrittenAsNotFound(t *testing.T) {
	ctx := context.Background()
	boom := errors.New("connection lost")
	repo := newFakeRoleRepo()
	repo.getErr = boom
	uc := NewRoleUseCase(repo, fakeARBAC{}, &fakeCacheInvalidator{})
	root := &Actor{IsRoot: true}
	calls := map[string]func() error{
		"Get":           func() error { _, err := uc.Get(ctx, 1); return err },
		"Update":        func() error { _, err := uc.Update(ctx, 1, RoleUpdate{Name: "x", Status: 1}); return err },
		"Delete":        func() error { return uc.Delete(ctx, 1) },
		"PermissionsOf": func() error { _, err := uc.PermissionsOf(ctx, root, 1); return err },
	}
	for name, call := range calls {
		if err := call(); !errors.Is(err, boom) {
			t.Errorf("%s: err = %v, want the repo error", name, err)
		}
	}
	repo.getErr = nil
	if _, err := uc.Get(ctx, 999); !errors.Is(err, ErrRoleNotFound) {
		t.Errorf("missing role: err = %v, want ErrRoleNotFound", err)
	}
}
