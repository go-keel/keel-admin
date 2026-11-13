// Example file: delete the whole file after forking

package biz

import (
	"context"
	"errors"
	"testing"

	"github.com/go-keel/keel-admin/internal/core/perm"
)

// fakeNoticeRepo is a minimal in-memory NoticeRepo double for exercising
// NoticeUseCase wiring without a real DB (ApplyScope filtering itself is
// covered by internal/data/notice_integration_test.go against MySQL).
type fakeNoticeRepo struct {
	created        *Notice
	createErr      error
	listSpec       perm.ScopeSpec
	listCatSpec    perm.ScopeSpec
	deleteAffected int64
	batchIDs       []int64
	deleteErr      error
	hasInDept      bool
	hasInDeptErr   error

	categories   []*NoticeCategory
	nameExists   bool
	catDeleteErr error
	catDeleted   []int64
}

func (f *fakeNoticeRepo) List(_ context.Context, spec, catSpec perm.ScopeSpec, _ NoticeFilter, _, _ int) ([]*Notice, int64, error) {
	f.listSpec, f.listCatSpec = spec, catSpec
	return nil, 0, nil
}

func (f *fakeNoticeRepo) Create(_ context.Context, n *Notice) error {
	if f.createErr != nil {
		return f.createErr
	}
	n.ID = 1
	f.created = n
	return nil
}

func (f *fakeNoticeRepo) Get(context.Context, perm.ScopeSpec, perm.ScopeSpec, int64) (*Notice, error) {
	return nil, nil
}

func (f *fakeNoticeRepo) Delete(context.Context, perm.ScopeSpec, perm.ScopeSpec, int64) (int64, error) {
	return f.deleteAffected, f.deleteErr
}

func (f *fakeNoticeRepo) BatchDelete(_ context.Context, _, _ perm.ScopeSpec, ids []int64) (int64, error) {
	f.batchIDs = ids
	return f.deleteAffected, f.deleteErr
}

func (f *fakeNoticeRepo) HasInDept(context.Context, int64) (bool, error) {
	return f.hasInDept, f.hasInDeptErr
}

func (f *fakeNoticeRepo) ListCategories(_ context.Context, _ perm.ScopeSpec, offset, limit int) ([]*NoticeCategory, int64, error) {
	total := int64(len(f.categories))
	if offset >= len(f.categories) {
		return nil, total, nil
	}
	return f.categories[offset:min(offset+limit, len(f.categories))], total, nil
}

func (f *fakeNoticeRepo) CategoryOptions(context.Context, perm.ScopeSpec) ([]*NoticeCategory, error) {
	return f.categories, nil
}

func (f *fakeNoticeRepo) AllCategories(context.Context) ([]*NoticeCategory, error) {
	return f.categories, nil
}

func (f *fakeNoticeRepo) CreateCategory(_ context.Context, c *NoticeCategory) error {
	c.ID = 9
	return nil
}

func (f *fakeNoticeRepo) DeleteCategory(_ context.Context, id int64) error {
	f.catDeleted = append(f.catDeleted, id)
	return f.catDeleteErr
}

func (f *fakeNoticeRepo) CategoryNameExists(context.Context, string) (bool, error) {
	return f.nameExists, nil
}

// fakeScopeResolver is a fake perm.ScopeResolver — the real implementation
// (core/sys/biz.ScopeUseCase) lives in the kernel and is injected into
// notice via the perm.ScopeResolver interface (see cmd/admin/di.go).
// specs are keyed by resource type: the notice row-level spec and the category grant spec are resolved separately.
type fakeScopeResolver struct {
	specs map[perm.Resource]perm.ScopeSpec
	err   error
}

func (f *fakeScopeResolver) Resolve(_ context.Context, _ *perm.Principal, res perm.Resource) (perm.ScopeSpec, error) {
	return f.specs[res], f.err
}

// fakeRevoker records the revocation the use case requests through the
// perm.GrantRevoker port.
type fakeRevoker struct {
	res perm.Resource
	id  int64
}

func (f *fakeRevoker) RevokeGrants(_ context.Context, res perm.Resource, id int64) error {
	f.res, f.id = res, id
	return nil
}

func allScopes() *fakeScopeResolver {
	return &fakeScopeResolver{specs: map[perm.Resource]perm.ScopeSpec{
		ResNotice:         {All: true},
		ResNoticeCategory: {All: true},
	}}
}

func TestNoticeCreateOwnerFromActor(t *testing.T) {
	repo := &fakeNoticeRepo{}
	uc := NewNoticeUseCase(repo, allScopes(), &fakeRevoker{})
	actor := &perm.Principal{UserID: 7, DeptID: 20}

	// Create's signature takes no owner argument — owner must come from
	// actor, not any client-supplied value.
	n, err := uc.Create(context.Background(), actor, "title", "content", 3)
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	if n.OwnerUserID != 7 || n.OwnerDeptID != 20 {
		t.Fatalf("owner = (%d,%d), want (7,20) from actor", n.OwnerUserID, n.OwnerDeptID)
	}
	if repo.created.OwnerUserID != 7 || repo.created.OwnerDeptID != 20 {
		t.Fatalf("repo.Create got owner = (%d,%d), want (7,20)", repo.created.OwnerUserID, repo.created.OwnerDeptID)
	}
	if repo.created.CategoryID != 3 {
		t.Fatalf("repo.Create got category %d, want 3", repo.created.CategoryID)
	}
}

// An invisible category (outside the grants) and a missing one both report ErrNoticeCategoryNotFound, so the
// existence of a category does not leak to someone without access.
func TestNoticeCreateCategoryGate(t *testing.T) {
	repo := &fakeNoticeRepo{}
	scoped := &fakeScopeResolver{specs: map[perm.Resource]perm.ScopeSpec{
		ResNotice:         {All: true},
		ResNoticeCategory: {ResourceIDs: []int64{3}}, // only category 3 granted
	}}
	uc := NewNoticeUseCase(repo, scoped, &fakeRevoker{})
	actor := &perm.Principal{UserID: 7, DeptID: 20}

	if _, err := uc.Create(context.Background(), actor, "t", "c", 5); !errors.Is(err, ErrNoticeCategoryNotFound) {
		t.Fatalf("create into invisible category: got %v, want ErrNoticeCategoryNotFound", err)
	}
	if _, err := uc.Create(context.Background(), actor, "t", "c", 3); err != nil {
		t.Fatalf("create into granted category: %v", err)
	}
	// Category grants deny-all (zero-value spec) → no category accepts a post
	denied := &fakeScopeResolver{specs: map[perm.Resource]perm.ScopeSpec{ResNotice: {All: true}}}
	uc = NewNoticeUseCase(repo, denied, &fakeRevoker{})
	if _, err := uc.Create(context.Background(), actor, "t", "c", 3); !errors.Is(err, ErrNoticeCategoryNotFound) {
		t.Fatalf("create under deny-all category scope: got %v, want ErrNoticeCategoryNotFound", err)
	}
	// Visible by grant but the row is gone: repo cannot lock the category row in its transaction and reports the same
	// error, passed through as is
	repo.createErr = ErrNoticeCategoryNotFound
	uc = NewNoticeUseCase(repo, allScopes(), &fakeRevoker{})
	if _, err := uc.Create(context.Background(), actor, "t", "c", 3); !errors.Is(err, ErrNoticeCategoryNotFound) {
		t.Fatalf("create into nonexistent category: got %v, want ErrNoticeCategoryNotFound", err)
	}
}

func TestNoticeDeleteNotFound(t *testing.T) {
	repo := &fakeNoticeRepo{deleteAffected: 0}
	uc := NewNoticeUseCase(repo, allScopes(), &fakeRevoker{})
	actor := &perm.Principal{UserID: 1}

	err := uc.Delete(context.Background(), actor, 99)
	if !errors.Is(err, ErrNoticeNotFound) {
		t.Fatalf("Delete affected=0 error = %v, want ErrNoticeNotFound", err)
	}
}

func TestNoticeDeleteSuccess(t *testing.T) {
	repo := &fakeNoticeRepo{deleteAffected: 1}
	uc := NewNoticeUseCase(repo, allScopes(), &fakeRevoker{})
	actor := &perm.Principal{UserID: 1}

	if err := uc.Delete(context.Background(), actor, 1); err != nil {
		t.Fatalf("Delete: %v", err)
	}
}

// Batch delete is all or nothing: repo reports "only 2 rows matched" while the caller selected 3, so the whole batch
// fails. This guards the condition itself: written as "fail only when affected == 0", a partial match would pass,
// which is exactly the in-between state of "2 deleted, the 3rd failed".
func TestNoticeBatchDeletePartial(t *testing.T) {
	repo := &fakeNoticeRepo{deleteAffected: 2}
	uc := NewNoticeUseCase(repo, allScopes(), &fakeRevoker{})
	actor := &perm.Principal{UserID: 1}

	err := uc.BatchDelete(context.Background(), actor, []int64{1, 2, 3})
	if !errors.Is(err, ErrNoticeNotFound) {
		t.Fatalf("BatchDelete affected=2 of 3 error = %v, want ErrNoticeNotFound", err)
	}
}

func TestNoticeBatchDeleteSuccess(t *testing.T) {
	repo := &fakeNoticeRepo{deleteAffected: 3}
	uc := NewNoticeUseCase(repo, allScopes(), &fakeRevoker{})
	actor := &perm.Principal{UserID: 1}

	if err := uc.BatchDelete(context.Background(), actor, []int64{1, 2, 3}); err != nil {
		t.Fatalf("BatchDelete: %v", err)
	}
	if len(repo.batchIDs) != 3 {
		t.Fatalf("ids received by repo = %v, want 3", repo.batchIDs)
	}
}

// TestNoticeResourceDefs: two resources, notice (row-level scope + DeptOccupancy closure) and notice_category
// (explicit assignment: Anchors=0 + ListGrantable closure). Both closures must forward to repo: this is the business
// half of the inverted dependency (the kernel only calls back through the registry and knows no business table names).
func TestNoticeResourceDefs(t *testing.T) {
	repo := &fakeNoticeRepo{hasInDept: true, categories: []*NoticeCategory{{ID: 3, Name: "Operations"}}}
	defs := NoticeResourceDefs(repo)
	if len(defs) != 2 {
		t.Fatalf("NoticeResourceDefs() len = %d, want 2", len(defs))
	}
	notice, category := defs[0], defs[1]
	if notice.Res != ResNotice || category.Res != ResNoticeCategory {
		t.Fatalf("resources = (%q,%q), want (%q,%q)", notice.Res, category.Res, ResNotice, ResNoticeCategory)
	}
	if notice.DeptOccupancy == nil {
		t.Fatal("notice.DeptOccupancy is nil, want a closure over repo.HasInDept")
	}
	occupied, err := notice.DeptOccupancy(context.Background(), 42)
	if err != nil || !occupied {
		t.Fatalf("DeptOccupancy = (%v,%v), want (true,nil)", occupied, err)
	}

	// category: Anchors=0 (SELF/DEPT/ALL_SUB cannot be configured; the UI offers only explicit assignment and ALL),
	// and ListGrantable forwards to repo.AllCategories.
	if category.Anchors != 0 {
		t.Fatalf("category.Anchors = %v, want 0", category.Anchors)
	}
	if category.ListGrantable == nil {
		t.Fatal("category.ListGrantable is nil, want a closure over repo.AllCategories")
	}
	opts, err := category.ListGrantable(context.Background())
	if err != nil {
		t.Fatalf("ListGrantable: %v", err)
	}
	if len(opts) != 1 || opts[0].ID != 3 || opts[0].Label != "Operations" {
		t.Fatalf("ListGrantable = %+v, want [{3 Operations}]", opts)
	}
}

func TestNoticeListResolvesScope(t *testing.T) {
	repo := &fakeNoticeRepo{}
	resolver := &fakeScopeResolver{specs: map[perm.Resource]perm.ScopeSpec{
		ResNotice:         {SelfUserID: 5},
		ResNoticeCategory: {ResourceIDs: []int64{3}},
	}}
	uc := NewNoticeUseCase(repo, resolver, &fakeRevoker{})
	actor := &perm.Principal{UserID: 5}

	if _, _, err := uc.List(context.Background(), actor, NoticeFilter{}, 0, 20); err != nil {
		t.Fatalf("List: %v", err)
	}
	if repo.listSpec.SelfUserID != 5 {
		t.Fatalf("repo.List got spec = %+v, want SelfUserID=5 (resolved via scope.Resolve)", repo.listSpec)
	}
	// The category grant spec and the row-level spec are resolved separately, and both must reach the data layer.
	if len(repo.listCatSpec.ResourceIDs) != 1 || repo.listCatSpec.ResourceIDs[0] != 3 {
		t.Fatalf("repo.List got catSpec = %+v, want ResourceIDs=[3]", repo.listCatSpec)
	}
}

// Deleting a category: repo's conditional delete decides "in use by notices / missing", which the use case passes
// through without cleaning up grants; only a successful delete cleans up the grant rows through the GrantRevoker port.
func TestNoticeDeleteCategory(t *testing.T) {
	repo := &fakeNoticeRepo{catDeleteErr: ErrNoticeCategoryInUse}
	revoker := &fakeRevoker{}
	uc := NewNoticeUseCase(repo, allScopes(), revoker)
	actor := &perm.Principal{UserID: 7, DeptID: 20}

	if err := uc.DeleteCategory(context.Background(), actor, 3); !errors.Is(err, ErrNoticeCategoryInUse) {
		t.Fatalf("delete in-use category: got %v, want ErrNoticeCategoryInUse", err)
	}
	if revoker.id != 0 {
		t.Fatal("in-use category must not trigger revocation")
	}

	repo.catDeleteErr = ErrNoticeCategoryNotFound
	if err := uc.DeleteCategory(context.Background(), actor, 9); !errors.Is(err, ErrNoticeCategoryNotFound) {
		t.Fatalf("delete nonexistent category: got %v, want ErrNoticeCategoryNotFound", err)
	}
	if revoker.id != 0 {
		t.Fatal("nonexistent category must not trigger revocation")
	}

	repo.catDeleteErr = nil
	if err := uc.DeleteCategory(context.Background(), actor, 3); err != nil {
		t.Fatalf("DeleteCategory: %v", err)
	}
	if revoker.res != ResNoticeCategory || revoker.id != 3 {
		t.Fatalf("revoked (%q,%d), want (%q,3)", revoker.res, revoker.id, ResNoticeCategory)
	}
}

// A category outside the actor's grants reports ErrNoticeCategoryNotFound, like a missing one, and is not deleted:
// whoever manages only category 3 cannot delete category 5 by id.
func TestNoticeDeleteCategoryOutsideGrants(t *testing.T) {
	repo := &fakeNoticeRepo{}
	revoker := &fakeRevoker{}
	scoped := &fakeScopeResolver{specs: map[perm.Resource]perm.ScopeSpec{
		ResNotice:         {All: true},
		ResNoticeCategory: {ResourceIDs: []int64{3}},
	}}
	uc := NewNoticeUseCase(repo, scoped, revoker)
	actor := &perm.Principal{UserID: 7, DeptID: 20}

	if err := uc.DeleteCategory(context.Background(), actor, 5); !errors.Is(err, ErrNoticeCategoryNotFound) {
		t.Fatalf("delete category outside grants: got %v, want ErrNoticeCategoryNotFound", err)
	}
	if len(repo.catDeleted) != 0 || revoker.id != 0 {
		t.Fatalf("category outside grants reached repo %v / revoker %d, want neither", repo.catDeleted, revoker.id)
	}
	if err := uc.DeleteCategory(context.Background(), actor, 3); err != nil {
		t.Fatalf("delete granted category: %v", err)
	}
	if len(repo.catDeleted) != 1 || repo.catDeleted[0] != 3 {
		t.Fatalf("repo deleted %v, want [3]", repo.catDeleted)
	}
}

// Creating a category: a duplicate name is rejected (categories are explicit-assignment options, and equal names
// could not be told apart in the selector).
func TestNoticeCreateCategoryDuplicate(t *testing.T) {
	repo := &fakeNoticeRepo{nameExists: true}
	uc := NewNoticeUseCase(repo, allScopes(), &fakeRevoker{})
	if _, err := uc.CreateCategory(context.Background(), "Operations"); !errors.Is(err, ErrNoticeCategoryExists) {
		t.Fatalf("duplicate category name: got %v, want ErrNoticeCategoryExists", err)
	}
	repo.nameExists = false
	c, err := uc.CreateCategory(context.Background(), "Operations")
	if err != nil || c.ID == 0 {
		t.Fatalf("CreateCategory = (%+v,%v), want created", c, err)
	}
}
