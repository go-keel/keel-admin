// Example file: delete the whole file after forking

package data

import (
	"context"
	"errors"

	"gorm.io/gorm"
	"gorm.io/gorm/clause"

	"github.com/go-keel/keel-admin/internal/biz"
	"github.com/go-keel/keel-admin/internal/core/perm"
	"github.com/go-keel/keel-admin/internal/core/permdata"
	"github.com/go-keel/keel-admin/internal/core/timefmt"
)

var _ biz.NoticeRepo = (*noticeRepo)(nil)

type noticeRepo struct {
	data *Data
}

// NewNoticeRepo builds the notice repository.
func NewNoticeRepo(data *Data) biz.NoticeRepo { return &noticeRepo{data: data} }

// catCols applies category grants to the notice table: the instance set (ResourceIDs) of the category spec filters
// not this table's primary key but the category_id foreign key. The same grant is a row filter on the category table
// (default id column) and a gate on the notice table. All adds no condition and no grant at all means deny-all; these
// semantics are handled uniformly by ApplyScopeWith.
var catCols = permdata.Cols{PK: "category_id"}

// List applies spec via permdata.ApplyScope's default owner_user_id/
// owner_dept_id columns, AND catSpec via the category_id gate (two Scopes
// AND-combine). categoryID>0 narrows to one category; a category outside the grants intersects with the gate to an
// empty result, so it needs no separate check.
func (r *noticeRepo) List(ctx context.Context, spec, catSpec perm.ScopeSpec, f biz.NoticeFilter, offset, limit int) ([]*biz.Notice, int64, error) {
	q := r.data.DB.WithContext(ctx).Model(&notice{}).
		Scopes(permdata.ApplyScope(spec), permdata.ApplyScopeWith(catSpec, catCols))
	if f.CategoryID > 0 {
		q = q.Where("category_id = ?", f.CategoryID)
	}
	if f.Keyword != "" {
		q = q.Where("title LIKE ?", "%"+f.Keyword+"%")
	}
	if !f.CreatedFrom.IsZero() {
		q = q.Where("created_at >= ?", f.CreatedFrom)
	}
	if !f.CreatedTo.IsZero() {
		q = q.Where("created_at < ?", f.CreatedTo)
	}

	var total int64
	if err := q.Count(&total).Error; err != nil {
		return nil, 0, err
	}
	var rows []notice
	if err := q.Order("id desc").Offset(offset).Limit(limit).Find(&rows).Error; err != nil {
		return nil, 0, err
	}
	out := make([]*biz.Notice, 0, len(rows))
	for i := range rows {
		out = append(out, rows[i].toBiz())
	}
	return out, total, nil
}

// Create first locks the category row by primary key with FOR UPDATE inside one transaction and inserts the notice
// only once it holds the lock; if the row cannot be locked the category is gone, and it returns
// ErrNoticeCategoryNotFound.
//
// Locking the parent row is the other half of a pair with DeleteCategory. Without a foreign key constraint, parent
// and child tables stay consistent only if both sides cooperate: deleting the parent carries a "no children"
// condition (the NOT EXISTS in DeleteCategory), and inserting a child locks the parent first in the same transaction.
// Without this half, "category found → category deleted and committed → notice inserted" still succeeds and leaves a
// notice under a deleted category: NOT EXISTS constrains only the moment of the delete, not an insert that arrives
// after it; checking the category outside the transaction is no better, as it cannot block the gap between the check
// and the insert.
//
// Lock order: category row → notice rows, the same in DeleteCategory (both go through lockCategory). When the delete
// takes the category row first, this waits for its commit, and the subsequent locking read finds no category; when
// this takes it first, the delete waits for this commit, and its NOT EXISTS then sees the notice and deletes nothing.
func (r *noticeRepo) Create(ctx context.Context, n *biz.Notice) error {
	row := newNotice(n)
	err := r.data.DB.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := lockCategory(tx, n.CategoryID); err != nil {
			return err
		}
		return tx.Create(row).Error
	})
	if err != nil {
		return err
	}
	n.ID = row.ID
	n.CreatedAt = timefmt.RFC3339(row.CreatedAt)
	return nil
}

// Get applies both scopes the same way List does — an id outside either
// range is filtered out by the WHERE clause, so First returns
// gorm.ErrRecordNotFound exactly as it would for a nonexistent id (the two
// cases are indistinguishable by design, see biz.NoticeUseCase.Delete).
func (r *noticeRepo) Get(ctx context.Context, spec, catSpec perm.ScopeSpec, id int64) (*biz.Notice, error) {
	var row notice
	err := r.data.DB.WithContext(ctx).Model(&notice{}).
		Scopes(permdata.ApplyScope(spec), permdata.ApplyScopeWith(catSpec, catCols)).
		Where("id = ?", id).First(&row).Error
	if err != nil {
		return nil, err
	}
	return row.toBiz(), nil
}

// Delete scopes the same way Get/List do; RowsAffected==0 covers both a
// nonexistent id and an id outside either scope.
func (r *noticeRepo) Delete(ctx context.Context, spec, catSpec perm.ScopeSpec, id int64) (int64, error) {
	tx := r.data.DB.WithContext(ctx).
		Scopes(permdata.ApplyScope(spec), permdata.ApplyScopeWith(catSpec, catCols)).
		Where("id = ?", id).Delete(&notice{})
	return tx.RowsAffected, tx.Error
}

// errBatchIncomplete only triggers a rollback inside the transaction closure and never leaves this file: the caller
// gets "how many rows were actually deleted", and biz compares that and translates it into a domain error.
var errBatchIncomplete = errors.New("batch delete incomplete")

// BatchDelete deletes a batch with the same scope filter as Delete, but **all or nothing**: if any id misses (does
// not exist, or exists outside the scope), the whole batch rolls back.
//
// There is no partial success because such a result cannot be explained to the person using it: after "2 deleted,
// the 3rd failed", nobody can tell from the refreshed list which ones remain; after a full failure, selecting again
// starts clean.
//
// On rollback, affected still carries the number of rows actually matched, which the caller judges by (see
// biz.BatchDelete).
func (r *noticeRepo) BatchDelete(ctx context.Context, spec, catSpec perm.ScopeSpec, ids []int64) (int64, error) {
	var affected int64
	err := r.data.DB.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		res := tx.Scopes(permdata.ApplyScope(spec), permdata.ApplyScopeWith(catSpec, catCols)).
			Where("id IN ?", ids).Delete(&notice{})
		if res.Error != nil {
			return res.Error
		}
		affected = res.RowsAffected
		if affected != int64(len(ids)) {
			return errBatchIncomplete
		}
		return nil
	})
	if errors.Is(err, errBatchIncomplete) {
		return affected, nil
	}
	return affected, err
}

// HasInDept reports whether dept still owns a notice row — the closure
// NoticeResourceDefs (internal/biz/notice.go) registers into perm's
// registry so biz.DeptUseCase.Delete (core/sys, the kernel) can check
// resource occupancy without ever naming notice itself.
func (r *noticeRepo) HasInDept(ctx context.Context, deptID int64) (bool, error) {
	var n int64
	err := r.data.DB.WithContext(ctx).Model(&notice{}).Where("owner_dept_id = ?", deptID).Count(&n).Error
	return n > 0, err
}

// ListCategories is the category management list: paged, with category rows filtered by catSpec. Here ResourceIDs
// applies to this table's own primary key (ApplyScope's default id column); this and the category_id gate on the
// notice table are two uses of the same grant.
func (r *noticeRepo) ListCategories(ctx context.Context, catSpec perm.ScopeSpec, offset, limit int) ([]*biz.NoticeCategory, int64, error) {
	q := r.data.DB.WithContext(ctx).Model(&noticeCategory{}).
		Scopes(permdata.ApplyScope(catSpec))
	var total int64
	if err := q.Count(&total).Error; err != nil {
		return nil, 0, err
	}
	var rows []noticeCategory
	if err := q.Order("id asc").Offset(offset).Limit(limit).Find(&rows).Error; err != nil {
		return nil, 0, err
	}
	return toCategoryBiz(rows), total, nil
}

// CategoryOptions returns every category visible to the actor: the selector's option set, with the same scope filter
// as ListCategories but unpaged (a dropdown must be able to pick any item).
func (r *noticeRepo) CategoryOptions(ctx context.Context, catSpec perm.ScopeSpec) ([]*biz.NoticeCategory, error) {
	var rows []noticeCategory
	err := r.data.DB.WithContext(ctx).Model(&noticeCategory{}).
		Scopes(permdata.ApplyScope(catSpec)).Order("id asc").Find(&rows).Error
	if err != nil {
		return nil, err
	}
	return toCategoryBiz(rows), nil
}

// AllCategories returns all categories, for the registered ListGrantable callback (a root-only configuration surface,
// see biz.NoticeResourceDefs), with no scope filter.
func (r *noticeRepo) AllCategories(ctx context.Context) ([]*biz.NoticeCategory, error) {
	var rows []noticeCategory
	err := r.data.DB.WithContext(ctx).Model(&noticeCategory{}).Order("id asc").Find(&rows).Error
	if err != nil {
		return nil, err
	}
	return toCategoryBiz(rows), nil
}

func (r *noticeRepo) CreateCategory(ctx context.Context, c *biz.NoticeCategory) error {
	row := newNoticeCategory(c)
	if err := r.data.DB.WithContext(ctx).Create(row).Error; err != nil {
		// The only key this notice_category insert can collide on is uk_name (the primary key auto-increments), so a
		// collision maps directly to "category name exists"; on MySQL the translated error does not say which key
		// collided, so this premise is all there is. The biz pre-check only rejects early; concurrent creation of the
		// same name is caught here.
		if errors.Is(err, gorm.ErrDuplicatedKey) {
			return biz.ErrNoticeCategoryExists
		}
		return err
	}
	c.ID = row.ID
	c.CreatedAt = timefmt.RFC3339(row.CreatedAt)
	return nil
}

// DeleteCategory deletes only when the category has no notices: inside one transaction it first locks the category
// row by primary key with FOR UPDATE, then runs one conditional delete (id = ? AND NOT EXISTS (subquery)). "No
// notices" is part of the delete statement's condition, so check and delete are one statement, with no gap of
// "counted 0, then someone published a notice before the delete".
//
// It is only half of a pair: NOT EXISTS cannot cover a notice inserted after the delete commits. The other half is in
// Create: before inserting a notice it locks the category row in the same transaction, and does not insert if it
// cannot.
//
// The category row is locked separately first to pin the lock order to "category row → notice rows", the same as
// Create. The delete statement alone cannot do that: it reads two tables, and MySQL decides which to lock first. When
// the subquery does not reference the outer row, MySQL runs it first during optimization, taking shared locks on
// notice's category_id index before locking the category row, exactly the reverse of Create; in the real-database
// concurrency test they deadlock within the first round or two. Here the subquery still takes shared locks on that
// category's notices (or the gap), but the category row is already held, and the side inserting a notice is still
// waiting to lock the category row and cannot reach that part of the index.
//
// Failing to lock the category row means the category does not exist (ErrNoticeCategoryNotFound); holding the lock
// while the delete affects 0 rows can only mean the category still has notices (ErrNoticeCategoryInUse).
func (r *noticeRepo) DeleteCategory(ctx context.Context, id int64) error {
	return r.data.DB.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := lockCategory(tx, id); err != nil {
			return err
		}
		res := tx.Where("id = ? AND NOT EXISTS (?)", id,
			tx.Model(&notice{}).Select("1").Where("category_id = ?", id)).
			Delete(&noticeCategory{})
		if res.Error != nil {
			return res.Error
		}
		if res.RowsAffected == 0 {
			return biz.ErrNoticeCategoryInUse
		}
		return nil
	})
}

// lockCategory locks the category row by primary key with FOR UPDATE in tx, returning ErrNoticeCategoryNotFound if
// there is none. Create and DeleteCategory both call it before touching the notice table: this is the notice
// example's lock order (category row → notice rows), and with both sides going through this one place the order
// cannot drift apart.
//
// When the category does not exist, MySQL locks a gap on the primary key; the caller rolls back at once without
// inserting into that gap, so it waits on nobody. sqlite ignores FOR UPDATE (it has no row locks), but "not found in
// the transaction means it does not exist" still holds.
func lockCategory(tx *gorm.DB, id int64) error {
	err := tx.Clauses(clause.Locking{Strength: clause.LockingStrengthUpdate}).
		Select("id").Where("id = ?", id).First(&noticeCategory{}).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return biz.ErrNoticeCategoryNotFound
	}
	return err
}

func (r *noticeRepo) CategoryNameExists(ctx context.Context, name string) (bool, error) {
	var n int64
	err := r.data.DB.WithContext(ctx).Model(&noticeCategory{}).Where("name = ?", name).Count(&n).Error
	return n > 0, err
}
