package middleware

import (
	"context"
	stderrors "errors"
	"log/slog"

	"github.com/go-kratos/kratos/v3/errors"
	"github.com/go-kratos/kratos/v3/middleware"
	"github.com/go-kratos/kratos/v3/transport"
	goi18n "github.com/nicksnyder/go-i18n/v2/i18n"

	"github.com/go-keel/keel-admin/internal/core/perm"
	"github.com/go-keel/keel-admin/internal/core/sys/biz"
	"github.com/go-keel/keel-admin/internal/i18n"
	kauth "github.com/go-keel/keel/middleware/auth"
	"github.com/go-keel/keel/middleware/locale"
	"github.com/go-keel/keel/mw"
)

var (
	ErrUnauthenticated = errors.Unauthorized("UNAUTHENTICATED", "login required")
	ErrForbidden       = errors.Forbidden("FORBIDDEN", "permission denied")
	ErrNoAnnotation    = errors.Forbidden("FORBIDDEN", "operation not authorized")
	ErrStepUpOTP       = errors.Unauthorized("OTP_STEPUP", "step-up OTP required")
	// ErrOTPBindRequired: the operation requires step-up but the operator has not bound an authenticator. They
	// cannot supply any X-OTP-Code, so a plain rejection would make the operation unusable for them forever. A
	// separate reason lets the frontend open the binding guide, after which they can continue.
	ErrOTPBindRequired = errors.Unauthorized("OTP_BIND_REQUIRED", "bind an authenticator first")
	// ErrOTPUsed: the code is right but its use count has reached the limit (sys.otp.max_uses). It is separate
	// from ErrStepUpOTP so the frontend can say "wait for the next code". The reason name stays OTP_ALREADY_USED: it
	// is an external contract the frontend's errorReason.ts dispatches on, so renaming it breaks.
	ErrOTPUsed = errors.Unauthorized("OTP_ALREADY_USED", "this code has no uses left")
	// ErrOTPBad: a code was sent but does not verify. It is separate from ErrStepUpOTP because that one's text is
	// "enter the code": a user who mistyped one digit and is told to enter it only keeps checking that they did.
	// The reason is the existing OTP_INVALID in the proto (already 401), so no new contract is added.
	ErrOTPBad = errors.Unauthorized("OTP_INVALID", "invalid OTP code")
	// ErrOTPVerifyUnavailable: the verification itself failed (the replay counter is unavailable, the key cannot
	// be decrypted, the database query failed). Neither is the user's code wrong nor did they omit it. Answering
	// "enter the code" to a field just filled in only makes the user retype, the same problem ErrOTPBad avoids.
	// The fault is on the server, so it is reported as a server fault (500), still failing closed: this sensitive
	// operation is rejected all the same. It reuses the existing UNEXPECTED_ERROR instead of a new reason: the
	// frontend has only two rules for errors ("session ended: go to login", "everything else: toast"), nothing
	// would dispatch on a new reason, and it would only add one more external contract.
	ErrOTPVerifyUnavailable = errors.InternalServer("UNEXPECTED_ERROR", "one-time code verification unavailable")
	ErrNotRoot              = errors.Forbidden("NOT_ROOT", "root only")
)

// localized returns a copy of the prototype error with its message localized
// per request locale. Middleware errors do not pass through the sentinel middleware (it wraps the handler and
// sits deeper than authz), so they must be localized where they are raised; WithLocale is fixed right after metadata
// (before auth), so the localizer is already in ctx. Code and reason are kept as they are, so errors.Is is
// unchanged. A missing entry falls back to English through internal/i18n.MustLocalizeMessage (the same behaviour as
// localed in core/sys/service and the business area), instead of falling back to the English hard-coded in the
// proto, so the two English wordings need not be kept in step by hand.
func localized(ctx context.Context, proto *errors.Error, msgID string) *errors.Error {
	l, ok := locale.FromContext(ctx)
	if !ok {
		return proto
	}
	msg := i18n.MustLocalizeMessage(l, &goi18n.Message{ID: msgID, Other: proto.Message})
	return errors.New(int(proto.Code), proto.Reason, msg)
}

// StepUpVerifier verifies a fresh OTP code for sensitive (require_otp)
// operations, and reports whether the operator has an authenticator at all.
type StepUpVerifier interface {
	Verify(ctx context.Context, userID int64, code string) (bool, error)
	// Bound reports whether the user has a confirmed OTP secret. Checked
	// before demanding a code, so "not bound" and "wrong code" get different answers.
	Bound(ctx context.Context, userID int64) (bool, error)
}

const stepUpHeader = "X-OTP-Code"

// bindScopeAllowed is the operation allowlist for a restricted bind-scope token (the OTP binding bootstrap flow).
var bindScopeAllowed = map[string]bool{
	"/api.keel_admin.v1.AuthService/BindOTP":    true,
	"/api.keel_admin.v1.AuthService/ConfirmOTP": true,
}

// Authz enforces dimension-one functional permission + operation-level step-up OTP.
// Order: AllowAnonymous → principal → step-up OTP (if require_otp) → root bypass → code match.
// No annotation = deny by default.
func Authz(stepUp StepUpVerifier) mw.Middleware {
	return func(next middleware.Handler) middleware.Handler {
		return func(ctx context.Context, req any) (any, error) {
			tr, ok := transport.FromServerContext(ctx)
			if !ok {
				return next(ctx, req)
			}
			op := tr.Operation()
			if perm.IsAnonymous(op) {
				return next(ctx, req) // proto allow_anonymous: skip everything below
			}
			reqd, ok := perm.OperationRequirement(op)
			if !ok {
				return nil, localized(ctx, ErrNoAnnotation, "ErrForbidden") // deny by default
			}
			p, ok := perm.FromContext(ctx)
			if !ok {
				return nil, localized(ctx, ErrUnauthenticated, "ErrUnauthenticated")
			}
			// A restricted bind-scope token (OTP binding bootstrap) may call only the binding operations.
			if claims, has := kauth.FromContext(ctx); has {
				if scope, _ := claims["scope"].(string); scope == biz.BindScope {
					if !bindScopeAllowed[op] {
						return nil, localized(ctx, ErrForbidden, "ErrForbidden")
					}
					return next(ctx, req) // The binding endpoints themselves skip the permission-code check.
				}
			}
			// Operation-level step-up OTP applies to everyone, including root.
			// Someone without a bound authenticator cannot supply any code, so route them to the binding guide first
			// (the frontend shows the QR code) instead of answering with an OTP_STEPUP they can never satisfy. Any
			// Bound lookup error fails closed: better to block the sensitive operation than to degrade to "no
			// step-up needed".
			if reqd.RequireOTP {
				bound, err := stepUp.Bound(ctx, p.UserID)
				if err != nil {
					// Without logging err the root cause can never be found: the caller only sees "cannot verify
					// right now", and these three fail-closed branches look identical in the UI.
					slog.WarnContext(ctx, "otp bound lookup failed, failing closed",
						"operation", op, "user_id", p.UserID, "error", err)
					return nil, localized(ctx, ErrOTPVerifyUnavailable, "ErrOTPVerifyUnavailable")
				}
				if !bound {
					return nil, localized(ctx, ErrOTPBindRequired, "ErrOTPBindRequired")
				}
				code := tr.RequestHeader().Get(stepUpHeader)
				if code == "" {
					return nil, localized(ctx, ErrStepUpOTP, "ErrStepUpOTP")
				}
				valid, err := stepUp.Verify(ctx, p.UserID, code)
				if stderrors.Is(err, biz.ErrOTPUsed) {
					// When several step-up operations are done in a row, the authenticator shows the same code for
					// up to 90s, and copying it more than sys.otp.max_uses times hits the limit. Say so truthfully
					// instead of calling it a wrong code.
					return nil, localized(ctx, ErrOTPUsed, "ErrOTPUsed")
				}
				if err != nil {
					// The verification itself failed (guard unavailable, key cannot be decrypted): neither a wrong
					// code nor a missing one, so report a server fault and fail closed.
					slog.WarnContext(ctx, "otp step-up verify failed, failing closed",
						"operation", op, "user_id", p.UserID, "error", err)
					return nil, localized(ctx, ErrOTPVerifyUnavailable, "ErrOTPVerifyUnavailable")
				}
				if !valid {
					return nil, localized(ctx, ErrOTPBad, "ErrOTPInvalid")
				}
			}
			if p.IsRoot {
				return next(ctx, req) // root bypasses dimensions 1-3 (audit still applies)
			}
			if reqd.RootOnly {
				return nil, localized(ctx, ErrNotRoot, "ErrNotRoot")
			}
			if !matches(p.Perms, reqd) {
				return nil, localized(ctx, ErrForbidden, "ErrForbidden")
			}
			return next(ctx, req)
		}
	}
}

func matches(set perm.Set, r perm.Requirement) bool {
	if len(r.Codes) == 0 {
		return true
	}
	if r.AllOf {
		for _, c := range r.Codes {
			if !set.Has(c) {
				return false
			}
		}
		return true
	}
	for _, c := range r.Codes {
		if set.Has(c) {
			return true
		}
	}
	return false
}
