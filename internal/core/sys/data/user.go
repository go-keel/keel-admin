package data

import (
	"context"
	"errors"

	"gorm.io/gorm"

	coredata "github.com/go-keel/keel-admin/internal/core/data"
	"github.com/go-keel/keel-admin/internal/core/sys/biz"
)

var (
	_ biz.UserRepo      = (*userRepo)(nil)
	_ biz.UserAdminRepo = (*userRepo)(nil)
)

type userRepo struct {
	data *coredata.Data
}

// NewUserAdminRepo builds the account repository (superset interface).
func NewUserAdminRepo(data *coredata.Data) biz.UserAdminRepo {
	return &userRepo{data: data}
}

// NewUserRepo exposes the same instance as the base UserRepo for auth.
func NewUserRepo(r biz.UserAdminRepo) biz.UserRepo {
	return r
}

func (r *userRepo) FindByUsername(ctx context.Context, username string) (*biz.User, error) {
	var m sysUser
	err := r.data.DB.WithContext(ctx).Where("username = ?", username).First(&m).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, biz.ErrInvalidCredential
	}
	if err != nil {
		return nil, err
	}
	return m.toBiz(), nil
}

func (r *userRepo) FindByID(ctx context.Context, id int64) (*biz.User, error) {
	var m sysUser
	err := r.data.DB.WithContext(ctx).Where("id = ?", id).First(&m).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, biz.ErrInvalidCredential
	}
	if err != nil {
		return nil, err
	}
	return m.toBiz(), nil
}

func (r *userRepo) RolePermCodesByRole(ctx context.Context, roleID int64) ([]string, error) {
	var codes []string
	err := r.data.DB.WithContext(ctx).
		Model(&sysRolePermission{}).
		Joins(joinActiveRole("sys_role", "sys_role_permission.role_id")).
		Where("sys_role_permission.role_id = ?", roleID).
		Pluck("sys_role_permission.perm_code", &codes).Error
	if err != nil {
		return nil, err
	}
	return codes, nil
}
