package data

import (
	"time"

	"github.com/go-keel/keel-admin/internal/biz"
	"github.com/go-keel/keel-admin/internal/core/timefmt"
)

// The business area's POs (GORM structs private to the data layer), unrelated to the kernel's
// core/sys/data/*_model.go. A PO is named after its table in lower camel case, without a Model suffix; its TableName,
// toBiz and new<Resource> follow right after the definition; beyond 5 POs, split into {domain}_model.go per domain.
// The tables match migrations/business/0_schema.sql one to one (no AutoMigrate, see docs/practices/database.md).

// Example: delete after forking (to the end of the file)

// notice is the GORM model for the notice table. Its owner_user_id/
// owner_dept_id columns are permdata.ApplyScope's default scope columns
// (contrast a non-default column mapping, e.g. dept_id/created_by, which
// needs permdata.ApplyScopeWith to remap). category_id is gated by category grants
// (ApplyScopeWith(catSpec, Cols{PK: "category_id"}), see notice.go).
type notice struct {
	ID          int64 `gorm:"primaryKey;autoIncrement"`
	Title       string
	Content     string
	CategoryID  int64     `gorm:"column:category_id"`
	OwnerUserID int64     `gorm:"column:owner_user_id"`
	OwnerDeptID int64     `gorm:"column:owner_dept_id"`
	CreatedAt   time.Time `gorm:"autoCreateTime"`
}

func (notice) TableName() string { return "notice" }

// newNotice converts DO → PO (write direction). The database generates ID and CreatedAt; they are not taken from
// the DO.
func newNotice(n *biz.Notice) *notice {
	return &notice{
		Title:       n.Title,
		Content:     n.Content,
		CategoryID:  n.CategoryID,
		OwnerUserID: n.OwnerUserID,
		OwnerDeptID: n.OwnerDeptID,
	}
}

// toBiz converts PO → DO (read direction).
func (m *notice) toBiz() *biz.Notice {
	return &biz.Notice{
		ID:          m.ID,
		Title:       m.Title,
		Content:     m.Content,
		CategoryID:  m.CategoryID,
		OwnerUserID: m.OwnerUserID,
		OwnerDeptID: m.OwnerDeptID,
		CreatedAt:   timefmt.RFC3339(m.CreatedAt),
	}
}

// noticeCategory is the GORM model for notice_category, a grantable instance of explicit assignment
// (RESOURCE_GRANT). It has no owner columns: categories are not isolated by user or department; visibility is decided
// by the role's category grants (sys_role_resource_grant, on the kernel side).
type noticeCategory struct {
	ID        int64 `gorm:"primaryKey;autoIncrement"`
	Name      string
	CreatedAt time.Time `gorm:"autoCreateTime"`
}

func (noticeCategory) TableName() string { return "notice_category" }

// newNoticeCategory converts DO → PO (write direction). The database generates ID and CreatedAt; they are not taken
// from the DO.
func newNoticeCategory(c *biz.NoticeCategory) *noticeCategory {
	return &noticeCategory{Name: c.Name}
}

// toBiz converts PO → DO (read direction).
func (m *noticeCategory) toBiz() *biz.NoticeCategory {
	return &biz.NoticeCategory{
		ID:        m.ID,
		Name:      m.Name,
		CreatedAt: timefmt.RFC3339(m.CreatedAt),
	}
}

// toCategoryBiz is the batch version of toBiz: all three category read methods return batches.
func toCategoryBiz(rows []noticeCategory) []*biz.NoticeCategory {
	out := make([]*biz.NoticeCategory, 0, len(rows))
	for i := range rows {
		out = append(out, rows[i].toBiz())
	}
	return out
}
