package biz

import (
	"context"
	"time"

	"github.com/go-keel/keel-admin/internal/core/audit"
)

// AuditLog is a stored audit record.
type AuditLog struct {
	ID         int64
	OperatorID int64
	// OperatorName comes from List joining sys_user. The operator may have been deleted, so it may be empty, and
	// the interface falls back to showing the ID.
	OperatorName string
	Action       string
	TargetType   string
	TargetID     string
	IP           string
	// RequestPayload is the request body as JSON with sensitive fields already erased by the middleware; the empty
	// string means a non-proto request with no serializable payload.
	RequestPayload string
	Reason         string
	CreatedAt      string
}

// AuditFilter is the audit-log query filter (the zero value filters nothing).
//
// Each field matches an existing composite index on sys_audit_log: idx_operator(operator_id, created_at) and
// idx_action(action, created_at). The indexes were built for filtering long ago; no query used them until now.
type AuditFilter struct {
	OperatorID int64
	// OperatorKeyword fuzzily matches the operator's username or nickname (existing accounts only) and is converted to
	// operator_id before filtering.
	OperatorKeyword string
	Action          string
	TargetType      string
	TargetID        string
	IP              string
	// Start and End are the instant range [Start, End); a zero end is unbounded (ADR admin-0003).
	Start time.Time
	End   time.Time
}

// LoginFilter is the login-log query filter (the zero value filters nothing).
type LoginFilter struct {
	// UserID matches an account exactly (0 means any), to see one account's own login records. Username is a fuzzy
	// match.
	UserID   int64
	Username string
	// 0 = all, 1 = success, 2 = failure.
	Status int32
	IP     string
	Start  time.Time
	End    time.Time
}

// AuditRepo persists and queries audit records.
type AuditRepo interface {
	Record(ctx context.Context, e audit.Entry)
	// List does not read RequestPayload or Reason (the list does not show them, and the request body is large
	// text); only Get fills them.
	List(ctx context.Context, f AuditFilter, offset, limit int) ([]*AuditLog, int64, error)
	// Get reads all fields of one record and returns ErrAuditLogNotFound when it does not exist.
	Get(ctx context.Context, id int64) (*AuditLog, error)
}

// AuditUseCase wraps audit recording/query and login-log query.
type AuditUseCase struct {
	repo     AuditRepo
	loginLog LoginLogger
}

// NewAuditUseCase builds the audit use case.
func NewAuditUseCase(repo AuditRepo, loginLog LoginLogger) *AuditUseCase {
	return &AuditUseCase{repo: repo, loginLog: loginLog}
}

// ListLogin returns login-log records. A limit of 0 or less takes the default count.
func (uc *AuditUseCase) ListLogin(ctx context.Context, f LoginFilter, offset, limit int) ([]*LoginRecord, int64, error) {
	if limit <= 0 { // a negative limit also counts as "not given": gorm treats a negative Limit as unlimited
		limit = defaultPageSize
	}
	return uc.loginLog.List(ctx, f, offset, limit)
}

// Record persists an audit entry (best-effort).
func (uc *AuditUseCase) Record(ctx context.Context, e audit.Entry) { uc.repo.Record(ctx, e) }

// List returns audit records. A limit of 0 or less takes the default count.
func (uc *AuditUseCase) List(ctx context.Context, f AuditFilter, offset, limit int) ([]*AuditLog, int64, error) {
	if limit <= 0 { // a negative limit also counts as "not given": gorm treats a negative Limit as unlimited
		limit = defaultPageSize
	}
	return uc.repo.List(ctx, f, offset, limit)
}

// Get returns one audit record with its request payload.
func (uc *AuditUseCase) Get(ctx context.Context, id int64) (*AuditLog, error) {
	return uc.repo.Get(ctx, id)
}
