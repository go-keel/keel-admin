package perm

import (
	"context"
	"errors"
)

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

// ErrNoPrincipal means the ctx carries no principal. A bound endpoint never sees it (the authz middleware rejects an
// unauthenticated call first); the kernel's sentinel group maps it to "sign in first" for any other path.
var ErrNoPrincipal = errors.New("perm: no principal in context")

// Current returns the principal of the current request: the operator a service passes on to its use case.
func Current(ctx context.Context) (*Principal, error) {
	if p, ok := FromContext(ctx); ok {
		return p, nil
	}
	return nil, ErrNoPrincipal
}
