package perm

import "context"

// Principal is the authenticated caller, injected by the principal middleware.
type Principal struct {
	UserID  int64
	IsRoot  bool
	Perms   Set // effective permission set
	DeptID  int64
	RoleIDs []int64
}

type principalKey struct{}

// NewContext stores the principal in the context.
func NewContext(ctx context.Context, p *Principal) context.Context {
	return context.WithValue(ctx, principalKey{}, p)
}

// FromContext extracts the principal from the context.
func FromContext(ctx context.Context) (*Principal, bool) {
	p, ok := ctx.Value(principalKey{}).(*Principal)
	return p, ok
}
