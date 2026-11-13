// Package audit holds the audit primitives shared by the audit middleware and
// the biz layer: the entry type and the recorder interface.
//
// It is a package of its own because both middleware and core/sys/biz need these things, and biz cannot import
// middleware (that would invert the dependency direction). Defining an entry on each side with a field-by-field
// adapter in between would not decouple anything; it would move the coupling to a place nobody looks at: add a field
// on one side, forget to copy it, and both sides still compile, both sides' unit tests still pass, and the field is
// silently dropped.
//
// # Request bodies only, no data snapshots
//
// An audit record holds "who, when, on what, sent which request", without snapshots of the data before and after
// the change.
//
// Only business code knows the shape of a snapshot: the framework can neither redact it (it gets an any without a
// schema) nor guarantee it is complete (forget to copy a field and both sides still compile). That leaves intruding
// into every UseCase or leaving a leak the framework cannot control, and neither is worth it, so the audit boundary
// matches the framework's capability boundary: the request body is the only thing it can obtain automatically,
// completely and safely, so that is all it records.
//
// The cost is real: the audit of a delete keeps only target_id and cannot tell which codes the deleted role had.
//
// ponytail: no data snapshots, so the audit of deletes is incomplete. Introduce a ctx sink (WithSink/SetBefore) the
// day it is really needed, and make redaction explicitly the caller's job: whoever has the schema is responsible.
package audit

import (
	"context"
	"strings"
)

// Entry is one audit record, from the middleware that assembles it to the
// repo that persists it.
//
// RequestPayload is a pointer: nil → SQL NULL, meaning the call had no request body (a non-proto request).
type Entry struct {
	OperatorID     int64
	Action         string
	TargetType     string
	TargetID       string
	IP             string
	RequestPayload *string
	Reason         string
}

// Recorder persists audit entries (implemented by biz.AuditUseCase).
type Recorder interface {
	Record(ctx context.Context, e Entry)
}

// ActionOf turns "/api.keel_admin.v1.RoleService/CreateRole" into "RoleService.CreateRole"
// — the stable action identifier stored in sys_audit_log. The writing side (middleware) and the display side
// (service, the message completeness test) share this one derivation; neither implements its own.
func ActionOf(op string) string {
	parts := strings.Split(strings.TrimPrefix(op, "/"), "/")
	if len(parts) != 2 {
		return op
	}
	return trimPackage(parts[0]) + "." + parts[1]
}

// TargetOf turns "/api.keel_admin.v1.RoleService/CreateRole" into "Role".
func TargetOf(op string) string {
	parts := strings.Split(strings.TrimPrefix(op, "/"), "/")
	if len(parts) == 0 {
		return ""
	}
	return strings.TrimSuffix(trimPackage(parts[0]), "Service")
}

func trimPackage(svc string) string {
	if i := strings.LastIndex(svc, "."); i >= 0 {
		return svc[i+1:]
	}
	return svc
}

// ActionNameKey / TargetNameKey are the i18n message ids for the localized
// display names, resolved at read time (ListAudit / ListAuditActions) — the
// same pattern as perm.ResourceDef.NameKey. The database stores only stable identifiers, never display text; the
// messages are registered in the internal/i18n toml files, and their completeness is locked by the registration
// test under the entry cmd.
func ActionNameKey(action string) string { return "audit.act." + action }
func TargetNameKey(target string) string { return "audit.tt." + target }
