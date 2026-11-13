package data

import (
	"time"

	"gorm.io/gorm"

	"github.com/go-keel/keel-admin/internal/core/sys/biz"
)

// This file holds the department domain's POs (GORM structs private to the data layer): each is named after its
// table in lowerCamelCase, with its TableName and toBiz right after the definition. The structs match the DDL one to
// one; the DDL is in migrations/admin/0_schema.sql (AutoMigrate is not used).

type sysDept struct {
	ID        int64  `gorm:"primaryKey;autoIncrement"`
	ParentID  int64  `gorm:"column:parent_id;default:0"`
	Ancestors string `gorm:"column:ancestors;default:''"`
	Name      string
	Sort      int            `gorm:"default:0"`
	Status    int8           `gorm:"default:1"`
	CreatedAt time.Time      `gorm:"autoCreateTime"`
	UpdatedAt time.Time      `gorm:"autoUpdateTime"`
	DeletedAt gorm.DeletedAt `gorm:"column:deleted_at"`
}

func (sysDept) TableName() string { return "sys_dept" }

// toBiz converts the PO to a DO (read direction).
func (m *sysDept) toBiz() *biz.Dept {
	return &biz.Dept{
		ID:        m.ID,
		ParentID:  m.ParentID,
		Ancestors: m.Ancestors,
		Name:      m.Name,
		Sort:      m.Sort,
		Status:    m.Status,
	}
}
