package middleware

import (
	"context"
	"strconv"

	"github.com/go-kratos/kratos/v3/middleware"
	"github.com/go-kratos/kratos/v3/transport"
	"google.golang.org/protobuf/encoding/protojson"
	"google.golang.org/protobuf/proto"

	"github.com/go-keel/keel-admin/internal/core/audit"
	"github.com/go-keel/keel-admin/internal/core/perm"
	"github.com/go-keel/keel/middleware/metadata"
	"github.com/go-keel/keel/mw"
	"github.com/go-keel/keel/pkg/redact"
)

type idGetter interface{ GetId() int64 }

// auditTargeter is for requests that cannot be identified by a single integer primary key (such as a composite key
// "business + channel type + number"): the request declares target_id itself. It declares only the ID, not the type:
// target_type is derived from the operation alone, so one endpoint never shows up under two types in the log and
// filtering by type stays reliable.
type auditTargeter interface{ AuditTarget() string }

// targetIDMaxLen matches varchar(64) of sys_audit_log.target_id. Longer values are truncated for the same reason as
// auditReasonMaxLen: a failed insert would lose the whole audit entry, which is worse than an incomplete ID.
const targetIDMaxLen = 64

// auditReasonHeader lets a caller attach why an operation was done. The "why" of an audit entry cannot be inferred
// from the request and can only come from the caller. It is a header rather than a body field because it is the same
// for every audited operation, and adding it to every proto message would pollute the contract (same as X-OTP-Code).
const auditReasonHeader = "X-Audit-Reason"

// auditReasonMaxLen matches varchar(255) of sys_audit_log.reason. Longer values are truncated rather than rejected:
// audit is a side path, and a legitimate operation must not fail because an explanation is too long.
const auditReasonMaxLen = 255

// Audit records annotated (audit=true) operations after they succeed.
func Audit(rec audit.Recorder) mw.Middleware {
	return func(next middleware.Handler) middleware.Handler {
		return func(ctx context.Context, req any) (any, error) {
			tr, ok := transport.FromServerContext(ctx)
			audited := false
			if ok {
				r, found := perm.OperationRequirement(tr.Operation())
				audited = found && r.Audit
			}
			resp, err := next(ctx, req)
			if err != nil || !audited {
				return resp, err
			}

			var operatorID int64
			if p, ok := perm.FromContext(ctx); ok {
				operatorID = p.UserID
			}
			rec.Record(ctx, audit.Entry{
				OperatorID:     operatorID,
				Action:         audit.ActionOf(tr.Operation()),
				TargetType:     audit.TargetOf(tr.Operation()),
				TargetID:       auditTargetID(req),
				IP:             metadata.GetRemoteIP(ctx),
				RequestPayload: requestJSON(req),
				Reason:         auditReason(tr),
			})
			return resp, nil
		}
	}
}

// auditTargetID takes the target the request declares first, then falls back to the id field; with neither it stays
// empty (better missing than wrong).
func auditTargetID(req any) string {
	if t, ok := req.(auditTargeter); ok {
		id := t.AuditTarget()
		if len(id) > targetIDMaxLen {
			return id[:targetIDMaxLen]
		}
		return id
	}
	if g, ok := req.(idGetter); ok && g.GetId() != 0 {
		return strconv.FormatInt(g.GetId(), 10)
	}
	return ""
}

func auditReason(tr transport.Transporter) string {
	r := tr.RequestHeader().Get(auditReasonHeader)
	if len(r) > auditReasonMaxLen {
		return r[:auditReasonMaxLen]
	}
	return r
}

// requestJSON renders the request as request_payload, with fields marked
// (keel.v1.redact).sensitive masked. Redaction is driven by the proto contract: written once, it applies to every
// endpoint that uses the message, so no second list of field names is kept here.
func requestJSON(req any) *string {
	m, ok := req.(proto.Message)
	if !ok {
		return nil
	}
	raw, err := protojson.Marshal(redact.Redact(m))
	if err != nil {
		return nil
	}
	s := string(raw)
	return &s
}
