package service

import (
	v1 "github.com/go-keel/keel-admin/api/admin_app/v1"
	"github.com/go-keel/keel-admin/internal/biz"
	"github.com/go-keel/keel-admin/internal/i18n"
	"github.com/go-keel/keel/middleware/sentinel"
)

// RegisterErrors appends the business area's sentinel error → reason mappings to the shared sentinel group (di.go
// starts from the group of core/sys/service.NewErrorSentinel()). A new domain error touches four places, listed in
// docs/admin/errors.md: error_reason.proto, the biz sentinel, the mapping here, and the messages in every
// internal/i18n/app.<lang>.toml.
func RegisterErrors(g *sentinel.Group) *sentinel.Group {
	g.OnLocaled(biz.ErrNoticeNotFound, i18n.Localed("ErrNoticeNotFound", "Notice not found.", v1.ErrorNoticeNotFound))                                      // Example: delete after forking
	g.OnLocaled(biz.ErrNoticeCategoryNotFound, i18n.Localed("ErrNoticeCategoryNotFound", "Notice category not found.", v1.ErrorNoticeCategoryNotFound))     // Example: delete after forking
	g.OnLocaled(biz.ErrNoticeCategoryInUse, i18n.Localed("ErrNoticeCategoryInUse", "Notice category still has notices.", v1.ErrorNoticeCategoryInUse))      // Example: delete after forking
	g.OnLocaled(biz.ErrNoticeCategoryExists, i18n.Localed("ErrNoticeCategoryExists", "Notice category name already exists.", v1.ErrorNoticeCategoryExists)) // Example: delete after forking
	return g
}
