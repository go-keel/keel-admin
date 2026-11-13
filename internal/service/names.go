package service

import "context"

// NameResolver resolves kernel entity ids to display names in bulk, so columns of business lists such as "department"
// or "created by" render names returned by the backend. If the frontend queried the department or user endpoints
// itself, the permission threshold would be higher than the page's own.
// The business area does not import core/sys: di.go injects the kernel's refRepo, which satisfies this interface. Ids
// that cannot be found (deleted) are absent from the result.
type NameResolver interface {
	DeptNames(ctx context.Context, ids []int64) (map[int64]string, error)
	UserNames(ctx context.Context, ids []int64) (map[int64]string, error)
}
