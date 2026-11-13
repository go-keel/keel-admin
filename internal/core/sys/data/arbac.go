package data

import (
	"context"
	"strconv"

	"gorm.io/gorm"
	"gorm.io/gorm/clause"

	coredata "github.com/go-keel/keel-admin/internal/core/data"
	"github.com/go-keel/keel-admin/internal/core/sys/biz"
)

var _ biz.ARBACRepo = (*arbacRepo)(nil)

type arbacRepo struct {
	data *coredata.Data
}

// NewARBACRepo builds the ARBAC boundary repository.
func NewARBACRepo(data *coredata.Data) biz.ARBACRepo { return &arbacRepo{data: data} }

func (r *arbacRepo) AssignableRoleIDs(ctx context.Context, roleIDs []int64) ([]int64, error) {
	if len(roleIDs) == 0 {
		return nil, nil
	}
	var ids []int64
	err := r.data.DB.WithContext(ctx).Model(&sysRoleAssignable{}).
		Joins(joinActiveRole("source_role", "sys_role_assignable.role_id")).
		Joins(joinActiveRole("target_role", "sys_role_assignable.assignable_role_id")).
		Where("sys_role_assignable.role_id IN ?", roleIDs).
		Distinct().Pluck("sys_role_assignable.assignable_role_id", &ids).Error
	return ids, err
}

func (r *arbacRepo) ManageDeptIDs(ctx context.Context, roleIDs []int64) ([]int64, error) {
	if len(roleIDs) == 0 {
		return nil, nil
	}
	var rows []sysRoleManageDept
	if err := r.data.DB.WithContext(ctx).Model(&sysRoleManageDept{}).
		Joins(joinActiveRole("sys_role", "sys_role_manage_dept.role_id")).
		Joins("JOIN sys_dept ON sys_dept.id = sys_role_manage_dept.dept_id AND sys_dept.deleted_at IS NULL").
		Where("sys_role_manage_dept.role_id IN ?", roleIDs).Find(&rows).Error; err != nil {
		return nil, err
	}
	set := map[int64]struct{}{}
	for _, row := range rows {
		// Each configured department always expands to its whole subtree: managing a department means managing its line
		// (there is no "this level only" option).
		set[row.DeptID] = struct{}{}
		var desc []int64
		like := "%," + strconv.FormatInt(row.DeptID, 10) + ",%"
		if err := r.data.DB.WithContext(ctx).Model(&sysDept{}).
			Where("ancestors LIKE ?", like).Pluck("id", &desc).Error; err != nil {
			return nil, err
		}
		for _, d := range desc {
			set[d] = struct{}{}
		}
	}
	out := make([]int64, 0, len(set))
	for d := range set {
		out = append(out, d)
	}
	return out, nil
}

// AssignableConfigs reads the configuration back as stored, without taking the union: the union is for decisions,
// while the form shows it grouped by role. Assigned roles are joined as held roles, so disabled ones are shown too:
// root submits the whole form as shown when editing a role, and filtering out disabled roles would clear them on save.
func (r *arbacRepo) AssignableConfigs(ctx context.Context, roleIDs []int64) (map[int64][]int64, error) {
	out := map[int64][]int64{}
	if len(roleIDs) == 0 {
		return out, nil
	}
	var rows []sysRoleAssignable
	if err := r.data.DB.WithContext(ctx).Model(&sysRoleAssignable{}).
		Joins(joinHeldRole("target_role", "sys_role_assignable.assignable_role_id")).
		Where("sys_role_assignable.role_id IN ?", roleIDs).Find(&rows).Error; err != nil {
		return nil, err
	}
	for _, row := range rows {
		out[row.RoleID] = append(out[row.RoleID], row.AssignableRoleID)
	}
	return out, nil
}

// ManageDeptConfigs reads back the configured department IDs as stored, without expanding subtrees: expansion is what
// ManageDeptIDs does for decisions.
func (r *arbacRepo) ManageDeptConfigs(ctx context.Context, roleIDs []int64) (map[int64][]int64, error) {
	out := map[int64][]int64{}
	if len(roleIDs) == 0 {
		return out, nil
	}
	var rows []sysRoleManageDept
	if err := r.data.DB.WithContext(ctx).Model(&sysRoleManageDept{}).
		Joins("JOIN sys_dept ON sys_dept.id = sys_role_manage_dept.dept_id AND sys_dept.deleted_at IS NULL").
		Where("sys_role_manage_dept.role_id IN ?", roleIDs).Find(&rows).Error; err != nil {
		return nil, err
	}
	for _, row := range rows {
		out[row.RoleID] = append(out[row.RoleID], row.DeptID)
	}
	return out, nil
}

func (r *arbacRepo) SensitiveRoleIDs(ctx context.Context, roleIDs []int64) ([]int64, error) {
	if len(roleIDs) == 0 {
		return nil, nil
	}
	var ids []int64
	// sensitive is a reserved word in MySQL 8: the column name goes through clause.Column so it is quoted per
	// dialect; written bare it is a 1064 syntax error (same as whereTriState in role.go).
	err := r.data.DB.WithContext(ctx).Model(&sysRole{}).
		Where("id IN ? AND ? = 1", roleIDs, clause.Column{Name: "sensitive"}).Pluck("id", &ids).Error
	return ids, err
}

// replaceAssignable replaces a role's assignable roles as a whole and is called only inside the RoleRepo.Save
// transaction (the method is described at replaceKeys). It lives in this file because the table belongs to the ARBAC
// dimension, but it must be written in the same transaction as the rest of the role's configuration, so it does not
// hang off arbacRepo.
func replaceAssignable(tx *gorm.DB, roleID int64, assignableRoleIDs []int64) error {
	return replaceKeys(tx, "role_id", roleID, "assignable_role_id", assignableRoleIDs, func(a int64) sysRoleAssignable {
		return sysRoleAssignable{RoleID: roleID, AssignableRoleID: a}
	})
}

// replaceManageDept replaces a role's manageable departments as a whole and is called only inside the RoleRepo.Save
// transaction (the method is described at replaceKeys).
func replaceManageDept(tx *gorm.DB, roleID int64, deptIDs []int64) error {
	return replaceKeys(tx, "role_id", roleID, "dept_id", deptIDs, func(d int64) sysRoleManageDept {
		return sysRoleManageDept{RoleID: roleID, DeptID: d}
	})
}
