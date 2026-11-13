// Example file: delete the whole file after forking

package service

import (
	"context"
	"time"

	v1 "github.com/go-keel/keel-admin/api/keel_admin/v1"
	"github.com/go-keel/keel-admin/internal/biz"
	"github.com/go-keel/keel-admin/internal/core/timefmt"
	"google.golang.org/protobuf/types/known/emptypb"
)

// NoticeService implements api.keel_admin.v1.NoticeService — the notice/announcement
// business reference module (business area: internal/biz|data|service).
type NoticeService struct {
	v1.UnsafeNoticeServiceServer
	uc    *biz.NoticeUseCase
	names NameResolver
	// Business time zone: business-day filters on the creation time convert to instants by it (ADR admin-0003).
	businessZone *time.Location
}

// NewNoticeService builds the notice service handler.
func NewNoticeService(uc *biz.NoticeUseCase, names NameResolver, businessZone *time.Location) *NoticeService {
	return &NoticeService{uc: uc, names: names, businessZone: businessZone}
}

func (s *NoticeService) ListNotice(ctx context.Context, req *v1.ListNoticeRequest) (*v1.ListNoticeReply, error) {
	actor, err := actorFromContext(ctx)
	if err != nil {
		return nil, err
	}
	from, to, err := timefmt.ParseDateRange(req.GetCreatedFromDate(), req.GetCreatedToDate(), s.businessZone)
	if err != nil {
		return nil, err
	}
	f := biz.NoticeFilter{CategoryID: req.GetCategoryId(), Keyword: req.GetKeyword(), CreatedFrom: from, CreatedTo: to}
	ns, total, err := s.uc.List(ctx, actor, f, int(req.GetOffset()), int(req.GetLimit()))
	if err != nil {
		return nil, err
	}
	items := make([]*v1.NoticeListItem, 0, len(ns))
	for _, n := range ns {
		items = append(items, toNoticeListItem(n))
	}
	if err := s.fillOwnerNames(ctx, items); err != nil {
		return nil, err
	}
	return &v1.ListNoticeReply{Items: items, Total: total}, nil
}

func (s *NoticeService) CreateNotice(ctx context.Context, req *v1.CreateNoticeRequest) (*v1.NoticeReply, error) {
	actor, err := actorFromContext(ctx)
	if err != nil {
		return nil, err
	}
	n, err := s.uc.Create(ctx, actor, req.GetTitle(), req.GetContent(), req.GetCategoryId())
	if err != nil {
		return nil, err
	}
	return toNoticeReply(n), nil
}

// ListNoticeCategoryOptions is the category selector's option set (bound to ops:notice:view): the notice page's filter
// dropdown, submission form and category names of list rows all need it, so whoever can open the page can fetch it.
func (s *NoticeService) ListNoticeCategoryOptions(ctx context.Context, _ *v1.ListNoticeCategoryOptionsRequest) (*v1.ListNoticeCategoryOptionsReply, error) {
	actor, err := actorFromContext(ctx)
	if err != nil {
		return nil, err
	}
	cs, err := s.uc.CategoryOptions(ctx, actor)
	if err != nil {
		return nil, err
	}
	items := make([]*v1.NoticeCategoryOption, 0, len(cs))
	for _, c := range cs {
		items = append(items, &v1.NoticeCategoryOption{Id: c.ID, Name: c.Name})
	}
	return &v1.ListNoticeCategoryOptionsReply{Items: items}, nil
}

// ListNoticeCategory is the category management list (bound to ops:notice:category), paged.
func (s *NoticeService) ListNoticeCategory(ctx context.Context, req *v1.ListNoticeCategoryRequest) (*v1.ListNoticeCategoryReply, error) {
	actor, err := actorFromContext(ctx)
	if err != nil {
		return nil, err
	}
	cs, total, err := s.uc.ListCategories(ctx, actor, int(req.GetOffset()), int(req.GetLimit()))
	if err != nil {
		return nil, err
	}
	items := make([]*v1.NoticeCategoryListItem, 0, len(cs))
	for _, c := range cs {
		items = append(items, toCategoryListItem(c))
	}
	return &v1.ListNoticeCategoryReply{Items: items, Total: total}, nil
}

func (s *NoticeService) CreateNoticeCategory(ctx context.Context, req *v1.CreateNoticeCategoryRequest) (*v1.NoticeCategoryReply, error) {
	c, err := s.uc.CreateCategory(ctx, req.GetName())
	if err != nil {
		return nil, err
	}
	return toCategoryReply(c), nil
}

func (s *NoticeService) DeleteNoticeCategory(ctx context.Context, req *v1.DeleteNoticeCategoryRequest) (*emptypb.Empty, error) {
	actor, err := actorFromContext(ctx)
	if err != nil {
		return nil, err
	}
	if err := s.uc.DeleteCategory(ctx, actor, req.GetId()); err != nil {
		return nil, err
	}
	return &emptypb.Empty{}, nil
}

func (s *NoticeService) DeleteNotice(ctx context.Context, req *v1.DeleteNoticeRequest) (*emptypb.Empty, error) {
	actor, err := actorFromContext(ctx)
	if err != nil {
		return nil, err
	}
	if err := s.uc.Delete(ctx, actor, req.GetId()); err != nil {
		return nil, err
	}
	return &emptypb.Empty{}, nil
}

// BatchDeleteNotice serves batch delete from the table's multi-select: one request, one audit entry, all or nothing.
func (s *NoticeService) BatchDeleteNotice(ctx context.Context, req *v1.BatchDeleteNoticeRequest) (*emptypb.Empty, error) {
	actor, err := actorFromContext(ctx)
	if err != nil {
		return nil, err
	}
	if err := s.uc.BatchDelete(ctx, actor, req.GetIds()); err != nil {
		return nil, err
	}
	return &emptypb.Empty{}, nil
}

func toNoticeReply(n *biz.Notice) *v1.NoticeReply {
	return &v1.NoticeReply{
		Id:          n.ID,
		Title:       n.Title,
		Content:     n.Content,
		CategoryId:  n.CategoryID,
		OwnerUserId: n.OwnerUserID,
		OwnerDeptId: n.OwnerDeptID,
		CreatedAt:   n.CreatedAt,
	}
}

// toNoticeListItem converts one list row; fillOwnerNames fills in the department and creator display names in bulk.
func toNoticeListItem(n *biz.Notice) *v1.NoticeListItem {
	return &v1.NoticeListItem{
		Id:          n.ID,
		Title:       n.Title,
		Content:     n.Content,
		CategoryId:  n.CategoryID,
		OwnerUserId: n.OwnerUserID,
		OwnerDeptId: n.OwnerDeptID,
		CreatedAt:   n.CreatedAt,
	}
}

// fillOwnerNames fills in the department and creator display names of list rows with two queries per page (one batch
// of departments, one of users), not one per row.
func (s *NoticeService) fillOwnerNames(ctx context.Context, items []*v1.NoticeListItem) error {
	deptIDs := make([]int64, 0, len(items))
	userIDs := make([]int64, 0, len(items))
	for _, it := range items {
		deptIDs = append(deptIDs, it.OwnerDeptId)
		userIDs = append(userIDs, it.OwnerUserId)
	}
	depts, err := s.names.DeptNames(ctx, deptIDs)
	if err != nil {
		return err
	}
	users, err := s.names.UserNames(ctx, userIDs)
	if err != nil {
		return err
	}
	for _, it := range items {
		it.OwnerDeptName, it.OwnerUserName = depts[it.OwnerDeptId], users[it.OwnerUserId]
	}
	return nil
}

func toCategoryReply(c *biz.NoticeCategory) *v1.NoticeCategoryReply {
	return &v1.NoticeCategoryReply{
		Id:        c.ID,
		Name:      c.Name,
		CreatedAt: c.CreatedAt,
	}
}

func toCategoryListItem(c *biz.NoticeCategory) *v1.NoticeCategoryListItem {
	return &v1.NoticeCategoryListItem{
		Id:        c.ID,
		Name:      c.Name,
		CreatedAt: c.CreatedAt,
	}
}
