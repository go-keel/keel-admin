package biz_test

import (
	"context"
	"testing"

	"github.com/go-keel/keel-admin/internal/core/perm"
	"github.com/go-keel/keel-admin/internal/core/sys/biz"
)

type fakeScopeRepo struct {
	scopes  []biz.RoleScope
	subtree []int64
	// grantIDs is returned when roleIDs is non-empty: RoleGrantedResourceIDs is keyed by the role set, and the
	// roleIDs passed in are recorded here to assert that resolution uses the actor's roles, not the user or
	// department.
	grantIDs   []int64
	gotRoleIDs []int64
	// scopeRoleIDs records the roles RoleScopes was asked for.
	scopeRoleIDs []int64
	revokedType  string
	revokedID    int64
}

func (f *fakeScopeRepo) RoleScopes(_ context.Context, roleIDs []int64, _ string) ([]biz.RoleScope, error) {
	f.scopeRoleIDs = roleIDs
	return f.scopes, nil
}
func (f *fakeScopeRepo) SubtreeDeptIDs(context.Context, int64) ([]int64, error) {
	return f.subtree, nil
}
func (f *fakeScopeRepo) RoleGrantedResourceIDs(_ context.Context, roleIDs []int64, _ string) ([]int64, error) {
	f.gotRoleIDs = roleIDs
	return f.grantIDs, nil
}
func (f *fakeScopeRepo) DeleteGrantsByResource(_ context.Context, resourceType string, resourceID int64) error {
	f.revokedType, f.revokedID = resourceType, resourceID
	return nil
}

func TestScope_RootIsAll(t *testing.T) {
	uc := biz.NewScopeUseCase(&fakeScopeRepo{})
	spec, _ := uc.Resolve(context.Background(), &perm.Principal{UserID: 1, DeptID: 1, IsRoot: true}, "notice")
	if !spec.All {
		t.Fatal("root must get All")
	}
}

func TestScope_AllShortCircuits(t *testing.T) {
	uc := biz.NewScopeUseCase(&fakeScopeRepo{scopes: []biz.RoleScope{{ScopeType: biz.ScopeSelf}, {ScopeType: biz.ScopeAll}}})
	spec, _ := uc.Resolve(context.Background(), &perm.Principal{UserID: 7, DeptID: 3}, "notice")
	if !spec.All {
		t.Fatal("ALL must short-circuit to unrestricted")
	}
}

func TestScope_UnionSelfDeptGrant(t *testing.T) {
	repo := &fakeScopeRepo{
		scopes:   []biz.RoleScope{{ScopeType: biz.ScopeSelf}, {ScopeType: biz.ScopeAllSub}, {ScopeType: biz.ScopeGrant}},
		subtree:  []int64{3, 4, 5},
		grantIDs: []int64{100, 101},
	}
	uc := biz.NewScopeUseCase(repo)
	actor := &perm.Principal{UserID: 7, DeptID: 3, RoleIDs: []int64{20, 21}}
	spec, _ := uc.Resolve(context.Background(), actor, "notice")
	if spec.All {
		t.Fatal("should not be All")
	}
	if spec.SelfUserID != 7 {
		t.Fatal("Self missing")
	}
	if len(spec.DeptIDs) != 3 {
		t.Fatalf("want 3 dept ids, got %v", spec.DeptIDs)
	}
	if len(spec.ResourceIDs) != 2 {
		t.Fatalf("want 2 resource ids, got %v", spec.ResourceIDs)
	}
	// Explicit assignment resolves by the actor's role set: not the user id, not the department subtree.
	if len(repo.gotRoleIDs) != 2 || repo.gotRoleIDs[0] != 20 || repo.gotRoleIDs[1] != 21 {
		t.Fatalf("grant resolution must use actor.RoleIDs, got %v", repo.gotRoleIDs)
	}
}

func TestScope_DeptOnlySelfDept(t *testing.T) {
	uc := biz.NewScopeUseCase(&fakeScopeRepo{scopes: []biz.RoleScope{{ScopeType: biz.ScopeDept}}})
	spec, _ := uc.Resolve(context.Background(), &perm.Principal{UserID: 7, DeptID: 9}, "notice")
	if len(spec.DeptIDs) != 1 || spec.DeptIDs[0] != 9 {
		t.Fatalf("DEPT must yield own dept, got %v", spec.DeptIDs)
	}
}

// No policy at all gives the zero-value spec (deny-all; permdata.ApplyScope falls back to WHERE 1=0, and a
// fan-out consumer must explicitly treat an empty ResourceIDs as "call none").
func TestScope_NoPolicyIsDenyAll(t *testing.T) {
	uc := biz.NewScopeUseCase(&fakeScopeRepo{})
	spec, _ := uc.Resolve(context.Background(), &perm.Principal{UserID: 7, DeptID: 3, RoleIDs: []int64{20}}, "notice")
	if spec.All || spec.SelfUserID != 0 || len(spec.DeptIDs) != 0 || len(spec.ResourceIDs) != 0 {
		t.Fatalf("no policy must resolve to the zero spec, got %+v", spec)
	}
}

// RevokeGrants (perm.GrantRevoker) forwards to the repo: when the business deletes a grantable instance, it
// cleans up the grant rows through this port instead of writing sys_* tables directly.
func TestScope_RevokeGrantsDelegates(t *testing.T) {
	repo := &fakeScopeRepo{}
	uc := biz.NewScopeUseCase(repo)
	if err := uc.RevokeGrants(context.Background(), "notice_category", 42); err != nil {
		t.Fatalf("RevokeGrants: %v", err)
	}
	if repo.revokedType != "notice_category" || repo.revokedID != 42 {
		t.Fatalf("revoke got (%s,%d), want (notice_category,42)", repo.revokedType, repo.revokedID)
	}
}

// A role preview carries only the previewed roles and root's own user id: scope policies must come from those roles,
// or root (who holds no roles) would preview a deny-all scope instead of the roles' range.
func TestScope_PoliciesFollowPrincipalRoles(t *testing.T) {
	repo := &fakeScopeRepo{scopes: []biz.RoleScope{{ScopeType: biz.ScopeDept}}}
	uc := biz.NewScopeUseCase(repo)
	if _, err := uc.Resolve(context.Background(), &perm.Principal{UserID: 1, DeptID: 3, RoleIDs: []int64{20}}, "notice"); err != nil {
		t.Fatalf("Resolve: %v", err)
	}
	if len(repo.scopeRoleIDs) != 1 || repo.scopeRoleIDs[0] != 20 {
		t.Fatalf("scope policies must be loaded for actor.RoleIDs, got %v", repo.scopeRoleIDs)
	}
}
