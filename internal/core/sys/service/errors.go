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
	"github.com/go-keel/keel/middleware/metadata"
	"github.com/go-keel/keel/middleware/sentinel"
)

// localed builds an OnLocaled handler: it looks up the localized message by messageID and passes it to the matching
// typed error_reason constructor (v1.ErrorXxx). A missing message (the current language has no translation, only the
// English source) is not a failure and falls back to English through internal/i18n.MustLocalizeMessage; a panic is
// reserved for real faults such as a template that fails to parse.
func localed(messageID, fallback string, build func(format string, args ...interface{}) *kerrors.Error) func(ctx context.Context, l *goi18n.Localizer, err error) error {
	return func(_ context.Context, l *goi18n.Localizer, _ error) error {
		msg := i18n.MustLocalizeMessage(l, &goi18n.Message{ID: messageID, Other: fallback})
		return build("%s", msg)
	}
}

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

// NewErrorSentinel maps biz sentinel errors to API errors carrying a reason enum and an i18n message. A new domain
// error needs four places updated together: the sentinel in biz/errors.go, the error_reason.proto enum, the mapping
// here, and the messages in the i18n toml files (en + zh-CN).
func NewErrorSentinel() *sentinel.Group {
	return sentinel.NewGroup().
		OnLocaled(biz.ErrInvalidArgument, localed("ErrInvalidArgument", "Invalid request. Please check your input.", v1.ErrorInvalidArgument)).
		// A time range with a bad format or a start not before its end: business area services return it directly too,
		// with no mapping of their own.
		OnLocaled(timefmt.ErrBadRange, localed("ErrInvalidArgument", "Invalid request. Please check your input.", v1.ErrorInvalidArgument)).
		OnLocaled(biz.ErrAvatarTooLarge, localed("ErrAvatarTooLarge", "Avatar image is too large.", v1.ErrorAvatarTooLarge)).
		OnLocaled(biz.ErrFavoriteLimit, localed("ErrFavoriteLimit", "You have reached the maximum number of favorites.", v1.ErrorFavoriteLimit)).
		OnLocaled(biz.ErrInvalidCredential, localed("ErrInvalidCredential", "Invalid username or password.", v1.ErrorInvalidCredential)).
		// The same reason as ErrInvalidCredential: to the caller both mean "credentials not valid"; login's message
		// just also mentions the one-time code.
		OnLocaled(biz.ErrLoginRejected, localed("ErrLoginRejected", "Sign-in failed. Check your username and password; if your account uses an authenticator, also enter the code it currently shows.", v1.ErrorInvalidCredential)).
		OnLocaled(biz.ErrCaptchaInvalid, localed("ErrCaptchaInvalid", "Invalid captcha.", v1.ErrorCaptchaInvalid)).
		OnLocaled(biz.ErrUserDisabled, localed("ErrUserDisabled", "This account is disabled.", v1.ErrorUserDisabled)).
		OnLocaled(biz.ErrIPNotAllowed, ipNotAllowed).
		OnLocaled(biz.ErrSessionExpired, localed("ErrSessionExpired", "Session expired. Please log in again.", v1.ErrorSessionExpired)).
		OnLocaled(biz.ErrOTPInvalid, localed("ErrOTPInvalid", "Invalid OTP code.", v1.ErrorOtpInvalid)).
		OnLocaled(biz.ErrOTPUsed, localed("ErrOTPUsed", "This code has no uses left. Wait for the next one.", v1.ErrorOtpAlreadyUsed)).
		OnLocaled(biz.ErrOTPNotBound, localed("ErrOTPNotBound", "OTP is not bound yet.", v1.ErrorOtpNotBound)).
		OnLocaled(biz.ErrOTPRequired, localed("ErrOTPRequired", "OTP binding is required.", v1.ErrorOtpRequired)).
		OnLocaled(biz.ErrOTPAlreadyBound, localed("ErrOTPAlreadyBound", "An authenticator is already bound. Unbind it first.", v1.ErrorOtpAlreadyBound)).
		OnLocaled(biz.ErrDeptNotFound, localed("ErrDeptNotFound", "Department not found.", v1.ErrorDeptNotFound)).
		OnLocaled(biz.ErrRoleNotFound, localed("ErrRoleNotFound", "Role not found.", v1.ErrorRoleNotFound)).
		OnLocaled(biz.ErrAuditLogNotFound, localed("ErrAuditLogNotFound", "Audit record not found.", v1.ErrorAuditLogNotFound)).
		OnLocaled(biz.ErrDeptNotEmpty, localed("ErrDeptNotEmpty", "Department still has sub-departments, users, or resources.", v1.ErrorDeptNotEmpty)).
		OnLocaled(biz.ErrDuplicateRole, localed("ErrDuplicateRole", "Role code already exists.", v1.ErrorDuplicateRole)).
		OnLocaled(biz.ErrUserExists, localed("ErrUserExists", "Username already exists.", v1.ErrorUserExists)).
		OnLocaled(biz.ErrDeptDisabled, localed("ErrDeptDisabled", "This department is disabled; choose another one.", v1.ErrorDeptDisabled)).
		OnLocaled(biz.ErrNotRoot, localed("ErrNotRoot", "Root only.", v1.ErrorNotRoot)).
		OnLocaled(biz.ErrSelfManageForbidden, localed("ErrSelfManageForbidden", "You cannot manage your own account.", v1.ErrorSelfManageForbidden)).
		OnLocaled(biz.ErrTargetProtected, localed("ErrTargetProtected", "Target account is protected.", v1.ErrorTargetProtected)).
		OnLocaled(biz.ErrDeptOutOfRange, localed("ErrDeptOutOfRange", "Department is out of your manage range.", v1.ErrorDeptOutOfRange)).
		OnLocaled(biz.ErrRoleOutOfRange, localed("ErrRoleOutOfRange", "Role is out of your assignable range.", v1.ErrorRoleOutOfRange)).
		OnLocaled(biz.ErrRoleRequired, localed("ErrRoleRequired", "Please assign at least one role.", v1.ErrorRoleRequired))
}
