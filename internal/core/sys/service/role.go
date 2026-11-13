package service

import (
	"context"

	v1 "github.com/go-keel/keel-admin/api/keel_admin/v1"
	"github.com/go-keel/keel-admin/internal/core/sys/biz"
	"google.golang.org/protobuf/types/known/emptypb"
)

// RoleService implements api.keel_admin.v1.RoleService (root-only enforced at transport).
type RoleService struct {
	v1.UnsafeRoleServiceServer
	uc  *biz.RoleUseCase
	ref biz.RefRepo
}

// NewRoleService builds the role service handler.
func NewRoleService(uc *biz.RoleUseCase, ref biz.RefRepo) *RoleService {
	return &RoleService{uc: uc, ref: ref}
}

func (s *RoleService) ListRole(ctx context.Context, req *v1.ListRoleRequest) (*v1.ListRoleReply, error) {
	rs, total, err := s.uc.List(ctx, biz.RoleFilter{
		Keyword:   req.GetKeyword(),
		Status:    req.GetStatus(),
		Sensitive: req.GetSensitive(),
		ForceOTP:  req.GetForceOtp(),
	}, int(req.GetOffset()), int(req.GetLimit()))
	if err != nil {
		return nil, err
	}
	items := make([]*v1.RoleListItem, 0, len(rs))
	for _, r := range rs {
		items = append(items, &v1.RoleListItem{
			Id:        r.ID,
			Code:      r.Code,
			Name:      r.Name,
			Sensitive: int32(r.Sensitive),
			ForceOtp:  int32(r.ForceOTP),
			Status:    int32(r.Status),
			PermCount: r.PermCount,
			CreatedAt: r.CreatedAt,
		})
	}
	return &v1.ListRoleReply{Items: items, Total: total}, nil
}

// ListAssignableRoleOptions is not root-only (binding in perm.go): it returns the roles the operator may assign. It
// is an option set, neither paged nor carrying total (reasons in role.proto).
//
// The elements are RoleOption rather than RoleInfo because the dropdown only needs names. Sending RoleInfo would
// drag the perms and three-dimension boundary fields along with every item: even always empty, they would announce
// to non-root callers "each role's codes belong here", and the next person would naturally fill them in.
func (s *RoleService) ListAssignableRoleOptions(ctx context.Context, _ *v1.ListAssignableRoleOptionsRequest) (*v1.ListAssignableRoleOptionsReply, error) {
	actor, err := actorFrom(ctx)
	if err != nil {
		return nil, err
	}
	rs, err := s.uc.ListAssignable(ctx, actor)
	if err != nil {
		return nil, err
	}
	items := make([]*v1.RoleOption, 0, len(rs))
	for _, r := range rs {
		items = append(items, &v1.RoleOption{Id: r.ID, Code: r.Code, Name: r.Name})
	}
	return &v1.ListAssignableRoleOptionsReply{Items: items}, nil
}

// ListRoleOptions returns the role picker's option set (slim: id/code/name). It is bound to sys:account:view rather
// than this group's root-only because the bar follows the purpose; the reasons are in role.proto and perm.go.
func (s *RoleService) ListRoleOptions(ctx context.Context, req *v1.ListRoleOptionsRequest) (*v1.ListRoleOptionsReply, error) {
	rs, err := s.uc.Options(ctx, req.GetExcludeId(), req.GetExcludeSensitive())
	if err != nil {
		return nil, err
	}
	items := make([]*v1.RoleOption, 0, len(rs))
	for _, r := range rs {
		items = append(items, &v1.RoleOption{Id: r.ID, Code: r.Code, Name: r.Name})
	}
	return &v1.ListRoleOptionsReply{Items: items}, nil
}

// GetRolePermissions serves business administrators who may assign the role (not root-only, see perm.go): it gives
// only the permission codes, not the role's configuration, which is GetRole's job and root-only.
func (s *RoleService) GetRolePermissions(ctx context.Context, req *v1.GetRolePermissionsRequest) (*v1.GetRolePermissionsReply, error) {
	actor, err := actorFrom(ctx)
	if err != nil {
		return nil, err
	}
	perms, err := s.uc.PermissionsOf(ctx, actor, req.GetId())
	if err != nil {
		return nil, err
	}
	return &v1.GetRolePermissionsReply{Perms: perms}, nil
}

// GetRole supplies the initial values of the edit form (root-only, see perm.go).
func (s *RoleService) GetRole(ctx context.Context, req *v1.GetRoleRequest) (*v1.RoleInfo, error) {
	r, err := s.uc.Get(ctx, req.GetId())
	if err != nil {
		return nil, err
	}
	return s.toRoleInfo(ctx, r), nil
}

// toScopeInputs converts the proto DataScopeItem list into biz input.
func toScopeInputs(items []*v1.DataScopeItem) []biz.DataScopeInput {
	scopes := make([]biz.DataScopeInput, 0, len(items))
	for _, it := range items {
		scopes = append(scopes, biz.DataScopeInput{
			ResourceType: it.GetResourceType(),
			ScopeType:    biz.ScopeType(it.GetScopeType()),
			ResourceIDs:  it.GetResourceIds(),
		})
	}
	return scopes
}

// CreateRole creates a role with every editable field at once (the same form as editing). status is stored as
// given, so a role can be created disabled.
func (s *RoleService) CreateRole(ctx context.Context, req *v1.CreateRoleRequest) (*v1.RoleInfo, error) {
	r, err := s.uc.Create(ctx, req.GetCode(), biz.RoleUpdate{
		Name:              req.GetName(),
		ForceOTP:          int8(req.GetForceOtp()),
		Status:            int8(req.GetStatus()),
		Sensitive:         int8(req.GetSensitive()),
		Perms:             req.GetPerms(),
		AssignableRoleIDs: req.GetAssignableRoleIds(),
		ManageDepts:       req.GetManageDeptIds(),
		Scopes:            toScopeInputs(req.GetScopes()),
	})
	if err != nil {
		return nil, err
	}
	return s.toRoleInfo(ctx, r), nil
}

// UpdateRole changes every editable field of a role at once (why it is one call: role.proto / biz.RoleUpdate).
func (s *RoleService) UpdateRole(ctx context.Context, req *v1.UpdateRoleRequest) (*v1.RoleInfo, error) {
	r, err := s.uc.Update(ctx, req.GetId(), biz.RoleUpdate{
		Name:              req.GetName(),
		ForceOTP:          int8(req.GetForceOtp()),
		Status:            int8(req.GetStatus()),
		Sensitive:         int8(req.GetSensitive()),
		Perms:             req.GetPerms(),
		AssignableRoleIDs: req.GetAssignableRoleIds(),
		ManageDepts:       req.GetManageDeptIds(),
		Scopes:            toScopeInputs(req.GetScopes()),
	})
	if err != nil {
		return nil, err
	}
	return s.toRoleInfo(ctx, r), nil
}

func (s *RoleService) DeleteRole(ctx context.Context, req *v1.DeleteRoleRequest) (*emptypb.Empty, error) {
	if err := s.uc.Delete(ctx, req.GetId()); err != nil {
		return nil, err
	}
	return &emptypb.Empty{}, nil
}

func (s *RoleService) toRoleInfo(ctx context.Context, r *biz.Role) *v1.RoleInfo {
	scopes := make([]*v1.DataScopeItem, 0, len(r.Scopes))
	for _, sc := range r.Scopes {
		scopes = append(scopes, &v1.DataScopeItem{
			ResourceType: sc.ResourceType,
			ScopeType:    string(sc.ScopeType),
			ResourceIds:  sc.ResourceIDs,
		})
	}
	// Assignable roles and manageable departments carry names resolved by the backend, so the detail view renders
	// them directly (dynamic values are not mapped in the frontend).
	roleNames, _ := s.ref.RoleNames(ctx, r.AssignableRoleIDs)
	deptNames, _ := s.ref.DeptNames(ctx, r.ManageDepts)
	return &v1.RoleInfo{
		Id:              r.ID,
		Code:            r.Code,
		Name:            r.Name,
		Sensitive:       int32(r.Sensitive),
		ForceOtp:        int32(r.ForceOTP),
		Status:          int32(r.Status),
		Perms:           r.Perms,
		AssignableRoles: toNamedRefs(r.AssignableRoleIDs, roleNames),
		ManageDepts:     toNamedRefs(r.ManageDepts, deptNames),
		Scopes:          scopes,
	}
}

// toNamedRefs pairs ids with their resolved names (a missing name becomes an empty string and the frontend shows
// #id).
func toNamedRefs(ids []int64, names map[int64]string) []*v1.NamedRef {
	out := make([]*v1.NamedRef, 0, len(ids))
	for _, id := range ids {
		out = append(out, &v1.NamedRef{Id: id, Name: names[id]})
	}
	return out
}
