package data

import (
	"time"

	"gorm.io/gorm"

	"github.com/go-keel/keel-admin/internal/core/sys/biz"
	"github.com/go-keel/keel-admin/internal/core/timefmt"
)

// This file holds the user domain's POs (GORM structs private to the data layer): each is named after its table in
// lowerCamelCase, with its TableName and toBiz right after the definition. The structs match the DDL one to one; the
// DDL is in migrations/admin/0_schema.sql (AutoMigrate is not used).

type sysUser struct {
	ID           int64 `gorm:"primaryKey;autoIncrement"`
	Username     string
	Password     string
	Nickname     string     `gorm:"default:''"`
	Email        string     `gorm:"default:''"`
	Phone        string     `gorm:"default:''"`
	DeptID       int64      `gorm:"column:dept_id"`
	IsRoot       int8       `gorm:"column:is_root;default:0"`
	Status       int8       `gorm:"default:1"`
	OTPSecretEnc []byte     `gorm:"column:otp_secret_enc"`
	OTPBound     int8       `gorm:"column:otp_bound;default:0"`
	OTPBoundAt   *time.Time `gorm:"column:otp_bound_at"`
	TokenVersion int        `gorm:"column:token_version;default:0"`
	Avatar       string
	IPWhitelist  string `gorm:"column:ip_whitelist;default:''"`
	// The user settings as JSON text; empty means never set. The shape is settingsJSON.
	Settings  string
	CreatedAt time.Time      `gorm:"autoCreateTime"`
	UpdatedAt time.Time      `gorm:"autoUpdateTime"`
	DeletedAt gorm.DeletedAt `gorm:"column:deleted_at"`
}

func (sysUser) TableName() string { return "sys_user" }

// toBiz converts the PO to a DO (read direction).
func (m *sysUser) toBiz() *biz.User {
	return &biz.User{
		ID:           m.ID,
		Username:     m.Username,
		Password:     m.Password,
		Nickname:     m.Nickname,
		Email:        m.Email,
		Phone:        m.Phone,
		DeptID:       m.DeptID,
		IsRoot:       m.IsRoot == 1,
		Avatar:       m.Avatar,
		IPWhitelist:  m.IPWhitelist,
		Settings:     decodeSettings(m.Settings),
		OTPBound:     m.OTPBound == 1,
		OTPBoundAt:   otpBoundAt(m.OTPBoundAt),
		Status:       m.Status,
		TokenVersion: m.TokenVersion,
		CreatedAt:    timefmt.RFC3339(m.CreatedAt),
	}
}

// otpBoundAt formats the nullable binding time as RFC3339, and gives an empty string when not bound (NULL).
func otpBoundAt(t *time.Time) string {
	if t == nil {
		return ""
	}
	return timefmt.RFC3339(*t)
}

type sysUserRole struct {
	UserID int64 `gorm:"column:user_id;primaryKey"`
	RoleID int64 `gorm:"column:role_id;primaryKey"`
}

func (sysUserRole) TableName() string { return "sys_user_role" }
