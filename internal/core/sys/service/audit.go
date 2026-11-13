package service

import (
	"context"
	"sort"

	goi18n "github.com/nicksnyder/go-i18n/v2/i18n"

	v1 "github.com/go-keel/keel-admin/api/keel_admin/v1"
	"github.com/go-keel/keel-admin/internal/core/audit"
	"github.com/go-keel/keel-admin/internal/core/perm"
	"github.com/go-keel/keel-admin/internal/core/sys/biz"
	"github.com/go-keel/keel-admin/internal/core/timefmt"
	"github.com/go-keel/keel/middleware/locale"
)

// displayName looks up the audit display name in the request's language. When the request's language lacks the
// message, go-i18n returns the default language (en) text together with MessageNotFoundErr, which is still more
// readable than the raw identifier, so only an empty text counts: only when no language has the message (a business
// module forgot to register it, or a legacy action) does it fall back to the raw fallback value.
func displayName(ctx context.Context, id, fallback string) string {
	loc, ok := locale.FromContext(ctx)
	if !ok {
		return fallback
	}
	s, _ := loc.Localize(&goi18n.LocalizeConfig{MessageID: id})
	if s == "" {
		return fallback
	}
	return s
}

// AuditService implements api.keel_admin.v1.AuditService (root-only).
type AuditService struct {
	v1.UnsafeAuditServiceServer
	uc *biz.AuditUseCase
}

// NewAuditService builds the audit service handler.
func NewAuditService(uc *biz.AuditUseCase) *AuditService { return &AuditService{uc: uc} }

func (s *AuditService) ListAudit(ctx context.Context, req *v1.ListAuditRequest) (*v1.ListAuditReply, error) {
	start, end, err := timefmt.ParseInstantRange(req.GetCreatedFromAt(), req.GetCreatedToAt())
	if err != nil {
		return nil, err
	}
	f := biz.AuditFilter{
		OperatorID:      req.GetOperatorId(),
		OperatorKeyword: req.GetOperatorKeyword(),
		Action:          req.GetAction(),
		TargetType:      req.GetTargetType(),
		TargetID:        req.GetTargetId(),
		IP:              req.GetIp(),
		Start:           start,
		End:             end,
	}
	logs, total, err := s.uc.List(ctx, f, int(req.GetOffset()), int(req.GetLimit()))
	if err != nil {
		return nil, err
	}
	items := make([]*v1.AuditLogListItem, 0, len(logs))
	for _, l := range logs {
		items = append(items, &v1.AuditLogListItem{
			Id:           l.ID,
			OperatorId:   l.OperatorID,
			OperatorName: l.OperatorName,
			Action:       l.Action,
			TargetType:   l.TargetType,
			TargetId:     l.TargetID,
			Ip:           l.IP,
			CreatedAt:    l.CreatedAt,
			ActionName:   displayName(ctx, audit.ActionNameKey(l.Action), l.Action),
			TargetName:   displayName(ctx, audit.TargetNameKey(l.TargetType), l.TargetType),
		})
	}
	return &v1.ListAuditReply{Items: items, Total: total}, nil
}

func (s *AuditService) GetAudit(ctx context.Context, req *v1.GetAuditRequest) (*v1.AuditLogInfo, error) {
	l, err := s.uc.Get(ctx, req.GetId())
	if err != nil {
		return nil, err
	}
	return &v1.AuditLogInfo{
		Id:             l.ID,
		OperatorId:     l.OperatorID,
		OperatorName:   l.OperatorName,
		Action:         l.Action,
		TargetType:     l.TargetType,
		TargetId:       l.TargetID,
		Ip:             l.IP,
		CreatedAt:      l.CreatedAt,
		ActionName:     displayName(ctx, audit.ActionNameKey(l.Action), l.Action),
		TargetName:     displayName(ctx, audit.TargetNameKey(l.TargetType), l.TargetType),
		RequestPayload: l.RequestPayload,
		Reason:         l.Reason,
	}, nil
}

// ListAuditActions reads the live registry (perm.Audited), not the historical data in the database: the dropdown
// lists only actions the system currently supports and audits, grouped by target type. Historical actions (whose
// endpoints are gone) are not offered as filters, and old records still display through the historical message
// fallback.
func (s *AuditService) ListAuditActions(ctx context.Context, _ *v1.ListAuditActionsRequest) (*v1.ListAuditActionsReply, error) {
	byTarget := map[string][]*v1.AuditActionOption{}
	for _, op := range perm.Audited() {
		action := audit.ActionOf(op)
		target := audit.TargetOf(op)
		byTarget[target] = append(byTarget[target], &v1.AuditActionOption{
			Action: action,
			Name:   displayName(ctx, audit.ActionNameKey(action), action),
		})
	}
	groups := make([]*v1.AuditActionGroup, 0, len(byTarget))
	for target, opts := range byTarget {
		sort.Slice(opts, func(i, j int) bool { return opts[i].Action < opts[j].Action })
		groups = append(groups, &v1.AuditActionGroup{
			Target:  target,
			Name:    displayName(ctx, audit.TargetNameKey(target), target),
			Actions: opts,
		})
	}
	// Registry iteration is unordered; sorting keeps the dropdown stable.
	sort.Slice(groups, func(i, j int) bool { return groups[i].Target < groups[j].Target })
	return &v1.ListAuditActionsReply{Groups: groups}, nil
}

func (s *AuditService) ListLogin(ctx context.Context, req *v1.ListLoginRequest) (*v1.ListLoginReply, error) {
	start, end, err := timefmt.ParseInstantRange(req.GetCreatedFromAt(), req.GetCreatedToAt())
	if err != nil {
		return nil, err
	}
	f := biz.LoginFilter{
		UserID:   req.GetUserId(),
		Username: req.GetUsername(),
		Status:   req.GetStatus(),
		IP:       req.GetIp(),
		Start:    start,
		End:      end,
	}
	logs, total, err := s.uc.ListLogin(ctx, f, int(req.GetOffset()), int(req.GetLimit()))
	if err != nil {
		return nil, err
	}
	items := make([]*v1.LoginLogListItem, 0, len(logs))
	for _, l := range logs {
		items = append(items, &v1.LoginLogListItem{
			Id: l.ID, UserId: l.UserID, Username: l.Username,
			Success: l.Success, Ip: l.IP, Msg: l.Msg, CreatedAt: l.CreatedAt,
		})
	}
	return &v1.ListLoginReply{Items: items, Total: total}, nil
}
