package data

import (
	"context"
	"strconv"

	"gorm.io/gorm"
	"gorm.io/gorm/clause"

	coredata "github.com/go-keel/keel-admin/internal/core/data"
	"github.com/go-keel/keel-admin/internal/core/sys/biz"
)

var _ biz.ScopeRepo = (*scopeRepo)(nil)

type scopeRepo struct {
	data *coredata.Data
}

// NewScopeRepo builds the data-scope repository.
func NewScopeRepo(data *coredata.Data) biz.ScopeRepo { return &scopeRepo{data: data} }

func (r *scopeRepo) RoleScopes(ctx context.Context, roleIDs []int64, resourceType string) ([]biz.RoleScope, error) {
	if len(roleIDs) == 0 {
		return nil, nil
	}
	var rows []sysRoleDataScope
	err := r.data.DB.WithContext(ctx).
		Model(&sysRoleDataScope{}).
		Joins(joinActiveRole("r", "sys_role_data_scope.role_id")).
		Where("sys_role_data_scope.role_id IN ? AND sys_role_data_scope.resource_type = ?", roleIDs, resourceType).
		Find(&rows).Error
	if err != nil {
		return nil, err
	}
	out := make([]biz.RoleScope, 0, len(rows))
	for _, s := range rows {
		out = append(out, biz.RoleScope{ScopeType: biz.ScopeType(s.ScopeType)})
	}
	return out, nil
}

func (r *scopeRepo) SubtreeDeptIDs(ctx context.Context, deptID int64) ([]int64, error) {
	ids := []int64{deptID}
	var descendants []int64
	like := "%," + strconv.FormatInt(deptID, 10) + ",%"
	err := r.data.DB.WithContext(ctx).Model(&sysDept{}).
		Where("ancestors LIKE ?", like).Pluck("id", &descendants).Error
	if err != nil {
		return nil, err
	}
	return append(ids, descendants...), nil
}

func (r *scopeRepo) RoleGrantedResourceIDs(ctx context.Context, roleIDs []int64, resourceType string) ([]int64, error) {
	if len(roleIDs) == 0 {
		return nil, nil
	}
	var ids []int64
	err := r.data.DB.WithContext(ctx).Model(&sysRoleResourceGrant{}).
		Joins(joinActiveRole("sys_role", "sys_role_resource_grant.role_id")).
		Where("sys_role_resource_grant.role_id IN ? AND sys_role_resource_grant.resource_type = ?", roleIDs, resourceType).
		Distinct().Pluck("sys_role_resource_grant.resource_id", &ids).Error
	return ids, err
}

// DeleteGrantsByResource deletes every role's explicit-assignment rows for one resource instance (called through
// perm.GrantRevoker after the business area deletes the instance).
//
// It deletes sys_role_resource_grant rows across roles and locks along the same role chain as role saving
// (RoleRepo.Save): it first reads the roles holding this grant with a plain read, then locks their sys_role rows one by
// one in ascending ID order, and finally deletes by primary key one by one. This way "lock the role's sys_role row
// before writing its configuration tables" has no exception in the kernel, and the lock-order table needs one rule.
// Once the role rows are locked, saves of those roles queue behind it and the delete waits for nobody.
//
// The earlier form was one `DELETE … WHERE resource_type = ? AND resource_id = ?`: it goes through the
// (resource_type, resource_id) secondary index, locking the index entries before the primary-key rows, while role
// saving locks the primary-key rows before changing the index entries, the opposite order. Measurements (two deletes
// interleaved with saves of five roles, thousands of times each) produced no deadlock, because the reversed step falls
// within one statement on both sides and the window is very narrow; locking the role rows first removes the exception,
// it does not fix a reproduced deadlock.
//
// Role rows are locked one by one by primary key: `id IN (…) FOR UPDATE` is optimised into a scan of the whole index
// when the role table is small, locking every role. Deleted roles are locked too (their grant rows must go as well), so
// there is no soft-delete condition.
//
// A role that newly gets this grant after the roles are read (a concurrent save that happens to pick this instance) is
// not part of this delete and leaves a grant row pointing at an instance that no longer exists. It widens no
// permission, and is the same trade-off as the cross-database order of deleting the instance in the business database
// first and cleaning up grants in the admin database afterwards (docs/admin/domain-model.md "No transactions across
// databases").
func (r *scopeRepo) DeleteGrantsByResource(ctx context.Context, resourceType string, resourceID int64) error {
	return r.data.DB.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		var roleIDs []int64
		if err := tx.Model(&sysRoleResourceGrant{}).
			Where("resource_type = ? AND resource_id = ?", resourceType, resourceID).
			Order("role_id").Pluck("role_id", &roleIDs).Error; err != nil {
			return err
		}
		for _, roleID := range roleIDs {
			var locked []int64
			if err := tx.Unscoped().Model(&sysRole{}).Clauses(clause.Locking{Strength: clause.LockingStrengthUpdate}).
				Where("id = ?", roleID).Pluck("id", &locked).Error; err != nil {
				return err
			}
		}
		for _, roleID := range roleIDs {
			if err := tx.Where("role_id = ? AND resource_type = ? AND resource_id = ?", roleID, resourceType, resourceID).
				Delete(&sysRoleResourceGrant{}).Error; err != nil {
				return err
			}
		}
		return nil
	})
}
