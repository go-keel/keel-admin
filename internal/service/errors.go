package service

import (
	goi18n "github.com/nicksnyder/go-i18n/v2/i18n"

	v1 "github.com/go-keel/keel-admin/api/admin_app/v1"
	"github.com/go-keel/keel-admin/internal/biz"
	"github.com/go-keel/keel/middleware/sentinel"
)

// RegisterErrors appends the business area's sentinel error → reason mappings to the shared sentinel group (di.go
// starts from the group of core/sys/service.NewErrorSentinel()). A new domain error touches four places, listed in
// docs/admin/errors.md: error_reason.proto, the biz sentinel, the mapping here, and the messages in every
// internal/i18n/app.<lang>.toml.
func RegisterErrors(g *sentinel.Group) *sentinel.Group {
	g.OnMessage(biz.ErrNoticeNotFound, &goi18n.Message{ID: "ErrNoticeNotFound", Other: "Announcement not found."}, v1.ErrorNoticeNotFound)                                                        // Example: delete after forking
	g.OnMessage(biz.ErrNoticeCategoryNotFound, &goi18n.Message{ID: "ErrNoticeCategoryNotFound", Other: "Announcement category not found."}, v1.ErrorNoticeCategoryNotFound)                       // Example: delete after forking
	g.OnMessage(biz.ErrNoticeCategoryInUse, &goi18n.Message{ID: "ErrNoticeCategoryInUse", Other: "This category still has announcements, so it cannot be deleted."}, v1.ErrorNoticeCategoryInUse) // Example: delete after forking
	g.OnMessage(biz.ErrNoticeCategoryExists, &goi18n.Message{ID: "ErrNoticeCategoryExists", Other: "An announcement category with this name already exists."}, v1.ErrorNoticeCategoryExists)      // Example: delete after forking
	return g
}
