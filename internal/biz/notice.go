// Example file: delete the whole file after forking

package biz

import (
	"context"
	"slices"
	"time"

	"github.com/go-keel/keel-admin/internal/core/perm"
)

// ResNotice is the resource_type notice registers for dimension-two data
// scope (see NoticeResources, registered via perm.MustRegister in
// cmd/admin/di.go).
const ResNotice perm.Resource = "notice"

// ResNoticeCategory registers "category" as an explicit-assignment resource (the RESOURCE_GRANT example):
// Anchors=0 (no owner columns, so SELF/DEPT/ALL_SUB cannot be configured) plus a ListGrantable callback, leaving the
// role data scope UI with only two settings for it: explicit assignment (ticking specific categories) and all.
// Notice visibility = row-level scope (whose notices) ∩ category grants (which categories), see List/Get/Delete.
const ResNoticeCategory perm.Resource = "notice_category"

// Permission codes. A code always has three segments `<business domain>:<feature module>:<feature operation>`; here
// the business domain is "ops" (operations) and the feature module "notice". Business codes must not use the "sys"
// business domain: perm.Seal rejects "sys:" codes registered outside the kernel batch (see MustRegister vs
// MustRegisterKernel).
//
// Write the module segment explicitly even with only one level of grouping (ops:notice:view, not notice:view): with a
// variable number of segments the UI grows branches of uneven depth, and "which module does this code belong to" has
// no single answer.
const (
	PermNoticeView     perm.Code = "ops:notice:view"
	PermNoticeCreate   perm.Code = "ops:notice:create"
	PermNoticeDelete   perm.Code = "ops:notice:delete"
	PermNoticeCategory perm.Code = "ops:notice:category" // manage categories (create/delete)
)

// Default page size (shared by the notice list and the category management list). The maximum is not in code: it is
// the lte on the proto's limit, enforced at the transport entry, so it is not clamped again here.
const defaultNoticePageSize = 20

// NoticeGroups declares notice's business domain / feature module groups (the display name's i18n key derives from
// the slug, see perm.Group.NameKey; entries go in internal/i18n/app.*.toml).
//
// The business domain is declared here too: ops has only the notice module so far, and whoever comes first declares
// it. Another module joining ops declares only its own Module, never the Domain again (registering the same group
// twice panics).
var NoticeGroups = []perm.Group{
	{Domain: "ops", Sort: 10},
	{Domain: "ops", Module: "notice", Sort: 0},
}

// NoticePerms declares notice's functional permissions: create/delete both
// require view through the permission dependency closure.
var NoticePerms = []perm.Permission{
	{Code: PermNoticeView},
	{Code: PermNoticeCreate, Deps: []perm.Code{PermNoticeView}},
	{Code: PermNoticeDelete, Deps: []perm.Code{PermNoticeView}},
	{Code: PermNoticeCategory, Deps: []perm.Code{PermNoticeView}},
}

// NoticeResourceDefs declares notice as a dimension-two data-scope resource
// anchored on both the owner user and the owner dept — notice uses
// permdata.ApplyScope's defaults (owner_user_id/owner_dept_id) unmodified
// (contrast a non-default column mapping, which needs permdata.ApplyScopeWith
// to remap onto different columns).
//
// It also fills DeptOccupancy: a closure over repo that answers "does this
// dept still own a notice row" (queries notice.owner_dept_id via
// repo.HasInDept). This is how the inverted dependency of the department occupancy check works end to end —
// biz.DeptUseCase.Delete (core/sys, the kernel) never names notice or
// any other business table; it iterates perm.ResourcesWithDeptOccupancy()
// and calls back into whatever business domain registered a callback here.
// A function (not a package var) because the closure needs repo, which only
// exists once cmd/admin/di.go has constructed it — see the call site there.
func NoticeResourceDefs(repo NoticeRepo) []perm.ResourceDef {
	return []perm.ResourceDef{
		{
			Res:     ResNotice,
			NameKey: "resource.notice",
			Anchors: perm.AnchorSelf | perm.AnchorDept,
			DeptOccupancy: func(ctx context.Context, deptID int64) (bool, error) {
				return repo.HasInDept(ctx, deptID)
			},
		},
		{
			// Category: the example of an explicit-assignment (RESOURCE_GRANT) resource. Anchors=0: the category
			// table has no owner columns, so SELF/DEPT/ALL_SUB cannot be configured, leaving explicit assignment and
			// ALL. ListGrantable provides the grantable instance options (the multi-select in role configuration);
			// the closure holds the business's own repo, so the kernel neither imports business packages nor knows
			// business table names, the same dependency direction as DeptOccupancy. The options are all categories,
			// not filtered by actor: this is a root-only configuration surface (meta ListResources), not a business
			// query surface.
			Res:     ResNoticeCategory,
			NameKey: "resource.notice_category",
			ListGrantable: func(ctx context.Context) ([]perm.GrantOption, error) {
				cats, err := repo.AllCategories(ctx)
				if err != nil {
					return nil, err
				}
				opts := make([]perm.GrantOption, 0, len(cats))
				for _, c := range cats {
					opts = append(opts, perm.GrantOption{ID: c.ID, Label: c.Name})
				}
				return opts, nil
			},
		},
	}
}

// Notice is a notice/announcement business resource: the owner_user_id/
// owner_dept_id columns are the default scope columns permdata.ApplyScope
// filters on. CategoryID places the notice in a category; category grants (the explicit-assignment dimension)
// constrain notice visibility through it.
type Notice struct {
	ID          int64
	Title       string
	Content     string
	CategoryID  int64
	OwnerUserID int64
	OwnerDeptID int64
	CreatedAt   string
}

// NoticeCategory is a notice category: a grantable instance of explicit assignment (RESOURCE_GRANT).
// The row has no owner columns: it is not isolated by user or department; visibility is decided by the role's
// category grants.
type NoticeCategory struct {
	ID        int64
	Name      string
	CreatedAt string
}

// NoticeFilter is the notice list's filter (zero value = no filter).
type NoticeFilter struct {
	// 0 = no category filter (category grants still apply).
	CategoryID int64
	Keyword    string
	// Instant range of the creation time [CreatedFrom, CreatedTo); a zero end is unbounded. The service has already
	// converted business days to instants by the business time zone (ADR admin-0003).
	CreatedFrom time.Time
	CreatedTo   time.Time
}

// NoticeRepo persists notices with data-scope-filtered reads/writes.
//
// spec filters the notices' own rows (owner columns); catSpec is the grant scope of the category resource, applied to
// the category_id column (permdata.ApplyScopeWith(catSpec, Cols{PK: "category_id"})). The same grant filters rows on
// the category table (ApplyScope's default id column) and the foreign key on the notice table: this is the derived
// shape of "child row visibility follows the parent instance's grant".
type NoticeRepo interface {
	List(ctx context.Context, spec, catSpec perm.ScopeSpec, f NoticeFilter, offset, limit int) ([]*Notice, int64, error)
	// Create locks the category row in the same transaction before inserting the notice and returns
	// ErrNoticeCategoryNotFound if the category is gone; paired with DeleteCategory's "delete only when the category
	// has no notices", a concurrent category delete leaves no orphan notices.
	Create(ctx context.Context, n *Notice) error
	Get(ctx context.Context, spec, catSpec perm.ScopeSpec, id int64) (*Notice, error)
	// Delete returns the number of rows actually removed — 0 means the id
	// doesn't exist, or exists but is out of the resolved scope (both cases
	// surface as ErrNoticeNotFound to the caller, indistinguishably).
	Delete(ctx context.Context, spec, catSpec perm.ScopeSpec, id int64) (int64, error)
	// BatchDelete is all or nothing: if any id misses, the whole batch rolls back. It returns the number of rows
	// actually matched, on rollback too; the caller compares it with len(ids).
	BatchDelete(ctx context.Context, spec, catSpec perm.ScopeSpec, ids []int64) (int64, error)
	// HasInDept reports whether dept still owns at least one notice row
	// (owner_dept_id = deptID) — the dept-occupancy check NoticeResourceDefs
	// registers into perm's registry (see NoticeResourceDefs's doc).
	HasInDept(ctx context.Context, deptID int64) (bool, error)

	// ListCategories is the category management list: paged, with category rows filtered by catSpec (ApplyScope's
	// default id column). CategoryOptions is the selector's option set: filtered by catSpec too, but complete, since a
	// dropdown must be able to pick any item and paging would mean "what you cannot page to, you cannot pick".
	// AllCategories differs again: complete and with no scope filter, for the registered callback (a root-only
	// configuration surface). The three serve different audiences; do not merge them.
	ListCategories(ctx context.Context, catSpec perm.ScopeSpec, offset, limit int) ([]*NoticeCategory, int64, error)
	CategoryOptions(ctx context.Context, catSpec perm.ScopeSpec) ([]*NoticeCategory, error)
	AllCategories(ctx context.Context) ([]*NoticeCategory, error)
	CreateCategory(ctx context.Context, c *NoticeCategory) error
	// DeleteCategory deletes only when the category has no notices, with check and delete in one statement; like
	// Create it locks the category row before touching notices, so both sides lock in the same order.
	// A missing category returns ErrNoticeCategoryNotFound; a category that still has notices returns
	// ErrNoticeCategoryInUse.
	DeleteCategory(ctx context.Context, id int64) error
	CategoryNameExists(ctx context.Context, name string) (bool, error)
}

// NoticeUseCase demonstrates both data-scope dimensions end to end: the
// default owner scope columns (row-level scope) and the explicit-grant category
// gate (explicit assignment). scope/revoker are injected as core/perm interfaces
// (implemented by core/sys/biz.ScopeUseCase) — notice depends on the
// core/perm contract only, never on core/sys itself (see cmd/admin/di.go).
type NoticeUseCase struct {
	repo    NoticeRepo
	scope   perm.ScopeResolver
	revoker perm.GrantRevoker
}

// NewNoticeUseCase builds the notice use case.
func NewNoticeUseCase(repo NoticeRepo, scope perm.ScopeResolver, revoker perm.GrantRevoker) *NoticeUseCase {
	return &NoticeUseCase{repo: repo, scope: scope, revoker: revoker}
}

// List resolves actor's visible range over ResNotice (row level) and ResNoticeCategory (category gate), intersected
// (AND) in the data layer. categoryID>0 narrows to that category; category grants still apply, so a category outside
// them yields an empty set without a separate check.
func (uc *NoticeUseCase) List(ctx context.Context, actor *perm.Principal, f NoticeFilter, offset, limit int) ([]*Notice, int64, error) {
	if limit <= 0 { // treat negative as unset too: gorm reads a negative Limit as no limit
		limit = defaultNoticePageSize
	}
	spec, err := uc.scope.Resolve(ctx, actor, ResNotice)
	if err != nil {
		return nil, 0, err
	}
	catSpec, err := uc.scope.Resolve(ctx, actor, ResNoticeCategory)
	if err != nil {
		return nil, 0, err
	}
	return uc.repo.List(ctx, spec, catSpec, f, offset, limit)
}

// Create takes owner_user_id/owner_dept_id from actor — never from
// client-supplied input, which this signature doesn't even accept.
// The category must be inside the actor's category grants (no posting to an invisible category); missing and
// invisible both report ErrNoticeCategoryNotFound, so existence does not leak.
//
// Whether the category exists is not checked here first: that is decided when repo.Create locks the category row in
// its transaction; a check outside the transaction cannot block "category deleted after the check" (see the comment
// on Create in the data layer).
func (uc *NoticeUseCase) Create(ctx context.Context, actor *perm.Principal, title, content string, categoryID int64) (*Notice, error) {
	catSpec, err := uc.scope.Resolve(ctx, actor, ResNoticeCategory)
	if err != nil {
		return nil, err
	}
	if !categoryVisible(catSpec, categoryID) {
		return nil, ErrNoticeCategoryNotFound
	}
	n := &Notice{Title: title, Content: content, CategoryID: categoryID, OwnerUserID: actor.UserID, OwnerDeptID: actor.DeptID}
	if err := uc.repo.Create(ctx, n); err != nil {
		return nil, err
	}
	return n, nil
}

// Delete resolves actor's scope and deletes within it; 0 rows affected
// (nonexistent id, or existing but out of scope) surfaces as
// ErrNoticeNotFound — the two cases are intentionally indistinguishable to
// the caller.
func (uc *NoticeUseCase) Delete(ctx context.Context, actor *perm.Principal, id int64) error {
	spec, err := uc.scope.Resolve(ctx, actor, ResNotice)
	if err != nil {
		return err
	}
	catSpec, err := uc.scope.Resolve(ctx, actor, ResNoticeCategory)
	if err != nil {
		return err
	}
	affected, err := uc.repo.Delete(ctx, spec, catSpec, id)
	if err != nil {
		return err
	}
	if affected == 0 {
		return ErrNoticeNotFound
	}
	return nil
}

// BatchDelete deletes a batch of notices, **all or nothing**: if any id does not exist or is outside the actor's
// scope, nothing is deleted and it reports ErrNoticeNotFound; as with Delete, the two cases are indistinguishable to
// the caller.
//
// This is the server side of the table's multi-select batch action. **Do not** replace it with a frontend loop
// calling Delete: that writes N audit entries (one batch delete should be one event in the audit) and cannot
// guarantee all or nothing.
//
// Deduplication of ids and the count limit are enforced by the proto's buf.validate in the middleware layer and are
// not checked again here.
func (uc *NoticeUseCase) BatchDelete(ctx context.Context, actor *perm.Principal, ids []int64) error {
	spec, err := uc.scope.Resolve(ctx, actor, ResNotice)
	if err != nil {
		return err
	}
	catSpec, err := uc.scope.Resolve(ctx, actor, ResNoticeCategory)
	if err != nil {
		return err
	}
	affected, err := uc.repo.BatchDelete(ctx, spec, catSpec, ids)
	if err != nil {
		return err
	}
	if affected != int64(len(ids)) {
		return ErrNoticeNotFound
	}
	return nil
}

// CategoryOptions returns the category options visible to the actor (for the notice form dropdown and list filters):
// the same category grants, applied here through ApplyScope to the category table's rows, while notice queries apply
// them to the category_id column.
//
// An option set, unpaged: a dropdown must be able to pick any item.
func (uc *NoticeUseCase) CategoryOptions(ctx context.Context, actor *perm.Principal) ([]*NoticeCategory, error) {
	catSpec, err := uc.scope.Resolve(ctx, actor, ResNoticeCategory)
	if err != nil {
		return nil, err
	}
	return uc.repo.CategoryOptions(ctx, catSpec)
}

// ListCategories is the category management list (ops:notice:category): shown paged, so paged on the server.
// Visibility is still the same category grants: the management surface does not bypass the data scope.
func (uc *NoticeUseCase) ListCategories(ctx context.Context, actor *perm.Principal, offset, limit int) ([]*NoticeCategory, int64, error) {
	if limit <= 0 { // treat negative as unset too: gorm reads a negative Limit as no limit
		limit = defaultNoticePageSize
	}
	catSpec, err := uc.scope.Resolve(ctx, actor, ResNoticeCategory)
	if err != nil {
		return nil, 0, err
	}
	return uc.repo.ListCategories(ctx, catSpec, offset, limit)
}

// CreateCategory creates a category (ops:notice:category). Names are unique: categories are explicit-assignment
// options, and two with the same name could not be told apart in the role configuration selector.
func (uc *NoticeUseCase) CreateCategory(ctx context.Context, name string) (*NoticeCategory, error) {
	dup, err := uc.repo.CategoryNameExists(ctx, name)
	if err != nil {
		return nil, err
	}
	if dup {
		return nil, ErrNoticeCategoryExists
	}
	c := &NoticeCategory{Name: name}
	if err := uc.repo.CreateCategory(ctx, c); err != nil {
		return nil, err
	}
	return c, nil
}

// DeleteCategory deletes a category only when it has no notices; that condition is checked in repo's single delete
// statement, so notices are not counted here first. Like every other category operation it stays inside the actor's
// category grants: a category outside them reports ErrNoticeCategoryNotFound, the same as a missing one, and is not
// touched. It then cleans up every role's explicit-assignment rows for the
// category through the kernel's GrantRevoker port: only the kernel reads and writes sys_* tables, so business code
// requests the cleanup instead of deleting directly (dangling grant rows do not widen permissions; the cleanup keeps
// deleted categories out of the role configuration display; there is no transaction across the business and admin
// databases, so cleanup runs after the delete succeeds).
func (uc *NoticeUseCase) DeleteCategory(ctx context.Context, actor *perm.Principal, id int64) error {
	catSpec, err := uc.scope.Resolve(ctx, actor, ResNoticeCategory)
	if err != nil {
		return err
	}
	if !categoryVisible(catSpec, id) {
		return ErrNoticeCategoryNotFound
	}
	if err := uc.repo.DeleteCategory(ctx, id); err != nil {
		return err
	}
	return uc.revoker.RevokeGrants(ctx, ResNoticeCategory, id)
}

// categoryVisible reports whether categoryID is inside catSpec. The category resource has Anchors=0, so spec takes
// only three forms: All / ResourceIDs / empty (deny-all).
func categoryVisible(catSpec perm.ScopeSpec, categoryID int64) bool {
	return catSpec.All || slices.Contains(catSpec.ResourceIDs, categoryID)
}
