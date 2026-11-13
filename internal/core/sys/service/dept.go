package service

import (
	"context"

	v1 "github.com/go-keel/keel-admin/api/keel_admin/v1"
	"github.com/go-keel/keel-admin/internal/core/sys/biz"
	"google.golang.org/protobuf/types/known/emptypb"
)

// DeptService implements api.keel_admin.v1.DeptService.
type DeptService struct {
	v1.UnsafeDeptServiceServer
	uc *biz.DeptUseCase
}

// NewDeptService builds the dept service handler.
func NewDeptService(uc *biz.DeptUseCase) *DeptService { return &DeptService{uc: uc} }

func (s *DeptService) ListDept(ctx context.Context, _ *v1.ListDeptRequest) (*v1.ListDeptReply, error) {
	actor, err := actorFrom(ctx)
	if err != nil {
		return nil, err
	}
	ds, err := s.uc.List(ctx, actor)
	if err != nil {
		return nil, err
	}
	items := make([]*v1.DeptListItem, 0, len(ds))
	for _, d := range ds {
		items = append(items, &v1.DeptListItem{
			Id:        d.ID,
			ParentId:  d.ParentID,
			Name:      d.Name,
			Sort:      int32(d.Sort),
			Status:    int32(d.Status),
			UserCount: int32(d.UserCount),
		})
	}
	return &v1.ListDeptReply{Items: items}, nil
}

// ListDeptOptions returns the department picker's option set (slim: id/parent_id/name). It is bound to
// sys:account:view rather than this group's sys:dept:view because the bar follows the purpose; the reasons are in
// dept.proto and perm.go. It reuses List's manageable-range filter (everything for root), the same criterion as the
// user list.
func (s *DeptService) ListDeptOptions(ctx context.Context, _ *v1.ListDeptOptionsRequest) (*v1.ListDeptOptionsReply, error) {
	actor, err := actorFrom(ctx)
	if err != nil {
		return nil, err
	}
	ds, err := s.uc.List(ctx, actor)
	if err != nil {
		return nil, err
	}
	items := make([]*v1.DeptOption, 0, len(ds))
	for _, d := range ds {
		items = append(items, &v1.DeptOption{Id: d.ID, ParentId: d.ParentID, Name: d.Name, Disabled: d.Status != 1})
	}
	return &v1.ListDeptOptionsReply{Items: items}, nil
}

// GetDept supplies the initial values of the edit form (bound to sys:dept:view, see perm.go).
func (s *DeptService) GetDept(ctx context.Context, req *v1.GetDeptRequest) (*v1.DeptInfo, error) {
	actor, err := actorFrom(ctx)
	if err != nil {
		return nil, err
	}
	d, err := s.uc.Get(ctx, actor, req.GetId())
	if err != nil {
		return nil, err
	}
	return toDeptInfo(d), nil
}

func (s *DeptService) CreateDept(ctx context.Context, req *v1.CreateDeptRequest) (*v1.DeptInfo, error) {
	actor, err := actorFrom(ctx)
	if err != nil {
		return nil, err
	}
	d, err := s.uc.Create(ctx, actor, req.GetParentId(), req.GetName(), int(req.GetSort()))
	if err != nil {
		return nil, err
	}
	return toDeptInfo(d), nil
}

func (s *DeptService) UpdateDept(ctx context.Context, req *v1.UpdateDeptRequest) (*v1.DeptInfo, error) {
	actor, err := actorFrom(ctx)
	if err != nil {
		return nil, err
	}
	// The organisation tree does not support moving a department: UpdateDeptRequest has no parent_id field, and
	// re-parenting needs its own migration or endpoint (see docs/admin/permission-model/management-boundary.md §6).
	d, err := s.uc.Update(ctx, actor, req.GetId(), req.GetName(), int(req.GetSort()), int8(req.GetStatus()))
	if err != nil {
		return nil, err
	}
	return toDeptInfo(d), nil
}

func (s *DeptService) DeleteDept(ctx context.Context, req *v1.DeleteDeptRequest) (*emptypb.Empty, error) {
	actor, err := actorFrom(ctx)
	if err != nil {
		return nil, err
	}
	if err := s.uc.Delete(ctx, actor, req.GetId()); err != nil {
		return nil, err
	}
	return &emptypb.Empty{}, nil
}

func toDeptInfo(d *biz.Dept) *v1.DeptInfo {
	return &v1.DeptInfo{
		Id:       d.ID,
		ParentId: d.ParentID,
		Name:     d.Name,
		Sort:     int32(d.Sort),
		Status:   int32(d.Status),
	}
}
