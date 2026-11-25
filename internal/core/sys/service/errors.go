package service

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"strings"

	kerrors "github.com/go-kratos/kratos/v3/errors"
	goi18n "github.com/nicksnyder/go-i18n/v2/i18n"

	v1 "github.com/go-keel/keel-admin/api/keel_admin/v1"
	"github.com/go-keel/keel-admin/internal/core/perm"
	"github.com/go-keel/keel-admin/internal/core/sys/biz"
	"github.com/go-keel/keel-admin/internal/core/timefmt"
	"github.com/go-keel/keel/middleware/locale"
	"github.com/go-keel/keel/middleware/metadata"
	"github.com/go-keel/keel/middleware/sentinel"
)

// ipNotAllowed's message carries the request's client IP, so the user can ask an administrator to add that address
// to the allowlist. The IP comes from the same source the login use case checks the allowlist with
// (metadata.GetRemoteIP).
//
// It needs template data, which locale.Localize does not take, so it uses go-i18n's localizer directly, with the
// same fallback: a missing translation is the English text, and without a localizer or on a template error the
// English text with the IP filled in.
func ipNotAllowed(ctx context.Context, _ error) error {
	ip := metadata.GetRemoteIP(ctx)
	msg := &goi18n.Message{ID: "ErrIPNotAllowed", Other: "This account cannot sign in from your current IP ({{.IP}}). Contact an administrator."}
	fallback := strings.Replace(msg.Other, "{{.IP}}", ip, 1)
	l, ok := locale.FromContext(ctx)
	if !ok {
		return v1.ErrorIpNotAllowed("%s", fallback)
	}
	s, err := l.Localize(&goi18n.LocalizeConfig{DefaultMessage: msg, TemplateData: map[string]string{"IP": ip}})
	if err != nil {
		if _, notFound := errors.AsType[*goi18n.MessageNotFoundErr](err); !notFound {
			slog.ErrorContext(ctx, "localize ErrIPNotAllowed failed, using the English text", "error", err)
			s = fallback
		}
	}
	return v1.ErrorIpNotAllowed("%s", s)
}

func unauthenticated(format string, args ...any) *kerrors.Error {
	return kerrors.Unauthorized("UNAUTHENTICATED", fmt.Sprintf(format, args...))
}

// internalError is the fallback for errors nothing maps: Keel's InternalWithLog (its log line and its 500 INTERNAL),
// with the message localized, because the frontend shows it to the user as is.
func internalError(ctx context.Context, err error) error {
	e := kerrors.FromError(sentinel.InternalWithLog(ctx, err))
	e.Message = locale.Localize(ctx, &goi18n.Message{ID: "ErrUnexpectedError", Other: e.Message})
	return e
}

// NewErrorSentinel maps biz sentinel errors to API errors carrying a reason enum and an i18n message. A new domain
// error needs four places updated together: the sentinel in biz/errors.go, the error_reason.proto enum, the mapping
// here, and the messages in the i18n toml files (en + zh-CN).
func NewErrorSentinel() *sentinel.Group {
	return sentinel.NewGroup().
		Default(internalError).
		OnMessage(biz.ErrInvalidArgument, &goi18n.Message{ID: "ErrInvalidArgument", Other: "Invalid request. Please check your input."}, v1.ErrorInvalidArgument).
		// A time range with a bad format or a start not before its end: business area services return it directly too,
		// with no mapping of their own.
		OnMessage(timefmt.ErrBadRange, &goi18n.Message{ID: "ErrInvalidArgument", Other: "Invalid request. Please check your input."}, v1.ErrorInvalidArgument).
		OnMessage(biz.ErrAvatarTooLarge, &goi18n.Message{ID: "ErrAvatarTooLarge", Other: "Avatar image is too large. Compress it or pick a smaller one."}, v1.ErrorAvatarTooLarge).
		OnMessage(biz.ErrFavoriteLimit, &goi18n.Message{ID: "ErrFavoriteLimit", Other: "You have reached the maximum number of favorites. Remove one before adding another."}, v1.ErrorFavoriteLimit).
		OnMessage(biz.ErrInvalidCredential, &goi18n.Message{ID: "ErrInvalidCredential", Other: "Invalid username or password."}, v1.ErrorInvalidCredential).
		// The same reason as ErrInvalidCredential: to the caller both mean "credentials not valid"; login's message
		// just also mentions the one-time code.
		OnMessage(biz.ErrLoginRejected, &goi18n.Message{ID: "ErrLoginRejected", Other: "Sign-in failed. Check your username and password; if your account uses an authenticator, also enter the code it currently shows."}, v1.ErrorInvalidCredential).
		OnMessage(biz.ErrCaptchaInvalid, &goi18n.Message{ID: "ErrCaptchaInvalid", Other: "Invalid captcha."}, v1.ErrorCaptchaInvalid).
		OnMessage(biz.ErrUserDisabled, &goi18n.Message{ID: "ErrUserDisabled", Other: "This account is disabled."}, v1.ErrorUserDisabled).
		On(biz.ErrIPNotAllowed, ipNotAllowed).
		// The same 401 UNAUTHENTICATED the authz middleware answers for a call without a principal.
		OnMessage(perm.ErrNoPrincipal, &goi18n.Message{ID: "ErrUnauthenticated", Other: "Please sign in first."}, unauthenticated).
		OnMessage(biz.ErrSessionExpired, &goi18n.Message{ID: "ErrSessionExpired", Other: "Session expired. Please sign in again."}, v1.ErrorSessionExpired).
		OnMessage(biz.ErrOTPInvalid, &goi18n.Message{ID: "ErrOTPInvalid", Other: "Invalid one-time code."}, v1.ErrorOtpInvalid).
		OnMessage(biz.ErrOTPUsed, &goi18n.Message{ID: "ErrOTPUsed", Other: "This code has no uses left. Wait for the next one in your authenticator."}, v1.ErrorOtpAlreadyUsed).
		OnMessage(biz.ErrOTPNotBound, &goi18n.Message{ID: "ErrOTPNotBound", Other: "No authenticator is bound yet."}, v1.ErrorOtpNotBound).
		OnMessage(biz.ErrOTPRequired, &goi18n.Message{ID: "ErrOTPRequired", Other: "An authenticator must be bound first."}, v1.ErrorOtpRequired).
		OnMessage(biz.ErrOTPAlreadyBound, &goi18n.Message{ID: "ErrOTPAlreadyBound", Other: "An authenticator is already bound. Unbind it, then bind the new one."}, v1.ErrorOtpAlreadyBound).
		OnMessage(biz.ErrDeptNotFound, &goi18n.Message{ID: "ErrDeptNotFound", Other: "Department not found."}, v1.ErrorDeptNotFound).
		OnMessage(biz.ErrRoleNotFound, &goi18n.Message{ID: "ErrRoleNotFound", Other: "Role not found."}, v1.ErrorRoleNotFound).
		OnMessage(biz.ErrAuditLogNotFound, &goi18n.Message{ID: "ErrAuditLogNotFound", Other: "Audit record not found."}, v1.ErrorAuditLogNotFound).
		OnMessage(biz.ErrDeptNotEmpty, &goi18n.Message{ID: "ErrDeptNotEmpty", Other: "This department still has sub-departments, users, or resources, so it cannot be deleted."}, v1.ErrorDeptNotEmpty).
		OnMessage(biz.ErrDuplicateRole, &goi18n.Message{ID: "ErrDuplicateRole", Other: "Role code already exists."}, v1.ErrorDuplicateRole).
		OnMessage(biz.ErrUserExists, &goi18n.Message{ID: "ErrUserExists", Other: "Username already exists."}, v1.ErrorUserExists).
		OnMessage(biz.ErrDeptDisabled, &goi18n.Message{ID: "ErrDeptDisabled", Other: "This department is disabled; choose another one."}, v1.ErrorDeptDisabled).
		OnMessage(biz.ErrNotRoot, &goi18n.Message{ID: "ErrNotRoot", Other: "Only a super administrator can do this."}, v1.ErrorNotRoot).
		OnMessage(biz.ErrSelfManageForbidden, &goi18n.Message{ID: "ErrSelfManageForbidden", Other: "You cannot manage your own account."}, v1.ErrorSelfManageForbidden).
		OnMessage(biz.ErrTargetProtected, &goi18n.Message{ID: "ErrTargetProtected", Other: "Target account is protected."}, v1.ErrorTargetProtected).
		OnMessage(biz.ErrDeptOutOfRange, &goi18n.Message{ID: "ErrDeptOutOfRange", Other: "This department is outside your management scope."}, v1.ErrorDeptOutOfRange).
		OnMessage(biz.ErrRoleOutOfRange, &goi18n.Message{ID: "ErrRoleOutOfRange", Other: "This role is outside the roles you can assign."}, v1.ErrorRoleOutOfRange).
		OnMessage(biz.ErrRoleRequired, &goi18n.Message{ID: "ErrRoleRequired", Other: "Please assign at least one role."}, v1.ErrorRoleRequired)
}
