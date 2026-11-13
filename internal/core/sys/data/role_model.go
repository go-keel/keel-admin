package data

import (
	"time"

	"gorm.io/gorm"

	"github.com/go-keel/keel-admin/internal/core/sys/biz"
	"github.com/go-keel/keel-admin/internal/core/timefmt"
)

// This file holds the role domain's POs (GORM structs private to the data layer): each is named after its table in
// lowerCamelCase, with its TableName and toBiz right after the definition. The structs match the DDL one to one; the
// DDL is in migrations/admin/0_schema.sql (AutoMigrate is not used).

type sysRole struct {
	ID        int64 `gorm:"primaryKey;autoIncrement"`
	Code      string
	Name      string
	Sensitive int8           `gorm:"default:0"`
	ForceOTP  int8           `gorm:"column:force_otp;default:0"`
	Status    int8           // No default: GORM would turn 0 into default 1, so no disabled role.
	CreatedAt time.Time      `gorm:"autoCreateTime"`
	UpdatedAt time.Time      `gorm:"autoUpdateTime"`
	DeletedAt gorm.DeletedAt `gorm:"column:deleted_at"`
}

func (sysRole) TableName() string { return "sys_role" }

// toBiz converts the PO to a DO (read direction).
func (m *sysRole) toBiz() *biz.Role {
	return &biz.Role{
		ID:        m.ID,
		Code:      m.Code,
		Name:      m.Name,
		Sensitive: m.Sensitive,
		ForceOTP:  m.ForceOTP,
		Status:    m.Status,
	}
}

// toListRow converts to the slim list row (the caller fills PermCount in a batch).
func (m *sysRole) toListRow() *biz.RoleListRow {
	return &biz.RoleListRow{
		ID:        m.ID,
		Code:      m.Code,
		Name:      m.Name,
		Sensitive: m.Sensitive,
		ForceOTP:  m.ForceOTP,
		Status:    m.Status,
		CreatedAt: timefmt.RFC3339(m.CreatedAt),
	}
}

type sysRolePermission struct {
	RoleID   int64  `gorm:"column:role_id;primaryKey"`
	PermCode string `gorm:"column:perm_code;primaryKey"`
}

func (sysRolePermission) TableName() string { return "sys_role_permission" }

type sysRoleDataScope struct {
	ID           int64     `gorm:"primaryKey;autoIncrement"`
	RoleID       int64     `gorm:"column:role_id"`
	ResourceType string    `gorm:"column:resource_type"`
	ScopeType    string    `gorm:"column:scope_type"`
	CreatedAt    time.Time `gorm:"autoCreateTime"`
}

func (sysRoleDataScope) TableName() string { return "sys_role_data_scope" }

type sysRoleAssignable struct {
	RoleID           int64 `gorm:"column:role_id;primaryKey"`
	AssignableRoleID int64 `gorm:"column:assignable_role_id;primaryKey"`
}

func (sysRoleAssignable) TableName() string { return "sys_role_assignable" }

type sysRoleManageDept struct {
	RoleID int64 `gorm:"column:role_id;primaryKey"`
	DeptID int64 `gorm:"column:dept_id;primaryKey"`
}

func (sysRoleManageDept) TableName() string { return "sys_role_manage_dept" }

// sysRoleResourceGrant is an explicit-assignment instance row: role × resource type × instance ID. RoleRepo.Save
// replaces the rows as a whole in the same transaction as the scope policy, and ScopeRepo reads them by the actor's
// role set; the kernel does not interpret what an instance means (a column, a brand group, a store).
type sysRoleResourceGrant struct {
	RoleID       int64     `gorm:"column:role_id;primaryKey;autoIncrement:false"`
	ResourceType string    `gorm:"column:resource_type;primaryKey"`
	ResourceID   int64     `gorm:"column:resource_id;primaryKey;autoIncrement:false"`
	CreatedAt    time.Time `gorm:"autoCreateTime"`
}

func (sysRoleResourceGrant) TableName() string { return "sys_role_resource_grant" }
