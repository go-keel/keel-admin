//go:build integration

// Example file: delete the whole file after forking

package data

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"testing"
	"time"

	"gorm.io/gorm"

	"github.com/go-keel/keel-admin/internal/biz"
	"github.com/go-keel/keel-admin/internal/core/perm"
)

// The tests in this file run under make integration, each on a fresh MySQL database built from
// migrations/*/0_schema.sql (testdb_integration_test.go).

// When publishing a notice and deleting its category arrive at the same time, the outcome is one of exactly two:
// publish succeeds and delete reports "category still has notices", or delete succeeds and publish reports "category
// not found". Both succeeding must never happen: that is a notice under a deleted category. Each side holds half of
// the constraint (Create locks the category row before inserting; DeleteCategory locks the category row before
// deleting with NOT EXISTS), and losing either half breaks it.
func TestNoticeCreateVsDeleteCategoryConcurrent(t *testing.T) {
	db := openTestDB(t)
	repo := &noticeRepo{data: &Data{DB: db}}
	ctx := context.Background()

	prefix := fmt.Sprintf("txrace_%d_", time.Now().UnixNano())
	t.Cleanup(func() {
		db.Exec("DELETE FROM notice WHERE title LIKE ?", prefix+"%")
		db.Exec("DELETE FROM notice_category WHERE name LIKE ?", prefix+"%")
	})

	// The round count was measured: on a local MySQL 8.4 both regressions show well within 500 rounds. With Create
	// changed to a plain unlocked read, orphan notices appear within the first dozen rounds; with DeleteCategory not
	// locking the category row first and keeping only the conditional delete, it deadlocks within the first few rounds
	// (landing in the default branch).
	const rounds = 500
	var published, deleted int
	for round := range rounds {
		// A new empty category each round, so the delete side has a chance to succeed.
		cat := &biz.NoticeCategory{Name: fmt.Sprintf("%s%d", prefix, round)}
		if err := repo.CreateCategory(ctx, cat); err != nil {
			t.Fatalf("round %d: seed category: %v", round, err)
		}
		n := &biz.Notice{Title: prefix + "notice", Content: "race", CategoryID: cat.ID, OwnerUserID: 1, OwnerDeptID: 1}

		start := make(chan struct{})
		var wg sync.WaitGroup
		var createErr, deleteErr error
		wg.Go(func() {
			<-start
			createErr = repo.Create(ctx, n)
		})
		wg.Go(func() {
			<-start
			deleteErr = repo.DeleteCategory(ctx, cat.ID)
		})
		close(start)
		wg.Wait()

		switch {
		case createErr == nil && errors.Is(deleteErr, biz.ErrNoticeCategoryInUse):
			published++
		case errors.Is(createErr, biz.ErrNoticeCategoryNotFound) && deleteErr == nil:
			deleted++
		case createErr == nil && deleteErr == nil:
			t.Fatalf("round %d: category %d was deleted, yet notice %d was inserted", round, cat.ID, n.ID)
		default:
			t.Fatalf("round %d: unexpected result: Create = %v, DeleteCategory = %v", round, createErr, deleteErr)
		}
	}
	t.Logf("%d rounds: publish first %d times, delete first %d times", rounds, published, deleted)
}

// allCat is the "category grants unrestricted" spec: most tests verify only the row-level scope and bypass the
// category gate with it; the gate itself is tested in TestNoticeListCategoryGate.
var allCat = perm.ScopeSpec{All: true}

// newNoticeRepo opens a fresh test database and seeds owner=1/dept=10/cat=1, owner=2/dept=10/cat=1,
// owner=3/dept=99/cat=2.
func newNoticeRepo(t *testing.T) *noticeRepo {
	t.Helper()
	db := openTestDB(t)
	cats := []noticeCategory{{Name: "Operations"}, {Name: "Engineering"}}
	if err := db.Create(&cats).Error; err != nil {
		t.Fatalf("seed categories: %v", err)
	}
	rows := []notice{
		{Title: "t1", Content: "c1", CategoryID: 1, OwnerUserID: 1, OwnerDeptID: 10},
		{Title: "t2", Content: "c2", CategoryID: 1, OwnerUserID: 2, OwnerDeptID: 10},
		{Title: "t3", Content: "c3", CategoryID: 2, OwnerUserID: 3, OwnerDeptID: 99},
	}
	if err := db.Create(&rows).Error; err != nil {
		t.Fatalf("seed: %v", err)
	}
	return &noticeRepo{data: &Data{DB: db}}
}

// categoryExists reports whether the category row still exists, by primary key (for assertions).
func categoryExists(t *testing.T, repo *noticeRepo, id int64) bool {
	t.Helper()
	var n int64
	if err := repo.data.DB.Model(&noticeCategory{}).Where("id = ?", id).Count(&n).Error; err != nil {
		t.Fatalf("count category %d: %v", id, err)
	}
	return n > 0
}

func TestNoticeListSelfScope(t *testing.T) {
	repo := newNoticeRepo(t)
	got, total, err := repo.List(context.Background(), perm.ScopeSpec{SelfUserID: 1}, allCat, biz.NoticeFilter{}, 0, 20)
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if total != 1 || len(got) != 1 || got[0].OwnerUserID != 1 {
		t.Fatalf("SELF should see only owner=1, got total=%d items=%d", total, len(got))
	}
}

// Category gate: catSpec's ResourceIDs apply to the category_id column
// (ApplyScopeWith(catSpec, Cols{PK: "category_id"})), intersected with the row-level scope.
func TestNoticeListCategoryGate(t *testing.T) {
	repo := newNoticeRepo(t)
	all := perm.ScopeSpec{All: true}

	// Only category 1 granted → only the two rows with cat=1
	got, total, err := repo.List(context.Background(), all, perm.ScopeSpec{ResourceIDs: []int64{1}}, biz.NoticeFilter{}, 0, 20)
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if total != 2 || len(got) != 2 {
		t.Fatalf("a grant on category 1 should see 2 rows, got total=%d items=%d", total, len(got))
	}
	for _, n := range got {
		if n.CategoryID != 1 {
			t.Fatalf("leaked a notice outside the granted categories: %+v", n)
		}
	}

	// Category grants deny-all (zero-value spec) → nothing visible, even with the row-level scope ALL
	_, total, err = repo.List(context.Background(), all, perm.ScopeSpec{}, biz.NoticeFilter{}, 0, 20)
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if total != 0 {
		t.Fatalf("category deny-all should see 0 rows, got %d", total)
	}

	// Row-level scope intersects category grants: SELF(3) ∩ category{1} = empty (t3 is in category 2)
	_, total, err = repo.List(context.Background(), perm.ScopeSpec{SelfUserID: 3}, perm.ScopeSpec{ResourceIDs: []int64{1}}, biz.NoticeFilter{}, 0, 20)
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if total != 0 {
		t.Fatalf("SELF(3) ∩ category{1} should be empty, got %d", total)
	}

	// The categoryID filter is outside the grants (category 2 requested, only category 1 granted) → empty intersection
	_, total, err = repo.List(context.Background(), all, perm.ScopeSpec{ResourceIDs: []int64{1}}, biz.NoticeFilter{CategoryID: 2}, 0, 20)
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if total != 0 {
		t.Fatalf("requesting a category outside the grants should be empty, got %d", total)
	}
}

// The creation time range is half-open: a row exactly at the end is not included (ADR admin-0003).
func TestNoticeListCreatedRangeHalfOpen(t *testing.T) {
	repo := newNoticeRepo(t)
	at := time.Date(2026, 9, 1, 16, 0, 0, 0, time.UTC)
	if err := repo.data.DB.Model(&notice{}).Where("id = ?", 1).Update("created_at", at).Error; err != nil {
		t.Fatalf("seed created_at: %v", err)
	}
	all := perm.ScopeSpec{All: true}
	count := func(f biz.NoticeFilter) int64 {
		_, total, err := repo.List(context.Background(), all, allCat, f, 0, 20)
		if err != nil {
			t.Fatalf("List: %v", err)
		}
		return total
	}
	if n := count(biz.NoticeFilter{CreatedFrom: at, CreatedTo: at.Add(time.Second)}); n != 1 {
		t.Fatalf("start inclusive: got %d, want 1", n)
	}
	if n := count(biz.NoticeFilter{CreatedFrom: at.Add(-time.Hour), CreatedTo: at}); n != 0 {
		t.Fatalf("end exclusive: got %d, want 0", n)
	}
}

func TestNoticeGetOutOfScopeIs404(t *testing.T) {
	repo := newNoticeRepo(t)
	_, err := repo.Get(context.Background(), perm.ScopeSpec{SelfUserID: 1}, allCat, 2) // owner=2 is outside SELF(1)
	if !errors.Is(err, gorm.ErrRecordNotFound) {
		t.Fatalf("an id outside the scope should be 404 (ErrRecordNotFound), got %v", err)
	}
	// Visible by row but not by category → also 404 (both dimensions are scopes; being outside either is
	// indistinguishable)
	_, err = repo.Get(context.Background(), perm.ScopeSpec{SelfUserID: 1}, perm.ScopeSpec{ResourceIDs: []int64{2}}, 1)
	if !errors.Is(err, gorm.ErrRecordNotFound) {
		t.Fatalf("an id outside the category scope should be 404, got %v", err)
	}
}

func TestNoticeCreateAssignsID(t *testing.T) {
	repo := newNoticeRepo(t)
	n := &biz.Notice{Title: "new", Content: "body", CategoryID: 1, OwnerUserID: 1, OwnerDeptID: 10}
	if err := repo.Create(context.Background(), n); err != nil {
		t.Fatalf("Create: %v", err)
	}
	if n.ID == 0 {
		t.Fatalf("expected assigned ID after Create")
	}
}

// Publishing to a category that does not exist: the transaction cannot lock the category row, returns
// ErrNoticeCategoryNotFound, and inserts no notice.
func TestNoticeCreateMissingCategory(t *testing.T) {
	repo := newNoticeRepo(t)
	ctx := context.Background()
	n := &biz.Notice{Title: "orphan", Content: "body", CategoryID: 99, OwnerUserID: 1, OwnerDeptID: 10}
	if err := repo.Create(ctx, n); !errors.Is(err, biz.ErrNoticeCategoryNotFound) {
		t.Fatalf("Create(missing category) = %v, want ErrNoticeCategoryNotFound", err)
	}
	var count int64
	if err := repo.data.DB.Model(&notice{}).Where("category_id = ?", 99).Count(&count).Error; err != nil || count != 0 {
		t.Fatalf("no notice should be inserted under a missing category: count = %d, err = %v", count, err)
	}
}

// TestNoticeHasInDept: the fixture seeds owner_dept_id 10 (twice) and 99 —
// this is the closure repo.HasInDept backs for NoticeResourceDefs's
// DeptOccupancy callback (see internal/biz/notice.go), which
// core/sys/biz.DeptUseCase.Delete calls through perm.ResourcesWithDeptOccupancy
// without ever naming notice itself.
func TestNoticeHasInDept(t *testing.T) {
	repo := newNoticeRepo(t)

	got, err := repo.HasInDept(context.Background(), 10)
	if err != nil {
		t.Fatalf("HasInDept(10): %v", err)
	}
	if !got {
		t.Fatal("HasInDept(10) = false, want true (two seeded rows own dept 10)")
	}

	got, err = repo.HasInDept(context.Background(), 12345)
	if err != nil {
		t.Fatalf("HasInDept(12345): %v", err)
	}
	if got {
		t.Fatal("HasInDept(12345) = true, want false (no notice owned by dept 12345)")
	}
}

func TestNoticeDeleteAffectedRows(t *testing.T) {
	repo := newNoticeRepo(t)
	// id=1 owner=1 is inside SELF(1), so it can be deleted.
	affected, err := repo.Delete(context.Background(), perm.ScopeSpec{SelfUserID: 1}, allCat, 1)
	if err != nil {
		t.Fatalf("Delete: %v", err)
	}
	if affected != 1 {
		t.Fatalf("affected = %d, want 1", affected)
	}
	// id=2 owner=2 is outside SELF(1), so affected should be 0.
	affected, err = repo.Delete(context.Background(), perm.ScopeSpec{SelfUserID: 1}, allCat, 2)
	if err != nil {
		t.Fatalf("Delete: %v", err)
	}
	if affected != 0 {
		t.Fatalf("out-of-scope delete affected = %d, want 0", affected)
	}
}

// Category CRUD: CategoryOptions and ListCategories both filter this table's rows by catSpec (ResourceIDs is the
// default id column); the former is unpaged (selector options), the latter paged (management list). AllCategories
// returns everything unfiltered (the data behind the registered ListGrantable callback).
func TestNoticeCategoryQueries(t *testing.T) {
	repo := newNoticeRepo(t)
	ctx := context.Background()

	all, err := repo.AllCategories(ctx)
	if err != nil || len(all) != 2 {
		t.Fatalf("AllCategories = (%d,%v), want 2", len(all), err)
	}

	opts, err := repo.CategoryOptions(ctx, perm.ScopeSpec{ResourceIDs: []int64{2}})
	if err != nil {
		t.Fatalf("CategoryOptions: %v", err)
	}
	if len(opts) != 1 || opts[0].ID != 2 {
		t.Fatalf("CategoryOptions(grant{2}) = %+v, want only id=2", opts)
	}

	// The management list's total must be the filtered count, not the table count: with the table count as total, the
	// pager shows pages that turn out empty when opened.
	visible, total, err := repo.ListCategories(ctx, perm.ScopeSpec{ResourceIDs: []int64{2}}, 0, 20)
	if err != nil {
		t.Fatalf("ListCategories: %v", err)
	}
	if len(visible) != 1 || visible[0].ID != 2 || total != 1 {
		t.Fatalf("ListCategories(grant{2}) = %+v/total=%d, want only id=2/1", visible, total)
	}

	none, _, err := repo.ListCategories(ctx, perm.ScopeSpec{}, 0, 20)
	if err != nil {
		t.Fatalf("ListCategories: %v", err)
	}
	if len(none) != 0 {
		t.Fatalf("deny-all should see 0 categories, got %d", len(none))
	}

	dup, err := repo.CategoryNameExists(ctx, "Operations")
	if err != nil || !dup {
		t.Fatalf("CategoryNameExists(Operations) = (%v,%v), want true", dup, err)
	}
}

// Deleting a category is one conditional delete: with notices still in it, nothing is deleted (the category row
// stays) and it returns ErrNoticeCategoryInUse; a missing category returns ErrNoticeCategoryNotFound.
// Creating a category that collides on uk_name returns ErrNoticeCategoryExists: the biz pre-check cannot stop
// concurrent creation of the same name, and once the unique key decides, the result must be a domain error, not a 500.
func TestNoticeCreateCategoryDuplicateName(t *testing.T) {
	repo := newNoticeRepo(t)
	err := repo.CreateCategory(context.Background(), &biz.NoticeCategory{Name: "Operations"})
	if !errors.Is(err, biz.ErrNoticeCategoryExists) {
		t.Fatalf("creating a category with an existing name = %v, want ErrNoticeCategoryExists", err)
	}
}

func TestNoticeDeleteCategoryInUse(t *testing.T) {
	repo := newNoticeRepo(t)
	ctx := context.Background()

	// In the seed, category 1 has t1 and t2, category 2 has t3.
	if err := repo.DeleteCategory(ctx, 1); !errors.Is(err, biz.ErrNoticeCategoryInUse) {
		t.Fatalf("DeleteCategory(category with notices) = %v, want ErrNoticeCategoryInUse", err)
	}
	if !categoryExists(t, repo, 1) {
		t.Fatalf("the category row must not be deleted while it has notices")
	}

	if err := repo.DeleteCategory(ctx, 99); !errors.Is(err, biz.ErrNoticeCategoryNotFound) {
		t.Fatalf("DeleteCategory(missing category) = %v, want ErrNoticeCategoryNotFound", err)
	}
}

// An empty category is deleted; after its notices are deleted, a category that could not be deleted before can be.
func TestNoticeDeleteCategoryEmpty(t *testing.T) {
	repo := newNoticeRepo(t)
	ctx := context.Background()

	cat := &biz.NoticeCategory{Name: "Empty"}
	if err := repo.CreateCategory(ctx, cat); err != nil {
		t.Fatalf("CreateCategory: %v", err)
	}
	if err := repo.DeleteCategory(ctx, cat.ID); err != nil {
		t.Fatalf("DeleteCategory(empty category) = %v, want nil", err)
	}
	if categoryExists(t, repo, cat.ID) {
		t.Fatalf("the empty category should have been deleted")
	}

	// Category 2 has only t3 (id=3); after deleting that notice, category 2 is empty and can be deleted.
	if _, err := repo.Delete(ctx, perm.ScopeSpec{All: true}, allCat, 3); err != nil {
		t.Fatalf("Delete notice: %v", err)
	}
	if err := repo.DeleteCategory(ctx, 2); err != nil {
		t.Fatalf("DeleteCategory(emptied category) = %v, want nil", err)
	}
}
