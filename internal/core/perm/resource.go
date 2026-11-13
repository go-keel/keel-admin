package perm

import (
	"context"
	"fmt"
)

// Anchor is a bit-flag set of data-scope anchors a resource supports.
// A resource's Anchors declare which ScopeType strategies are meaningful
// for it (e.g. a resource with no dept anchor cannot use DEPT/ALL_SUB).
type Anchor uint8

const (
	AnchorSelf Anchor = 1 << iota
	AnchorDept
)

// Derive marks a resource as scoped through a parent resource's anchors
// (e.g. "order_item" derives its scope from "order" via an "order_id" FK).
// Only one level of derivation is allowed: Parent must not itself be a
// derived resource (enforced by Seal).
type Derive struct {
	Parent Resource
	FK     string
}

// ResourceDef declares a dimension-two data-scope resource: its own anchors
// (Self/Dept) or, for derived resources, the parent it borrows anchors from.
type ResourceDef struct {
	Res     Resource
	Anchors Anchor
	Via     *Derive // Non-nil marks a derived resource; one level only (Parent must not be derived, Seal checks).

	// NameKey is the i18n message ID of the resource's display name (not a literal name). ListResources translates it
	// into name on the spot by the request's Accept-Language, the same server-side localisation path as error
	// messages. When empty, the frontend falls back to showing the slug; the permission configuration UI already
	// treats the slug as the stable identifier, so a missing key misleads nobody. The business area fills it in when
	// registering a resource and adds the entry to internal/i18n/app.*.toml.
	NameKey string

	// ListGrantable declares that the resource can be granted by explicit assignment (RESOURCE_GRANT) and returns the
	// option set of grantable instances (the multi-select in the role data-scope UI).
	//
	// RESOURCE_GRANT means "the role is explicitly granted a set of instances of this resource" (role × resource type
	// × instance ID set, stored in sys_role_resource_grant, resolved in the ScopeGrant branch of core/sys/biz.Resolve).
	// The kernel does not interpret what an instance is (a brand group, a category, a store): it stores and returns
	// IDs only, and expanding and consuming them belongs to the business. The instance list is runtime business data
	// the kernel cannot enumerate, so the business provides this callback at registration (the closure holds the
	// business's own repo, so the dependency direction is not inverted; same shape as DeptOccupancy).
	//
	// Empty means the resource takes no part in explicit assignment: RESOURCE_GRANT cannot be configured for it
	// (disabled in the frontend and rejected by the server). Scope of use: explicit assignment is for resources with
	// few, stable instances (groups or categories); the callback has no paging, which is how the mechanism expresses
	// that limit. Resources with huge instance counts (customers, orders) use owner columns or a business-built filter.
	ListGrantable func(ctx context.Context) ([]GrantOption, error)

	// DeptOccupancy reports whether the given dept still owns at least one
	// row of this resource. The occupancy check before deleting a dept uses it to decide whether the dept is empty,
	// so the kernel never hard-codes a business table name. It may be nil: it only makes sense for a resource that
	// declares AnchorDept. The business area builds the closure at registration (holding its own repo/db), and the
	// kernel (core/sys/biz.DeptUseCase) only obtains and calls it through ResourcesWithDeptOccupancy without importing
	// the business package, so the dependency direction is not inverted. Seal does not require it to be non-nil.
	DeptOccupancy func(ctx context.Context, deptID int64) (bool, error)
}

// GrantOption is one grantable instance of a resource (candidate for the
// explicit-grant picker). Label is the display name from business data (such as a category name), not an i18n
// entry, because the instance itself is runtime business data.
type GrantOption struct {
	ID    int64
	Label string
}

// MaxGrantOptions is the hard limit on grantable options for one resource.
//
// Explicit assignment is for resources with few, stable instances (groups or categories): an administrator ticking
// them one by one and the id IN (...) at resolution time only hold at that size. Options approaching the limit mean
// the resource has crossed that boundary and the design needs another look: switch to an owner column or a
// business-built ScopeResolver instead of raising the limit.
const MaxGrantOptions = 100

// GrantOptions invokes def.ListGrantable and enforces MaxGrantOptions. Both the configuration side (meta
// ListResources) and the validation side (CreateRole / UpdateRole) take options through it. Over the limit it returns an
// error instead of truncating: a truncated option set would have administrators configure grants from incomplete
// choices, while an error exposes "this resource should not use explicit assignment" at configuration time.
// It returns (nil, nil) when ListGrantable is not declared.
func GrantOptions(ctx context.Context, def ResourceDef) ([]GrantOption, error) {
	if def.ListGrantable == nil {
		return nil, nil
	}
	opts, err := def.ListGrantable(ctx)
	if err != nil {
		return nil, err
	}
	if len(opts) > MaxGrantOptions {
		return nil, fmt.Errorf(
			"perm: resource %s has %d grantable options (limit %d) — the resource has outgrown explicit-grant scoping; use owner columns or a custom ScopeResolver instead",
			def.Res, len(opts), MaxGrantOptions)
	}
	return opts, nil
}

// ResourceByType looks up a registered resource definition.
func ResourceByType(r Resource) (ResourceDef, bool) {
	d, ok := regRes[r]
	return d, ok
}

// Resources returns all registered resource definitions (read-only usage
// expected).
func Resources() []ResourceDef {
	out := make([]ResourceDef, 0, len(regRes))
	for _, d := range regRes {
		out = append(out, d)
	}
	return out
}

// ResourcesWithDeptOccupancy returns the subset of registered resource
// definitions that declared a non-nil DeptOccupancy callback — the set
// biz.DeptUseCase.Delete iterates to check whether a dept can be safely
// removed, without the kernel ever naming a business table.
func ResourcesWithDeptOccupancy() []ResourceDef {
	out := make([]ResourceDef, 0, len(regRes))
	for _, d := range regRes {
		if d.DeptOccupancy != nil {
			out = append(out, d)
		}
	}
	return out
}
