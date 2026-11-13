package perm

import "context"

// ScopeSpec is the resolved visible-range for a resource type. Conditions
// are OR-combined (see permdata.ApplyScope); an all-zero-value spec (no
// All, no SelfUserID, no DeptIDs, no ResourceIDs) means deny-all.
type ScopeSpec struct {
	All         bool    // unrestricted
	SelfUserID  int64   // owner = SelfUserID; 0 = no SELF condition
	DeptIDs     []int64 // dept IN (...)
	ResourceIDs []int64 // resource pk IN (...), from RESOURCE_GRANT
}

// ScopeResolver resolves the effective data scope an actor has over a
// resource type. Implemented by each domain's scope use case (e.g.
// sys/biz.ScopeUseCase) and consumed by callers that need row-level
// filtering (e.g. permdata.ApplyScope).
type ScopeResolver interface {
	Resolve(ctx context.Context, actor *Principal, res Resource) (ScopeSpec, error)
}

// GrantRevoker removes all explicit grants of one resource instance across
// every role. The business calls it when deleting a grantable instance (a category or brand group): only the kernel
// reads and writes sys_* tables, so the business asks for the cleanup through this port instead of deleting grant rows
// itself (see the sys/biz.ScopeUseCase implementation). Dangling grant rows do not widen permissions (the instance is
// gone, so the resolved IDs match no data); the cleanup keeps the role configuration from showing stale entries.
type GrantRevoker interface {
	RevokeGrants(ctx context.Context, res Resource, resourceID int64) error
}
