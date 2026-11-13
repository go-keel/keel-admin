package data

import (
	"cmp"
	"context"
	"errors"
	"slices"

	"gorm.io/gorm"
	"gorm.io/gorm/clause"

	coredata "github.com/go-keel/keel-admin/internal/core/data"
	"github.com/go-keel/keel-admin/internal/core/sys/biz"
)

var _ biz.RoleRepo = (*roleRepo)(nil)

type roleRepo struct {
	data *coredata.Data
}

// NewRoleRepo builds the role repository.
func NewRoleRepo(data *coredata.Data) biz.RoleRepo { return &roleRepo{data: data} }

func (r *roleRepo) List(ctx context.Context, f biz.RoleFilter, offset, limit int) ([]*biz.RoleListRow, int64, error) {
	q := r.data.DB.WithContext(ctx).Model(&sysRole{})
	if f.Keyword != "" {
		like := containsPattern(f.Keyword)
		q = q.Where("code LIKE ? OR name LIKE ?", like, like)
	}
	q = whereTriState(q, "status", f.Status)
	q = whereTriState(q, "sensitive", f.Sensitive)
	q = whereTriState(q, "force_otp", f.ForceOTP)

	var total int64
	if err := q.Count(&total).Error; err != nil {
		return nil, 0, err
	}
	var ms []sysRole
	if err := q.Order("id asc").Offset(offset).Limit(limit).Find(&ms).Error; err != nil {
		return nil, 0, err
	}
	ids := make([]int64, 0, len(ms))
	for i := range ms {
		ids = append(ids, ms[i].ID)
	}
	counts, err := r.permCounts(ctx, ids)
	if err != nil {
		return nil, 0, err
	}
	out := make([]*biz.RoleListRow, 0, len(ms))
	for i := range ms {
		row := ms[i].toListRow()
		row.PermCount = counts[row.ID]
		out = append(out, row)
	}
	return out, total, nil
}

// permCounts gets the code count of every role on the page in one GROUP BY. The list shows only this number; querying
// the code set row by row is a wasted N+1 and also sends out what only the detail view should get.
func (r *roleRepo) permCounts(ctx context.Context, roleIDs []int64) (map[int64]int32, error) {
	out := map[int64]int32{}
	if len(roleIDs) == 0 {
		return out, nil
	}
	var rows []struct {
		RoleID int64
		N      int32
	}
	if err := r.data.DB.WithContext(ctx).Model(&sysRolePermission{}).
		Select("role_id, COUNT(*) AS n").
		Where("role_id IN ?", roleIDs).Group("role_id").Scan(&rows).Error; err != nil {
		return nil, err
	}
	for _, row := range rows {
		out[row.RoleID] = row.N
	}
	return out, nil
}

// whereTriState turns the filter value 0 = all / 1 = yes / 2 = no into `col = 1` / `col = 0`. The role list and the
// user list have exactly the same tri-state filter shape; a switch in each would only copy the same 1↔2 offset around.
//
// The column name goes through clause.Column so GORM quotes it per dialect instead of splicing it bare into the SQL
// string: sensitive is a reserved word in MySQL 8, and written bare it is a 1064 syntax error. The rest of the data
// layer's tests run on sqlite, which does not reserve it and cannot catch this; mysql_quote_test.go guards it.
func whereTriState(q *gorm.DB, col string, v int32) *gorm.DB {
	switch v {
	case 1:
		return q.Where("? = 1", clause.Column{Name: col})
	case 2:
		return q.Where("? = 0", clause.Column{Name: col})
	}
	return q
}

// ListBrief only Finds role rows, without the permCodes N+1: selectors need only id, code and name.
func (r *roleRepo) ListBrief(ctx context.Context) ([]*biz.Role, error) {
	var ms []sysRole
	if err := r.data.DB.WithContext(ctx).Order("id asc").Find(&ms).Error; err != nil {
		return nil, err
	}
	out := make([]*biz.Role, 0, len(ms))
	for i := range ms {
		out = append(out, ms[i].toBiz())
	}
	return out, nil
}

func (r *roleRepo) Get(ctx context.Context, id int64) (*biz.Role, error) {
	var m sysRole
	err := r.data.DB.WithContext(ctx).Where("id = ?", id).First(&m).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, biz.ErrRoleNotFound
	}
	if err != nil {
		return nil, err
	}
	role := m.toBiz()
	codes, err := r.permCodes(ctx, id)
	if err != nil {
		return nil, err
	}
	role.Perms = codes
	return role, nil
}

func (r *roleRepo) GetByCode(ctx context.Context, code string) (*biz.Role, error) {
	var m sysRole
	// code is the stable identity used by audits and integrations. It is kept forever after a soft delete, and new
	// roles may not reuse it.
	err := r.data.DB.WithContext(ctx).Unscoped().Where("code = ?", code).First(&m).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, biz.ErrRoleNotFound
	}
	if err != nil {
		return nil, err
	}
	return m.toBiz(), nil
}

// Save writes the role's main row and its five configuration tables in one transaction, rolling back as a whole on
// failure.
//
// The write order is fixed as sys_role → sys_role_permission → sys_role_assignable → sys_role_manage_dept →
// sys_role_data_scope → sys_role_resource_grant, the kernel's
// role lock chain: locks are taken only forward along this order, never backwards. When changing an existing role, the
// first step locks its sys_role row by primary key with FOR UPDATE: concurrent saves of the same role queue into two in
// turn, the later replacing everything after the earlier commits, so the database keeps the complete configuration of
// one commit. When creating, the main row was just inserted by this transaction and the insert already holds its row
// lock, so no further lock is needed. Each configuration table is replaced by deleting and changing specific rows by
// primary key, without gap locks, so concurrent saves of different roles do not block each other (see replaceKeys).
//
// There is no deadlock retry: the same role queues on this row lock, different roles never touch the same row, and
// writing in the order of the table should never deadlock. A real deadlock (MySQL 1213) means some write path broke the
// order, and it must surface as an error so that path is fixed, not be hidden by a retry.
func (r *roleRepo) Save(ctx context.Context, role *biz.Role) error {
	var id int64
	err := r.data.DB.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if role.ID == 0 {
			m := sysRole{Code: role.Code, Name: role.Name, Sensitive: role.Sensitive, ForceOTP: role.ForceOTP, Status: role.Status}
			if err := tx.Create(&m).Error; err != nil {
				// The only key this sys_role insert can collide on is uk_code (the primary key auto-increments; it is
				// a single-column unique key without deleted_at, so a soft-deleted role's code collides too, matching
				// GetByCode going Unscoped), so a collision maps straight to "role code already exists"; under MySQL
				// the translated error does not say which key collided, so this premise is all there is. Only the
				// main-row insert is mapped: biz validates and deduplicates the inputs of the five configuration
				// tables, so a collision there can only mean the validation was bypassed, and it is returned as is.
				if errors.Is(err, gorm.ErrDuplicatedKey) {
					return biz.ErrDuplicateRole
				}
				return err
			}
			id = m.ID
		} else {
			var m sysRole
			err := tx.Clauses(clause.Locking{Strength: clause.LockingStrengthUpdate}).
				Select("id").Where("id = ?", role.ID).First(&m).Error
			if errors.Is(err, gorm.ErrRecordNotFound) {
				return biz.ErrRoleNotFound
			}
			if err != nil {
				return err
			}
			id = m.ID
			if err := tx.Model(&sysRole{}).Where("id = ?", id).Updates(map[string]any{
				"name":      role.Name,
				"force_otp": role.ForceOTP,
				"status":    role.Status,
				"sensitive": role.Sensitive,
			}).Error; err != nil {
				return err
			}
		}
		if err := replacePermissions(tx, id, role.Perms); err != nil {
			return err
		}
		if err := replaceAssignable(tx, id, role.AssignableRoleIDs); err != nil {
			return err
		}
		if err := replaceManageDept(tx, id, role.ManageDepts); err != nil {
			return err
		}
		return replaceDataScope(tx, id, role.Scopes)
	})
	if err != nil {
		return err
	}
	role.ID = id
	return nil
}

func (r *roleRepo) Delete(ctx context.Context, id int64) error {
	// A role is auditable permission-configuration master data: deleting only marks the main record, and its members,
	// permissions, data scope and ARBAC configuration are all kept. Every runtime read filters with the active
	// sys_role as the anchor, so the mark takes effect at once; keeping the related rows lets one trace everything the
	// deleted role ever granted.
	return r.data.DB.WithContext(ctx).Where("id = ?", id).Delete(&sysRole{}).Error
}

// replacePermissions replaces the role's permission codes as a whole and is called only inside Save's transaction.
func replacePermissions(tx *gorm.DB, roleID int64, codes []string) error {
	return replaceKeys(tx, "role_id", roleID, "perm_code", codes, func(c string) sysRolePermission {
		return sysRolePermission{RoleID: roleID, PermCode: c}
	})
}

// replaceKeys replaces the set in column keyCol under parent row parentID with want: it reads the existing set first,
// deletes only what was removed, inserts only what was added, and leaves unchanged rows alone. The caller must already
// hold the lock on the parent row (or the parent row was inserted by this transaction), so that this read sees the
// value at the moment of writing.
//
// It is not written as "WHERE parent_id = ?, delete everything and insert again": that is a range delete, which under
// MySQL repeatable read also locks the gaps at both ends of that index range, and one end borders the first row of the
// neighbouring parent (another role or user with the adjacent ID). Two transactions each deleting their own rows and
// then inserting would wait on each other's gap locks and deadlock, and different parents have no parent lock to queue
// on; editing two users with adjacent IDs reproduced it. Deleting specific, existing rows by primary key locks only
// those rows, inserting new rows contends with nobody, and replacements under different parents do not block each
// other. Deleted keys are sorted in ascending order.
//
// want is not deduplicated: giving the same key twice still collides on the primary key and rolls back as a whole, as
// before.
func replaceKeys[K cmp.Ordered, M any](tx *gorm.DB, parentCol string, parentID int64, keyCol string, want []K, newRow func(K) M) error {
	var have []K
	if err := tx.Model(new(M)).Where(parentCol+" = ?", parentID).Pluck(keyCol, &have).Error; err != nil {
		return err
	}
	add, del := diffKeys(have, want, cmp.Compare[K])
	if len(del) > 0 {
		if err := tx.Where(parentCol+" = ? AND "+keyCol+" IN ?", parentID, del).Delete(new(M)).Error; err != nil {
			return err
		}
	}
	if len(add) == 0 {
		return nil
	}
	rows := make([]M, 0, len(add))
	for _, k := range add {
		rows = append(rows, newRow(k))
	}
	return tx.Create(&rows).Error
}

// diffKeys computes the keys to add (add) and to delete (del) to go from have to want, each sorted ascending by
// compare. add keeps the duplicates in want; see replaceKeys.
func diffKeys[K comparable](have, want []K, compare func(a, b K) int) (add, del []K) {
	haveSet := make(map[K]bool, len(have))
	for _, k := range have {
		haveSet[k] = true
	}
	wantSet := make(map[K]bool, len(want))
	for _, k := range want {
		wantSet[k] = true
		if !haveSet[k] {
			add = append(add, k)
		}
	}
	for _, k := range have {
		if !wantSet[k] {
			del = append(del, k)
		}
	}
	slices.SortFunc(add, compare)
	slices.SortFunc(del, compare)
	return add, del
}

// DataScopes reads the scope configuration back in a batch. sys_role_data_scope is unique on (role_id, resource_type),
// so each role has at most one row per resource; the instance ID sets of RESOURCE_GRANT come back from
// sys_role_resource_grant at the same time (the configuration UI shows them: with whole-replacement semantics, every
// submit would otherwise clear the explicit assignments).
func (r *roleRepo) DataScopes(ctx context.Context, roleIDs []int64) (map[int64][]biz.DataScopeInput, error) {
	out := map[int64][]biz.DataScopeInput{}
	if len(roleIDs) == 0 {
		return out, nil
	}
	var rows []sysRoleDataScope
	if err := r.data.DB.WithContext(ctx).Where("role_id IN ?", roleIDs).
		Order("resource_type asc").Find(&rows).Error; err != nil {
		return nil, err
	}
	var grants []sysRoleResourceGrant
	if err := r.data.DB.WithContext(ctx).Where("role_id IN ?", roleIDs).
		Order("resource_id asc").Find(&grants).Error; err != nil {
		return nil, err
	}
	type grantKey struct {
		roleID       int64
		resourceType string
	}
	grantIDs := map[grantKey][]int64{}
	for _, g := range grants {
		k := grantKey{g.RoleID, g.ResourceType}
		grantIDs[k] = append(grantIDs[k], g.ResourceID)
	}
	for _, row := range rows {
		out[row.RoleID] = append(out[row.RoleID], biz.DataScopeInput{
			ResourceType: row.ResourceType,
			ScopeType:    biz.ScopeType(row.ScopeType),
			ResourceIDs:  grantIDs[grantKey{row.RoleID, row.ResourceType}],
		})
	}
	return out, nil
}

// replaceDataScope replaces the role's data scope as a whole and is called only inside Save's transaction. The policy
// rows and the explicit-assignment instance rows are two halves of one configuration: the policy table comes first and
// the instance table second, matching Save's write order; both tables change only the rows that changed, as in
// replaceKeys.
//
// The policy table's primary key is an auto-increment id that does not start with role_id: deleting by role_id would
// go through the unique index uk_role_res, and when the table is small the optimiser switches to a full table scan
// that locks other roles' rows too (two roles saved concurrently deadlocked because of this). So it reads the role's
// existing rows first and deletes and changes them by id; a changed scope_type is updated in place.
func replaceDataScope(tx *gorm.DB, roleID int64, items []biz.DataScopeInput) error {
	var have []sysRoleDataScope
	if err := tx.Where("role_id = ?", roleID).Find(&have).Error; err != nil {
		return err
	}
	existing := make(map[string]sysRoleDataScope, len(have))
	for _, row := range have {
		existing[row.ResourceType] = row
	}
	var inserts []sysRoleDataScope
	var wantGrants []grantRef
	for _, it := range items {
		if row, ok := existing[it.ResourceType]; ok {
			delete(existing, it.ResourceType)
			if row.ScopeType != string(it.ScopeType) {
				if err := tx.Model(&sysRoleDataScope{}).Where("id = ?", row.ID).
					Update("scope_type", string(it.ScopeType)).Error; err != nil {
					return err
				}
			}
		} else {
			inserts = append(inserts, sysRoleDataScope{RoleID: roleID, ResourceType: it.ResourceType, ScopeType: string(it.ScopeType)})
		}
		for _, rid := range it.ResourceIDs {
			wantGrants = append(wantGrants, grantRef{it.ResourceType, rid})
		}
	}
	if len(existing) > 0 {
		ids := make([]int64, 0, len(existing))
		for _, row := range existing {
			ids = append(ids, row.ID)
		}
		slices.Sort(ids)
		if err := tx.Where("id IN ?", ids).Delete(&sysRoleDataScope{}).Error; err != nil {
			return err
		}
	}
	if len(inserts) > 0 {
		if err := tx.Create(&inserts).Error; err != nil {
			return err
		}
	}
	return replaceGrants(tx, roleID, wantGrants)
}

// grantRef is the key of one explicit-assignment instance row under a role.
type grantRef struct {
	ResourceType string
	ResourceID   int64
}

func compareGrantRef(a, b grantRef) int {
	return cmp.Or(cmp.Compare(a.ResourceType, b.ResourceType), cmp.Compare(a.ResourceID, b.ResourceID))
}

// replaceGrants replaces the role's explicit-assignment instance rows as a whole, the same way as replaceKeys; the key
// has two columns, so deletes are grouped by resource type, one "primary-key prefix + IN" delete per group.
func replaceGrants(tx *gorm.DB, roleID int64, want []grantRef) error {
	var rows []sysRoleResourceGrant
	if err := tx.Select("role_id", "resource_type", "resource_id").Where("role_id = ?", roleID).Find(&rows).Error; err != nil {
		return err
	}
	have := make([]grantRef, 0, len(rows))
	for _, row := range rows {
		have = append(have, grantRef{row.ResourceType, row.ResourceID})
	}
	add, del := diffKeys(have, want, compareGrantRef)
	for i := 0; i < len(del); {
		rt, ids := del[i].ResourceType, []int64{}
		for ; i < len(del) && del[i].ResourceType == rt; i++ {
			ids = append(ids, del[i].ResourceID)
		}
		if err := tx.Where("role_id = ? AND resource_type = ? AND resource_id IN ?", roleID, rt, ids).
			Delete(&sysRoleResourceGrant{}).Error; err != nil {
			return err
		}
	}
	if len(add) == 0 {
		return nil
	}
	inserts := make([]sysRoleResourceGrant, 0, len(add))
	for _, k := range add {
		inserts = append(inserts, sysRoleResourceGrant{RoleID: roleID, ResourceType: k.ResourceType, ResourceID: k.ResourceID})
	}
	return tx.Create(&inserts).Error
}

func (r *roleRepo) permCodes(ctx context.Context, roleID int64) ([]string, error) {
	var codes []string
	err := r.data.DB.WithContext(ctx).Model(&sysRolePermission{}).
		Where("role_id = ?", roleID).Pluck("perm_code", &codes).Error
	return codes, err
}
