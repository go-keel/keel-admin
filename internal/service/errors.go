package service

import (
	"context"
	"errors"

	kerrors "github.com/go-kratos/kratos/v3/errors"
	goi18n "github.com/nicksnyder/go-i18n/v2/i18n"

	v1 "github.com/go-keel/keel-admin/api/keel_admin/v1"
	"github.com/go-keel/keel-admin/internal/biz"
	"github.com/go-keel/keel-admin/internal/core/perm"
	"github.com/go-keel/keel-admin/internal/i18n"
	"github.com/go-keel/keel/middleware/sentinel"
)

// errNoPrincipal: the ctx has no principal. Bound endpoints never get here (the authz middleware rejects
// unauthenticated calls before the handler); it exists so that a nil *perm.Principal is never dereferenced.
var errNoPrincipal = errors.New("service: missing principal in context")

// actorFromContext returns the principal of the current request, which business-area services pass on to biz.
func actorFromContext(ctx context.Context) (*perm.Principal, error) {
	p, ok := perm.FromContext(ctx)
	if !ok {
		return nil, errNoPrincipal
	}
	return p, nil
}

// localed deliberately duplicates the helper of the same name in core/sys/service instead of importing it: the
// business area does not depend on core/sys at all, its unexported helpers included. internal/i18n is not core/sys:
// it is a neutral leaf package that loads the bundle and falls back to English, imported directly by both core/sys
// and the business area.
func localed(messageID, fallback string, build func(format string, args ...interface{}) *kerrors.Error) func(ctx context.Context, l *goi18n.Localizer, err error) error {
	return func(_ context.Context, l *goi18n.Localizer, _ error) error {
		msg := i18n.MustLocalizeMessage(l, &goi18n.Message{ID: messageID, Other: fallback})
		return build("%s", msg)
	}
}

// The examples are the only callers of these helpers; this keeps them for the first business module after the
// examples are deleted.
var _, _ = actorFromContext, localed

// RegisterErrors appends the business area's sentinel error → reason mappings to the shared sentinel group (di.go
// starts from the group of core/sys/service.NewErrorSentinel()). A new domain error touches four places, listed in
// docs/admin/errors.md: error_reason.proto, the biz sentinel, the mapping here, and the messages in every
// internal/i18n/app.<lang>.toml.
func RegisterErrors(g *sentinel.Group) *sentinel.Group {
	g.OnLocaled(biz.ErrNoticeNotFound, localed("ErrNoticeNotFound", "Notice not found.", v1.ErrorNoticeNotFound))                                      // Example: delete after forking
	g.OnLocaled(biz.ErrNoticeCategoryNotFound, localed("ErrNoticeCategoryNotFound", "Notice category not found.", v1.ErrorNoticeCategoryNotFound))     // Example: delete after forking
	g.OnLocaled(biz.ErrNoticeCategoryInUse, localed("ErrNoticeCategoryInUse", "Notice category still has notices.", v1.ErrorNoticeCategoryInUse))      // Example: delete after forking
	g.OnLocaled(biz.ErrNoticeCategoryExists, localed("ErrNoticeCategoryExists", "Notice category name already exists.", v1.ErrorNoticeCategoryExists)) // Example: delete after forking
	return g
}
