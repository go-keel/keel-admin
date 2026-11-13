package middleware

import (
	"context"
	"strconv"
	"strings"

	"github.com/go-kratos/kratos/v3/middleware"
	"github.com/go-kratos/kratos/v3/transport"

	"github.com/go-keel/keel-admin/internal/core/perm"
	kauth "github.com/go-keel/keel/middleware/auth"
	"github.com/go-keel/keel/mw"
)

// PreviewHeader carries the role IDs (CSV) that root is previewing. It takes effect only when the resolved principal
// is root: it only reduces permissions, never raises them, and a non-root request with this header is ignored.
const PreviewHeader = "X-Preview-Roles"

// PrincipalResolver loads the full principal (roles → effective perms) for a
// user, rejecting stale sessions whose token ver claim no longer matches the
// persisted token_version. Implemented by the biz layer.
type PrincipalResolver interface {
	ResolveWithVersion(ctx context.Context, userID int64, ver int) (*perm.Principal, error)
	// RolePerms resolves role IDs into the effective permission set; unknown
	// IDs contribute nothing.
	RolePerms(ctx context.Context, roleIDs []int64) (perm.Set, error)
}

// Principal reads JWT claims already verified by Keel's auth middleware
// (mw.SlotAuth) and resolves the caller into a perm.Principal in ctx.
// Login-state (token validity) is enforced upstream by Keel auth — here we
// also enforce token_version freshness (session invalidation on password
// change/reset, disable, or kick) before translating claims into a principal.
func Principal(resolver PrincipalResolver) mw.Middleware {
	return func(next middleware.Handler) middleware.Handler {
		return func(ctx context.Context, req any) (any, error) {
			claims, ok := kauth.FromContext(ctx)
			if !ok {
				return next(ctx, req) // anonymous (e.g. Login); Authz decides
			}
			sub, _ := claims["sub"].(string)
			id, err := strconv.ParseInt(sub, 10, 64)
			if err != nil || id == 0 {
				return next(ctx, req)
			}
			ver, _ := claims["ver"].(float64)
			p, err := resolver.ResolveWithVersion(ctx, id, int(ver))
			if err != nil {
				return next(ctx, req)
			}
			if p.IsRoot {
				if tr, ok := transport.FromServerContext(ctx); ok {
					p = preview(ctx, resolver, p, tr.RequestHeader().Get(PreviewHeader))
				}
			}
			return next(perm.NewContext(ctx, p), req)
		}
	}
}

// preview turns root into an ordinary principal holding only the roles listed in the header (reduced-permission
// preview). UserID/DeptID stay root's own: it simulates "only these roles", not another user, so audit attribution
// and the "own dept" of data scope are computed for root. If resolution fails, root stays as is: role codes are
// cached and failures are rare, the subject is root anyway, and the worst case is one request not reduced.
func preview(ctx context.Context, resolver PrincipalResolver, p *perm.Principal, hdr string) *perm.Principal {
	if hdr == "" {
		return p
	}
	var ids []int64
	for _, s := range strings.Split(hdr, ",") {
		if id, err := strconv.ParseInt(strings.TrimSpace(s), 10, 64); err == nil && id > 0 {
			ids = append(ids, id)
		}
	}
	if len(ids) == 0 {
		return p
	}
	perms, err := resolver.RolePerms(ctx, ids)
	if err != nil {
		return p
	}
	return &perm.Principal{UserID: p.UserID, DeptID: p.DeptID, RoleIDs: ids, Perms: perms}
}
