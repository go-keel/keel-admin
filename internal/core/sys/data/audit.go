package data

import (
	"context"
	"errors"
	"log/slog"

	"gorm.io/gorm"

	"github.com/go-keel/keel-admin/internal/core/audit"
	coredata "github.com/go-keel/keel-admin/internal/core/data"
	"github.com/go-keel/keel-admin/internal/core/sys/biz"
)

var _ biz.AuditRepo = (*auditRepo)(nil)

type auditRepo struct {
	data *coredata.Data
}

// NewAuditRepo builds the audit repository.
func NewAuditRepo(data *coredata.Data) biz.AuditRepo { return &auditRepo{data: data} }

func (r *auditRepo) Record(ctx context.Context, e audit.Entry) {
	m := sysAuditLog{
		OperatorID:     e.OperatorID,
		Action:         e.Action,
		TargetType:     e.TargetType,
		TargetID:       e.TargetID,
		IP:             e.IP,
		RequestPayload: e.RequestPayload,
		Reason:         e.Reason,
	}
	if err := r.data.DB.WithContext(ctx).Create(&m).Error; err != nil {
		slog.ErrorContext(ctx, "write audit log failed", "error", err)
	}
}

// applyAuditFilter applies the filters. operator_id and action each have a composite index ending in created_at
// (idx_operator / idx_action), which the descending-id paging below can use.
func applyAuditFilter(q *gorm.DB, f biz.AuditFilter) *gorm.DB {
	if f.OperatorID > 0 {
		q = q.Where("operator_id = ?", f.OperatorID)
	}
	if f.OperatorKeyword != "" {
		// The subquery wraps the whole "username or nickname" OR, so it cannot collapse with the other filters into
		// a AND b OR c. It uses sysUser's default scope: soft-deleted accounts do not take part in the match, and
		// the list shows only their ID anyway.
		like := containsPattern(f.OperatorKeyword)
		q = q.Where("operator_id IN (?)", q.Session(&gorm.Session{NewDB: true}).
			Model(&sysUser{}).Select("id").
			Where("username LIKE ? OR nickname LIKE ?", like, like))
	}
	if f.Action != "" {
		q = q.Where("action = ?", f.Action)
	}
	if f.TargetType != "" {
		q = q.Where("target_type = ?", f.TargetType)
	}
	if f.TargetID != "" {
		q = q.Where("target_id = ?", f.TargetID)
	}
	if f.IP != "" {
		q = q.Where("ip = ?", f.IP)
	}
	if !f.Start.IsZero() {
		q = q.Where("created_at >= ?", f.Start)
	}
	if !f.End.IsZero() {
		q = q.Where("created_at < ?", f.End)
	}
	return q
}

func (r *auditRepo) List(ctx context.Context, f biz.AuditFilter, offset, limit int) ([]*biz.AuditLog, int64, error) {
	q := applyAuditFilter(r.data.DB.WithContext(ctx).Model(&sysAuditLog{}), f)
	var total int64
	if err := q.Count(&total).Error; err != nil {
		return nil, 0, err
	}
	var ms []sysAuditLog
	if err := q.Omit("request_payload", "reason").Order("id desc").Offset(offset).Limit(limit).Find(&ms).Error; err != nil {
		return nil, 0, err
	}

	// Operator usernames come from one batched IN query, with no N+1 and no JOIN: the operator may have been deleted
	// (audit logs outlive accounts), and the nulls of a LEFT JOIN would still need handling here. Missing is left
	// missing, and the UI falls back to showing the ID.
	names, err := r.operatorNames(ctx, ms)
	if err != nil {
		return nil, 0, err
	}

	out := make([]*biz.AuditLog, 0, len(ms))
	for i := range ms {
		l := ms[i].toBiz()
		l.OperatorName = names[ms[i].OperatorID]
		out = append(out, l)
	}
	return out, total, nil
}

func (r *auditRepo) Get(ctx context.Context, id int64) (*biz.AuditLog, error) {
	var m sysAuditLog
	if err := r.data.DB.WithContext(ctx).Where("id = ?", id).First(&m).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, biz.ErrAuditLogNotFound
		}
		return nil, err
	}
	names, err := r.operatorNames(ctx, []sysAuditLog{m})
	if err != nil {
		return nil, err
	}
	l := m.toBiz()
	l.OperatorName = names[m.OperatorID]
	return l, nil
}

func (r *auditRepo) operatorNames(ctx context.Context, ms []sysAuditLog) (map[int64]string, error) {
	out := map[int64]string{}
	if len(ms) == 0 {
		return out, nil
	}
	set := map[int64]struct{}{}
	for i := range ms {
		if ms[i].OperatorID > 0 {
			set[ms[i].OperatorID] = struct{}{}
		}
	}
	if len(set) == 0 {
		return out, nil
	}
	ids := make([]int64, 0, len(set))
	for id := range set {
		ids = append(ids, id)
	}
	var users []sysUser
	if err := r.data.DB.WithContext(ctx).Model(&sysUser{}).
		Select("id", "username").Where("id IN ?", ids).Find(&users).Error; err != nil {
		return nil, err
	}
	for _, u := range users {
		out[u.ID] = u.Username
	}
	return out, nil
}

func deref(s *string) string {
	if s == nil {
		return ""
	}
	return *s
}
