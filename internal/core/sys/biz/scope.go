package biz

import (
	"context"

	"github.com/go-keel/keel-admin/internal/core/perm"
)

// ScopeType is a code-fixed data-scope strategy.
type ScopeType string

const (
	ScopeSelf   ScopeType = "SELF"
	ScopeDept   ScopeType = "DEPT"
	ScopeAllSub ScopeType = "ALL_SUB"
	ScopeGrant  ScopeType = "RESOURCE_GRANT"
	ScopeAll    ScopeType = "ALL"
)

// RoleScope is one role's scope policy for a resource type.
type RoleScope struct {
	ScopeType ScopeType
}

// ScopeRepo loads role data-scope policies.
type ScopeRepo interface {
	// RoleScopes returns the scope policies of the given (active) roles for a resource type.
	RoleScopes(ctx context.Context, roleIDs []int64, resourceType string) ([]RoleScope, error)
	// SubtreeDeptIDs returns deptID and all descendant dept ids (via ancestors path).
	SubtreeDeptIDs(ctx context.Context, deptID int64) ([]int64, error)
	// RoleGrantedResourceIDs returns the resource instance ids explicitly
	// granted to any of the given roles for a resource type
	// (sys_role_resource_grant; written by RoleRepo.Save).
	RoleGrantedResourceIDs(ctx context.Context, roleIDs []int64, resourceType string) ([]int64, error)
	// DeleteGrantsByResource removes every role's explicit grant of one
	// resource instance (backs perm.GrantRevoker).
	DeleteGrantsByResource(ctx context.Context, resourceType string, resourceID int64) error
}

// ScopeUseCase resolves the effective data scope. Implements
// perm.ScopeResolver and perm.GrantRevoker.
type ScopeUseCase struct {
	repo ScopeRepo
}

var (
	_ perm.ScopeResolver = (*ScopeUseCase)(nil)
	_ perm.GrantRevoker  = (*ScopeUseCase)(nil)
)

// NewScopeUseCase builds the data-scope engine.
func NewScopeUseCase(repo ScopeRepo) *ScopeUseCase { return &ScopeUseCase{repo: repo} }

// Resolve composes the visible range for a resource type (union of all role scopes).
// Root actors get All=true.
func (uc *ScopeUseCase) Resolve(ctx context.Context, actor *perm.Principal, res perm.Resource) (perm.ScopeSpec, error) {
	if actor.IsRoot {
		return perm.ScopeSpec{All: true}, nil
	}
	resourceType := string(res)
	// By the principal's roles, not its user: a role preview carries only the previewed roles.
	scopes, err := uc.repo.RoleScopes(ctx, actor.RoleIDs, resourceType)
	if err != nil {
		return perm.ScopeSpec{}, err
	}
	spec := perm.ScopeSpec{}
	deptSet := map[int64]struct{}{}
	resSet := map[int64]struct{}{}
	for _, s := range scopes {
		switch s.ScopeType {
		case ScopeAll:
			return perm.ScopeSpec{All: true}, nil
		case ScopeSelf:
			spec.SelfUserID = actor.UserID
		case ScopeDept:
			deptSet[actor.DeptID] = struct{}{}
		case ScopeAllSub:
			ids, err := uc.repo.SubtreeDeptIDs(ctx, actor.DeptID)
			if err != nil {
				return perm.ScopeSpec{}, err
			}
			for _, id := range ids {
				deptSet[id] = struct{}{}
			}
		case ScopeGrant:
			// Explicit assignment: the instance list hangs on roles (sys_role_resource_grant); take the union of
			// the instances granted to all of the actor's roles. The kernel does not interpret instances: a
			// consumer either filters local rows with permdata.ApplyScope (id IN) or expands ResourceIDs into the
			// visible set directly (such as category to articles, or a brand group fanning out to brands).
			rids, err := uc.repo.RoleGrantedResourceIDs(ctx, actor.RoleIDs, resourceType)
			if err != nil {
				return perm.ScopeSpec{}, err
			}
			for _, rid := range rids {
				resSet[rid] = struct{}{}
			}
		}
	}
	for id := range deptSet {
		spec.DeptIDs = append(spec.DeptIDs, id)
	}
	for rid := range resSet {
		spec.ResourceIDs = append(spec.ResourceIDs, rid)
	}
	return spec, nil
}

// RevokeGrants implements perm.GrantRevoker: drops every role's explicit
// grant of one resource instance. Business code calls it after deleting a grantable instance (a category, a brand
// group), because business code does not write sys_* tables directly.
func (uc *ScopeUseCase) RevokeGrants(ctx context.Context, res perm.Resource, resourceID int64) error {
	return uc.repo.DeleteGrantsByResource(ctx, string(res), resourceID)
}
