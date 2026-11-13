package data

import (
	"time"

	"github.com/go-keel/keel-admin/internal/core/sys/biz"
	"github.com/go-keel/keel-admin/internal/core/timefmt"
)

// This file holds the POs (GORM structs private to the data layer) of the audit and login logs: each is named after
// its table in lowerCamelCase, with its TableName and toBiz right after the definition. Both tables are append-only
// logs with no delete. The structs match the DDL one to one; the DDL is in migrations/admin/0_schema.sql (AutoMigrate
// is not used).

type sysAuditLog struct {
	ID         int64 `gorm:"primaryKey;autoIncrement"`
	OperatorID int64 `gorm:"column:operator_id"`
	Action     string
	TargetType string `gorm:"column:target_type"`
	TargetID   string `gorm:"column:target_id"`
	// A pointer: nil maps to SQL NULL and means the call had no request body (a non-proto request).
	RequestPayload *string   `gorm:"column:request_payload"`
	Reason         string    `gorm:"default:''"`
	IP             string    `gorm:"default:''"`
	CreatedAt      time.Time `gorm:"autoCreateTime"`
}

func (sysAuditLog) TableName() string { return "sys_audit_log" }

// toBiz converts the PO to a DO (read direction). OperatorName is not in this table; List fills it in a batch query.
func (m *sysAuditLog) toBiz() *biz.AuditLog {
	return &biz.AuditLog{
		ID:             m.ID,
		OperatorID:     m.OperatorID,
		Action:         m.Action,
		TargetType:     m.TargetType,
		TargetID:       m.TargetID,
		IP:             m.IP,
		RequestPayload: deref(m.RequestPayload),
		Reason:         m.Reason,
		CreatedAt:      timefmt.RFC3339(m.CreatedAt),
	}
}

type sysLoginLog struct {
	ID        int64  `gorm:"primaryKey;autoIncrement"`
	UserID    int64  `gorm:"column:user_id;default:0"`
	Username  string `gorm:"default:''"`
	Status    int8
	IP        string    `gorm:"default:''"`
	Msg       string    `gorm:"default:''"`
	CreatedAt time.Time `gorm:"autoCreateTime"`
}

func (sysLoginLog) TableName() string { return "sys_login_log" }

// toBiz converts the PO to a DO (read direction).
func (m *sysLoginLog) toBiz() *biz.LoginRecord {
	return &biz.LoginRecord{
		ID:        m.ID,
		UserID:    m.UserID,
		Username:  m.Username,
		Success:   m.Status == 1,
		IP:        m.IP,
		Msg:       m.Msg,
		CreatedAt: timefmt.RFC3339(m.CreatedAt),
	}
}
