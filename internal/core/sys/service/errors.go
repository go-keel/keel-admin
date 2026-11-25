package service

import (
	"context"
	"errors"

	kerrors "github.com/go-kratos/kratos/v3/errors"
	goi18n "github.com/nicksnyder/go-i18n/v2/i18n"

	v1 "github.com/go-keel/keel-admin/api/keel_admin/v1"
	"github.com/go-keel/keel-admin/internal/core/sys/biz"
	"github.com/go-keel/keel-admin/internal/core/timefmt"
	"github.com/go-keel/keel-admin/internal/i18n"
	"github.com/go-keel/keel/middleware/locale"
	"github.com/go-keel/keel/middleware/metadata"
	"github.com/go-keel/keel/middleware/sentinel"
)

// ipNotAllowed's message carries the request's client IP, so the user can ask an administrator to add that address
// to the allowlist. The IP comes from the same source the login use case checks the allowlist with
// (metadata.GetRemoteIP).
func ipNotAllowed(ctx context.Context, l *goi18n.Localizer, _ error) error {
	msg, err := l.Localize(&goi18n.LocalizeConfig{
		DefaultMessage: &goi18n.Message{ID: "ErrIPNotAllowed", Other: "This account cannot sign in from your current IP ({{.IP}}). Contact an administrator."},
		TemplateData:   map[string]string{"IP": metadata.GetRemoteIP(ctx)},
	})
	if err != nil {
		var notFound *goi18n.MessageNotFoundErr
		if !errors.As(err, &notFound) {
			panic(err)
		}
	}
	return v1.ErrorIpNotAllowed("%s", msg)
}

// internalError is the fallback for errors nothing maps: Keel's InternalWithLog (its log line and its 500 INTERNAL),
// with the message localized, because the frontend shows it to the user as is.
func internalError(ctx context.Context, err error) error {
	e := kerrors.FromError(sentinel.InternalWithLog(ctx, err))
	if l, ok := locale.FromContext(ctx); ok {
		e.Message = i18n.MustLocalizeMessage(l, &goi18n.Message{ID: "ErrUnexpectedError", Other: e.Message})
	}
	return e
}

// NewErrorSentinel maps biz sentinel errors to API errors carrying a reason enum and an i18n message. A new domain
// error needs four places updated together: the sentinel in biz/errors.go, the error_reason.proto enum, the mapping
// here, and the messages in the i18n toml files (en + zh-CN).
func NewErrorSentinel() *sentinel.Group {
	return sentinel.NewGroup().
		Default(internalError).
		OnLocaled(biz.ErrInvalidArgument, i18n.Localed("ErrInvalidArgument", "Invalid request. Please check your input.", v1.ErrorInvalidArgument)).
		// A time range with a bad format or a start not before its end: business area services return it directly too,
		// with no mapping of their own.
		OnLocaled(timefmt.ErrBadRange, i18n.Localed("ErrInvalidArgument", "Invalid request. Please check your input.", v1.ErrorInvalidArgument)).
		OnLocaled(biz.ErrAvatarTooLarge, i18n.Localed("ErrAvatarTooLarge", "Avatar image is too large.", v1.ErrorAvatarTooLarge)).
		OnLocaled(biz.ErrFavoriteLimit, i18n.Localed("ErrFavoriteLimit", "You have reached the maximum number of favorites.", v1.ErrorFavoriteLimit)).
		OnLocaled(biz.ErrInvalidCredential, i18n.Localed("ErrInvalidCredential", "Invalid username or password.", v1.ErrorInvalidCredential)).
		// The same reason as ErrInvalidCredential: to the caller both mean "credentials not valid"; login's message
		// just also mentions the one-time code.
		OnLocaled(biz.ErrLoginRejected, i18n.Localed("ErrLoginRejected", "Sign-in failed. Check your username and password; if your account uses an authenticator, also enter the code it currently shows.", v1.ErrorInvalidCredential)).
		OnLocaled(biz.ErrCaptchaInvalid, i18n.Localed("ErrCaptchaInvalid", "Invalid captcha.", v1.ErrorCaptchaInvalid)).
		OnLocaled(biz.ErrUserDisabled, i18n.Localed("ErrUserDisabled", "This account is disabled.", v1.ErrorUserDisabled)).
		OnLocaled(biz.ErrIPNotAllowed, ipNotAllowed).
		OnLocaled(biz.ErrSessionExpired, i18n.Localed("ErrSessionExpired", "Session expired. Please log in again.", v1.ErrorSessionExpired)).
		OnLocaled(biz.ErrOTPInvalid, i18n.Localed("ErrOTPInvalid", "Invalid OTP code.", v1.ErrorOtpInvalid)).
		OnLocaled(biz.ErrOTPUsed, i18n.Localed("ErrOTPUsed", "This code has no uses left. Wait for the next one.", v1.ErrorOtpAlreadyUsed)).
		OnLocaled(biz.ErrOTPNotBound, i18n.Localed("ErrOTPNotBound", "OTP is not bound yet.", v1.ErrorOtpNotBound)).
		OnLocaled(biz.ErrOTPRequired, i18n.Localed("ErrOTPRequired", "OTP binding is required.", v1.ErrorOtpRequired)).
		OnLocaled(biz.ErrOTPAlreadyBound, i18n.Localed("ErrOTPAlreadyBound", "An authenticator is already bound. Unbind it first.", v1.ErrorOtpAlreadyBound)).
		OnLocaled(biz.ErrDeptNotFound, i18n.Localed("ErrDeptNotFound", "Department not found.", v1.ErrorDeptNotFound)).
		OnLocaled(biz.ErrRoleNotFound, i18n.Localed("ErrRoleNotFound", "Role not found.", v1.ErrorRoleNotFound)).
		OnLocaled(biz.ErrAuditLogNotFound, i18n.Localed("ErrAuditLogNotFound", "Audit record not found.", v1.ErrorAuditLogNotFound)).
		OnLocaled(biz.ErrDeptNotEmpty, i18n.Localed("ErrDeptNotEmpty", "Department still has sub-departments, users, or resources.", v1.ErrorDeptNotEmpty)).
		OnLocaled(biz.ErrDuplicateRole, i18n.Localed("ErrDuplicateRole", "Role code already exists.", v1.ErrorDuplicateRole)).
		OnLocaled(biz.ErrUserExists, i18n.Localed("ErrUserExists", "Username already exists.", v1.ErrorUserExists)).
		OnLocaled(biz.ErrDeptDisabled, i18n.Localed("ErrDeptDisabled", "This department is disabled; choose another one.", v1.ErrorDeptDisabled)).
		OnLocaled(biz.ErrNotRoot, i18n.Localed("ErrNotRoot", "Root only.", v1.ErrorNotRoot)).
		OnLocaled(biz.ErrSelfManageForbidden, i18n.Localed("ErrSelfManageForbidden", "You cannot manage your own account.", v1.ErrorSelfManageForbidden)).
		OnLocaled(biz.ErrTargetProtected, i18n.Localed("ErrTargetProtected", "Target account is protected.", v1.ErrorTargetProtected)).
		OnLocaled(biz.ErrDeptOutOfRange, i18n.Localed("ErrDeptOutOfRange", "Department is out of your manage range.", v1.ErrorDeptOutOfRange)).
		OnLocaled(biz.ErrRoleOutOfRange, i18n.Localed("ErrRoleOutOfRange", "Role is out of your assignable range.", v1.ErrorRoleOutOfRange)).
		OnLocaled(biz.ErrRoleRequired, i18n.Localed("ErrRoleRequired", "Please assign at least one role.", v1.ErrorRoleRequired))
}
