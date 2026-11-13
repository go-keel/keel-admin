package biz

import "errors"

var (
	ErrInvalidCredential = errors.New("invalid username or password")
	// ErrLoginRejected is every login rejection after the captcha check except the IP allowlist: an unknown,
	// disabled or locked account, a missing, wrong or used-up one-time code and a wrong password look the same
	// to the caller (see AuthUseCase.Login).
	ErrLoginRejected = errors.New("login rejected")
	ErrUserDisabled  = errors.New("user disabled")
	// ErrIPNotAllowed means the client IP is not in the account's login allowlist.
	ErrIPNotAllowed = errors.New("login from this IP is not allowed")
	ErrDeptNotFound = errors.New("dept not found")
	// ErrDeptDisabled means a disabled department was chosen as a user's department.
	ErrDeptDisabled     = errors.New("dept is disabled")
	ErrRoleNotFound     = errors.New("role not found")
	ErrAuditLogNotFound = errors.New("audit log not found")
	ErrInvalidArgument  = errors.New("invalid argument")
	// ErrAvatarTooLarge means the decoded avatar exceeds MaxAvatarBytes.
	ErrAvatarTooLarge = errors.New("avatar too large")
	// ErrSessionExpired means the token's ver claim differs from the user's current token_version, so old tokens
	// stop working at once after a password change or reset, a disable or a kick-out. biz owns this sentinel so
	// that it does not import middleware.
	ErrSessionExpired = errors.New("session expired")
)

var (
	// ErrDeptNotEmpty means only an empty department can be deleted. Child departments, users and resources (such
	// as merchants) all report this one error, without telling which kind is in the way (see
	// docs/admin/permission-model/management-boundary.md §6).
	ErrDeptNotEmpty  = errors.New("dept is not empty")
	ErrDuplicateRole = errors.New("role code already exists")
	ErrUserExists    = errors.New("username already exists")
	ErrNotRoot       = errors.New("root only")
)

var (
	ErrSelfManageForbidden = errors.New("cannot manage your own account")
	ErrTargetProtected     = errors.New("target is root or protected")
	ErrDeptOutOfRange      = errors.New("dept out of manage range")
	ErrRoleOutOfRange      = errors.New("role out of assignable range")
	// ErrRoleRequired means an account needs at least one role (when creating or editing a user).
	ErrRoleRequired = errors.New("at least one role is required")
	// ErrFavoriteLimit means the favorites are at their limit (MaxFavorites).
	ErrFavoriteLimit = errors.New("favorite limit reached")
)

var (
	ErrOTPInvalid = errors.New("invalid OTP code")
	// ErrOTPUsed means the code is correct but has reached its use limit (replay protection, see
	// sys.otp.max_uses). It is separate from ErrOTPInvalid so that the user is not lied to: the authenticator
	// shows exactly this code, so "the code is wrong" would look like a broken system; the right advice is
	// "wait for the next code".
	ErrOTPUsed     = errors.New("OTP code use limit reached")
	ErrOTPNotBound = errors.New("OTP not bound")
	ErrOTPRequired = errors.New("OTP binding required")
	// ErrOTPAlreadyBound means a bound authenticator cannot be rebound directly; it must be unbound first, which
	// needs the password. Overwriting would let one click on "rebind" kill the authenticator still in use, and
	// would let whoever hijacked the session swap in their own authenticator without the password.
	ErrOTPAlreadyBound = errors.New("OTP already bound")
)

// ErrCaptchaInvalid is the rejection at step 0 of Login (the captcha check, before the user is looked up); see
// the fail-closed comment on Login in auth.go.
var ErrCaptchaInvalid = errors.New("captcha invalid")
