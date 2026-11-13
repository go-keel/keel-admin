package biz

import (
	"context"
	"errors"
	"log/slog"
	"strconv"
	"time"

	"github.com/go-keel/keel-admin/internal/core/captcha"
	"github.com/go-keel/keel-admin/internal/core/perm"
	"github.com/go-keel/keel/pkg/jwt"
	"github.com/go-keel/keel/pkg/pwd"
)

// User is the admin account domain entity.
type User struct {
	ID           int64
	Username     string
	Password     string // hash
	Nickname     string
	Email        string // contact detail, empty = not given; not unique and not used to log in
	Phone        string
	DeptID       int64
	IsRoot       bool
	Avatar       string       // avatar data URI, empty = not set
	IPWhitelist  string       // login client IP allowlist (comma-separated IPs/CIDRs), empty = no restriction
	OTPBound     bool         // whether an authenticator is bound
	OTPBoundAt   string       // instant the authenticator was bound, UTC RFC3339; empty when not bound
	Status       int8         // 1 enabled, 0 disabled
	TokenVersion int          // a bump makes the ver claim of earlier tokens mismatch at once, ending the sessions
	Settings     UserSettings // personal settings that follow the account (favorite menus etc.), see settings.go
	CreatedAt    string       // UTC RFC3339, formatted by the data layer with timefmt
}

// LoginEntry records a login attempt.
type LoginEntry struct {
	UserID   int64
	Username string
	Success  bool
	IP       string
	Msg      string
}

// AccountChange is a security-related change to an account (UserRepo.UpdateAccount); a nil pointer field is not
// changed. The use case decides whether to revoke sessions; storage only writes what it is given.
type AccountChange struct {
	PasswordHash   *string
	Status         *int8
	IsRoot         *bool
	RevokeSessions bool
}

// UserRepo is the account persistence port.
type UserRepo interface {
	FindByUsername(ctx context.Context, username string) (*User, error)
	FindByID(ctx context.Context, id int64) (*User, error)
	RolePermCodesByRole(ctx context.Context, roleID int64) ([]string, error)
	RoleIDs(ctx context.Context, userID int64) ([]int64, error)
	// UpdateAccount writes an account's security-related change in one statement: unset fields are not changed,
	// and when RevokeSessions is true the same statement increments token_version so all of the user's earlier
	// tokens stop working at once. Written in two steps, a failing second step would leave the password changed
	// and the old sessions alive. Nothing set means no statement.
	UpdateAccount(ctx context.Context, id int64, c AccountChange) error
	// SetProfile updates the profile fields users may change themselves (nickname, avatar, email, phone).
	SetProfile(ctx context.Context, id int64, p ProfileEdit) error
	// UpdateSettings locks the user row in a transaction and reads, modifies and writes: it passes the current
	// settings to mutate, writes back what mutate changed and returns the result. Holding the lock across the
	// whole read-modify-write is why this port exists: when two browsers each add a favorite at once, the later
	// one reads the list the earlier one wrote, and both favorites stay.
	UpdateSettings(ctx context.Context, id int64, mutate func(*UserSettings) error) (*UserSettings, error)
}

// LoginRecord is a stored login attempt.
type LoginRecord struct {
	ID        int64
	UserID    int64
	Username  string
	Success   bool
	IP        string
	Msg       string
	CreatedAt string
}

// LoginLogger persists and queries login attempts.
type LoginLogger interface {
	Log(ctx context.Context, e LoginEntry)
	List(ctx context.Context, f LoginFilter, offset, limit int) ([]*LoginRecord, int64, error)
}

// PermCache caches a role's permission codes.
type PermCache interface {
	GetRoleCodes(ctx context.Context, roleID int64) ([]string, bool)
	SetRoleCodes(ctx context.Context, roleID int64, codes []string)
	InvalidateRole(ctx context.Context, roleID int64)
}

// AuthConfig is the startup-config (app.yaml sys.*) subset the auth use case
// needs: the login brute-force thresholds and the captcha display info. Read
// once at boot; changes require a restart.
type AuthConfig struct {
	CaptchaEnabled  bool
	CaptchaProvider string
	// CaptchaSiteKey is the public key of a widget-style provider, sent with login-meta.
	CaptchaSiteKey     string
	LockoutEnabled     bool
	LockoutMaxFailures int64
	LockoutLockMinutes int64
	LockoutScope       string
}

// AuthUseCase handles login and principal resolution.
type AuthUseCase struct {
	users      UserRepo
	loginLog   LoginLogger
	cache      PermCache
	otp        OTPGate
	signer     jwt.Signer
	sessionTTL time.Duration
	lockout    LockoutGuard
	cfg        AuthConfig
	captcha    CaptchaGate
}

// NewAuthUseCase builds the auth use case. lockout + cfg drive the login
// brute-force guard (sys.lockout.*) — see Login. captchaGate drives Login's
// step 0 admission gate (sys.captcha.*).
func NewAuthUseCase(users UserRepo, loginLog LoginLogger, cache PermCache, otp OTPGate, signer jwt.Signer, sessionTTL time.Duration, lockout LockoutGuard, cfg AuthConfig, captchaGate CaptchaGate) *AuthUseCase {
	return &AuthUseCase{users: users, loginLog: loginLog, cache: cache, otp: otp, signer: signer, sessionTTL: sessionTTL, lockout: lockout, cfg: cfg, captcha: captchaGate}
}

// passwordVerify is pwd.VerifyPassword, kept behind a package-level
// indirection so tests can substitute a call-counting fake without widening
// NewAuthUseCase's already-large parameter list (9 fields). Swapping this
// var is not goroutine-safe, but this package's tests never run with
// t.Parallel(), so serial test execution keeps the swap safe — see
// auth_timing_test.go's swapCountingPasswordVerify.
var passwordVerify = pwd.VerifyPassword

// dummyHash is a pre-computed Argon2id hash of a fixed decoy password, used
// to run a throwaway verify on every Login failure branch that would
// otherwise return before ever calling passwordVerify. Without this,
// "unknown username", "account disabled" and "account locked" all return
// far faster than "known username, wrong password" (which pays the real
// Argon2id cost) — a timing side channel that lets an attacker enumerate
// valid usernames or detect lockouts purely from response latency. Computed
// once at package init (not per-request) so it costs nothing at request
// time; Argon2id's per-call cost is dominated by its fixed work factor, not
// by which hash it's comparing against, so verifying against this constant
// hash is a faithful stand-in for verifying against a real user's hash.
var dummyHash = mustHashDummy()

func mustHashDummy() string {
	h, err := pwd.HashPassword("constant-time-login-guard")
	if err != nil {
		panic("biz: failed to precompute dummy password hash: " + err.Error())
	}
	return h
}

// BindScope is the scope claim of a restricted token: it may call only the OTP binding endpoints (gated by the
// authz middleware).
const BindScope = "otp_bind"

// bindTokenTTL is the lifetime of the OTP binding bootstrap token.
const bindTokenTTL = 10 * time.Minute

// lockoutSubject decides what the brute-force counter is keyed by.
//
// The default is username_ip. Keyed by username alone, anyone who knows a username (root is a ready one) could
// lock the account with a burst of wrong passwords, shutting its owner out too: the brute-force defence would
// itself become a denial-of-service tool. Counting per username plus IP confines the lock to the attacker's IP,
// and the owner logs in from elsewhere as usual. The cost is that an attacker resets the count by changing IP,
// so this is a rate limit, not a hard boundary; OTP and captcha are what really stop brute force.
//
// !! Deployment precondition: ip comes from metadata.GetRemoteIP, which trusts only the connection peer and the
// X-Forwarded-For written by listed proxies; on a direct connection it takes the connection address, which cannot
// be forged. The nginx in front of the program must overwrite XFF as docs/guides/deploy-nginx.md says; if it does
// not, an attacker gets a new counter bucket by sending a new XFF with each request, and this gate is useless. The
// login IP allowlist (see UserRepo.IPWhitelist) rests on the same precondition; see AGENTS.md.
//
// username (optional) locks the account globally: stronger against brute force, but it brings back the DoS above.
// Choose it only when the deployer explicitly accepts that risk, or when the service faces the internet directly.
func (uc *AuthUseCase) lockoutSubject(username, ip string) string {
	if uc.cfg.LockoutScope == LockoutScopeUsername {
		return username
	}
	return username + "|" + ip
}

// CompleteOTPBootstrap issues the access token directly at the last step of the binding bootstrap flow.
//
// Why it may issue directly: bindToken is issued only after the password check passed (and captcha, lockout and
// the IP allowlist too), and the caller has just proven in ConfirmOTP that it holds the authenticator. Both
// factors are present, which equals a complete login. Making the user log in again would be redundant and would
// cost another code: Confirm already spent one of the code's uses, and once they are spent the user has to wait
// for the next time window.
//
// It still re-runs admission: bindToken lives for 10 minutes, during which the account may have been disabled or
// its IP allowlist changed. The user has to be loaded for token_version anyway, so these steps cost almost nothing.
func (uc *AuthUseCase) CompleteOTPBootstrap(ctx context.Context, userID int64, ip string) (string, error) {
	u, err := uc.users.FindByID(ctx, userID)
	if err != nil {
		return "", err
	}
	if u.Status != 1 {
		return "", ErrUserDisabled
	}
	if !ipAllowed(u.IPWhitelist, ip) {
		return "", ErrIPNotAllowed
	}
	c := jwt.NewClaims(strconv.FormatInt(u.ID, 10), uc.sessionTTL)
	c["ver"] = float64(u.TokenVersion)
	token, err := uc.signer.Sign(c)
	if err != nil {
		return "", err
	}
	uc.loginLog.Log(ctx, LoginEntry{UserID: u.ID, Username: u.Username, Success: true, IP: ip, Msg: "otp bootstrap"})
	return token, nil
}

// Login verifies credentials and returns a signed JWT.
// Check order: captcha (when enabled) → the account exists and may log in → login IP allowlist → one-time code
// (when bound) → password. After the captcha, every rejection except the IP allowlist is ErrLoginRejected.
// When OTP is forced but not bound it returns (token="", bindToken!="", u, nil): the caller guides the binding and
// the user logs in again afterwards, which resolves the deadlock "a user forced to use OTP cannot bootstrap a
// binding".
//
// Step 0 (before FindByUsername): captcha admission gate. Captcha targets
// the request, not the account — it must block scripted/credential-stuffing
// traffic before any username lookup happens, so it runs first, ahead of
// even the brute-force guard.
//
// Fail-closed semantics, and why this differs from LockoutGuard's fail-OPEN
// (see LockoutGuard doc): lockout is a slow-down, not a hard boundary — a
// Redis outage there must never lock out legitimate users. Captcha is the
// opposite: it is only consulted when an operator has explicitly turned it
// on (sys.captcha.enabled), which is itself a decision to reject on any
// doubt. So here every non-true Verify outcome — wrong answer, an
// unregistered/misconfigured provider (see captchaGate.Verify), or an
// underlying backend error — collapses to ErrCaptchaInvalid and the login
// is rejected. This also covers the base64 provider's Redis-backed Verify,
// which by construction cannot distinguish "Redis is down" from "wrong
// answer" (it always returns a nil error — see captcha/base64 doc): that
// Redis instance is shared with sessions and the lockout guard, so if it is
// down the system is already degraded, and rejecting login here is an
// acceptable, safe default.
//
// A captcha rejection is never recorded as a lockout failure: it happens
// before FindByUsername, so there is no username-scoped brute-force signal
// yet, and captcha failures are typically bot noise, not targeted
// credential guessing.
func (uc *AuthUseCase) Login(ctx context.Context, username, password, otpCode, captchaID, captchaAnswer, ip string) (token string, bindToken string, user *User, err error) {
	if uc.captcha.Enabled(ctx) {
		ok, verr := uc.captcha.Verify(ctx, captchaID, captchaAnswer, ip)
		if verr != nil || !ok {
			return "", "", nil, ErrCaptchaInvalid
		}
	}

	// After the captcha, every rejection except the IP allowlist is ErrLoginRejected: the allowlist and the
	// one-time code are checked before the password, and distinct errors would tell someone without the password
	// "the account exists", "it is disabled", "it has an authenticator" or "the code is right". The timing must
	// match too: every branch that rejects before the real password check runs a dummy Argon2id check (see
	// dummyHash), or the response time would leak the same information.
	reject := func(e LoginEntry) error {
		_, _ = passwordVerify(password, dummyHash)
		e.Username, e.IP = username, ip
		uc.loginLog.Log(ctx, e)
		return ErrLoginRejected
	}
	lockoutEnabled := uc.cfg.LockoutEnabled
	recordFailure := func(u *User) {
		if !lockoutEnabled {
			return
		}
		window := time.Duration(uc.cfg.LockoutLockMinutes) * time.Minute
		if rerr := uc.lockout.RecordFailure(ctx, uc.lockoutSubject(u.Username, ip), window); rerr != nil {
			slog.WarnContext(ctx, "lockout guard RecordFailure failed, failing open", "username", username, "error", rerr)
		}
	}

	// Step 1: the account exists and may log in (not disabled, not locked after consecutive failures).
	u, err := uc.users.FindByUsername(ctx, username)
	if err != nil {
		return "", "", nil, reject(LoginEntry{Msg: "user not found"})
	}
	if u.Status != 1 {
		return "", "", nil, reject(LoginEntry{UserID: u.ID, Msg: "disabled"})
	}
	// Brute-force lockout is checked before any factor, so a locked account never reaches the code or the
	// password. An error from Failures lets the login through (see LockoutGuard): a Redis outage must not lock
	// everyone out.
	if lockoutEnabled {
		failures, ferr := uc.lockout.Failures(ctx, uc.lockoutSubject(u.Username, ip))
		if ferr != nil {
			slog.WarnContext(ctx, "lockout guard Failures failed, failing open", "username", username, "error", ferr)
		} else if failures >= uc.cfg.LockoutMaxFailures {
			return "", "", nil, reject(LoginEntry{UserID: u.ID, Msg: "locked"})
		}
	}

	// Step 2: the login IP allowlist. It is the only rejection with an explicit error: it says plainly that the IP
	// is the problem (the message includes the current IP), so the owner knows which address to ask an
	// administrator to add. It comes before the one-time code so that requests from the wrong source do not spend
	// the code's uses. An empty allowlist passes; a non-empty one denies when the client IP is unknown.
	//
	// !! Same deployment precondition as lockoutSubject: this gate holds only when nginx overwrites XFF as
	// docs/guides/deploy-nginx.md says; otherwise the client's own XFF is trusted and bypasses it; see AGENTS.md.
	if !ipAllowed(u.IPWhitelist, ip) {
		uc.loginLog.Log(ctx, LoginEntry{UserID: u.ID, Username: username, Success: false, IP: ip, Msg: "ip not allowed"})
		return "", "", nil, ErrIPNotAllowed
	}

	// Step 3: the one-time code, with two orthogonal dimensions:
	//   forced = root (system-wide) || any force_otp role: whether OTP is mandatory
	//   bound  = an authenticator is bound: whether this login checks OTP
	// root is forced system-wide rather than by role: otp.Required looks only at force_otp roles, and root gets
	// its permissions by bypass and usually holds no role, so it would slip past this gate, leaving the most
	// powerful account unchecked.
	// Fail closed: a query error from Required, Bound or Verify rejects the login; a database failure never
	// degrades into "no OTP needed".
	// Break glass (root lost its authenticator, and self-service ResetOTP needs a step-up OTP, a deadlock):
	// operations clear the account's otp_secret in the database, and the next login binds again.
	roleForced, err := uc.otp.Required(ctx, u.ID)
	if err != nil {
		return "", "", nil, err
	}
	forced := u.IsRoot || roleForced
	bound, err := uc.otp.Bound(ctx, u.ID)
	if err != nil {
		return "", "", nil, err
	}
	// Bound means checked, whether or not a role forces it: the profile page offers self-service binding to
	// everyone, and checking only when forced would make login ignore a self-bound account's OTP entirely. A forced
	// but unbound account has no code to give, so it skips this step and gets the binding guide after the password.
	if bound {
		if otpCode == "" {
			return "", "", nil, reject(LoginEntry{UserID: u.ID, Msg: "otp missing"})
		}
		valid, err := uc.otp.Verify(ctx, u.ID, otpCode)
		if errors.Is(err, ErrOTPUsed) {
			// The code is right but its uses are spent: it does not count towards lockout, but must not be
			// revealed either.
			return "", "", nil, reject(LoginEntry{UserID: u.ID, Msg: "otp used up"})
		}
		if err != nil {
			return "", "", nil, err
		}
		if !valid {
			// A wrong OTP counts towards lockout like a wrong password: a 6-digit code has only 10^6 values, and
			// without counting one could retry codes endlessly.
			recordFailure(u)
			return "", "", nil, reject(LoginEntry{UserID: u.ID, Msg: "bad otp"})
		}
	}

	// Step 4: the password.
	if ok, _ := passwordVerify(password, u.Password); !ok {
		recordFailure(u)
		uc.loginLog.Log(ctx, LoginEntry{UserID: u.ID, Username: username, Success: false, IP: ip, Msg: "bad password"})
		return "", "", nil, ErrLoginRejected
	}

	// Forced but not bound → bootstrap guide: issue a short-lived token restricted to OTP binding (scope allowlist
	// in middleware/authz.go). The user logs in again after binding.
	if forced && !bound {
		c := jwt.NewClaims(strconv.FormatInt(u.ID, 10), bindTokenTTL)
		c["scope"] = BindScope
		c["ver"] = float64(u.TokenVersion)
		bt, err := uc.signer.Sign(c)
		if err != nil {
			return "", "", nil, err
		}
		return "", bt, u, nil
	}

	// Full login success (past OTP, before issuing the final token): reset
	// the brute-force counter. Same fail-open as above — a Clear error must
	// not block an otherwise-successful login.
	if lockoutEnabled {
		if cerr := uc.lockout.Clear(ctx, uc.lockoutSubject(u.Username, ip)); cerr != nil {
			slog.WarnContext(ctx, "lockout guard Clear failed, failing open", "username", username, "error", cerr)
		}
	}
	c := jwt.NewClaims(strconv.FormatInt(u.ID, 10), uc.sessionTTL)
	c["ver"] = float64(u.TokenVersion)
	token, err = uc.signer.Sign(c)
	if err != nil {
		return "", "", nil, err
	}
	uc.loginLog.Log(ctx, LoginEntry{UserID: u.ID, Username: username, Success: true, IP: ip})
	return token, "", u, nil
}

// ProfileEdit is the profile users may change themselves, written back as a whole in one submission. Email and
// phone formats are checked at the contract layer (buf.validate rules in the proto).
type ProfileEdit struct {
	Nickname string
	Avatar   string
	Email    string
	Phone    string
}

// UpdateMyProfile changes the caller's own nickname, avatar, email and phone. Avatar size and format are always
// checked on the server: the frontend's compression and format limits are only convenience, not a boundary.
func (uc *AuthUseCase) UpdateMyProfile(ctx context.Context, userID int64, p ProfileEdit) error {
	if p.Nickname == "" {
		return ErrInvalidArgument
	}
	if err := validateAvatar(p.Avatar); err != nil {
		return err
	}
	return uc.users.SetProfile(ctx, userID, p)
}

// RecentLogins returns the caller's last n login attempts (successes and failures), newest first. The profile page
// uses it so users can spot "that was not me" or "someone is trying my password". Users see their own records
// without the login log view permission.
func (uc *AuthUseCase) RecentLogins(ctx context.Context, userID int64, n int) ([]*LoginRecord, error) {
	logs, _, err := uc.loginLog.List(ctx, LoginFilter{UserID: userID}, 0, n)
	return logs, err
}

// VerifyPassword checks the caller's own password, to reconfirm before a destructive self-service action (such as
// unbinding the authenticator).
//
// Why unbinding checks the password again: it disables a factor in effect. Allowing it on the session alone would
// let whoever hijacked the session swap in their own authenticator and take over; requiring a step-up OTP would
// leave someone who lost the authenticator unable to help themselves. The password sits right in between.
func (uc *AuthUseCase) VerifyPassword(ctx context.Context, userID int64, password string) error {
	u, err := uc.users.FindByID(ctx, userID)
	if err != nil {
		return ErrInvalidCredential
	}
	if ok, _ := pwd.VerifyPassword(password, u.Password); !ok {
		return ErrInvalidCredential
	}
	return nil
}

// ChangeMyPassword changes the caller's own password: it checks the old password, then stores the new hash.
func (uc *AuthUseCase) ChangeMyPassword(ctx context.Context, userID int64, oldPassword, newPassword string) error {
	u, err := uc.users.FindByID(ctx, userID)
	if err != nil {
		return ErrInvalidCredential
	}
	if ok, _ := pwd.VerifyPassword(oldPassword, u.Password); !ok {
		return ErrInvalidCredential
	}
	hash, err := pwd.HashPassword(newPassword)
	if err != nil {
		return err
	}
	// Old tokens stop working at once after a password change: the same statement as the new password revokes the
	// sessions.
	return uc.users.UpdateAccount(ctx, userID, AccountChange{PasswordHash: &hash, RevokeSessions: true})
}

// Profile returns the user by id.
func (uc *AuthUseCase) Profile(ctx context.Context, id int64) (*User, error) {
	return uc.users.FindByID(ctx, id)
}

// LoginMeta is the public login-page bootstrap info: whether captcha is
// required and which provider is active. Nothing secret here — the
// endpoint is anonymous. Read straight from startup config (sys.captcha).
type LoginMeta struct {
	CaptchaEnabled  bool
	CaptchaProvider string
	CaptchaSiteKey  string
}

// LoginMeta returns the current login-page bootstrap info for the
// anonymous login-meta endpoint.
func (uc *AuthUseCase) LoginMeta(ctx context.Context) LoginMeta {
	return LoginMeta{
		CaptchaEnabled:  uc.cfg.CaptchaEnabled,
		CaptchaProvider: uc.cfg.CaptchaProvider,
		CaptchaSiteKey:  uc.cfg.CaptchaSiteKey,
	}
}

// IssueCaptcha issues a fresh challenge from the currently configured
// provider (sys.captcha.provider). A provider that needs no server-issued
// challenge (a client-rendered widget, say) returns (nil, nil) — same as an
// unregistered/misconfigured provider name, so a captcha misconfiguration
// degrades to "no captcha" instead of breaking the login page.
func (uc *AuthUseCase) IssueCaptcha(ctx context.Context) (*captcha.Challenge, error) {
	p, ok := captcha.Get(uc.cfg.CaptchaProvider)
	if !ok {
		return nil, nil
	}
	return p.Issue(ctx)
}

// Resolve builds the principal without a session-version check — kept for
// internal/test reuse. It re-resolves via ResolveWithVersion against the
// user's *current* token_version, so it never rejects on staleness.
func (uc *AuthUseCase) Resolve(ctx context.Context, userID int64) (*perm.Principal, error) {
	u, err := uc.users.FindByID(ctx, userID)
	if err != nil {
		return nil, err
	}
	return uc.ResolveWithVersion(ctx, userID, u.TokenVersion)
}

// ResolveWithVersion builds the principal (effective perms) after verifying the
// token's ver claim still matches the persisted token_version — implements
// middleware.PrincipalResolver. A mismatch means the session was invalidated
// by a password change/reset, disable, or kick since the token was issued.
func (uc *AuthUseCase) ResolveWithVersion(ctx context.Context, userID int64, ver int) (*perm.Principal, error) {
	u, err := uc.users.FindByID(ctx, userID)
	if err != nil {
		return nil, err
	}
	if u.Status != 1 {
		return nil, ErrUserDisabled
	}
	if ver != u.TokenVersion {
		return nil, ErrSessionExpired
	}
	p := &perm.Principal{UserID: u.ID, IsRoot: u.IsRoot, DeptID: u.DeptID}
	if u.IsRoot {
		p.Perms = perm.Set{}
		return p, nil
	}
	roleIDs, err := uc.users.RoleIDs(ctx, userID)
	if err != nil {
		return nil, err
	}
	p.RoleIDs = roleIDs
	p.Perms, err = uc.RolePerms(ctx, roleIDs)
	if err != nil {
		return nil, err
	}
	return p, nil
}

// RolePerms resolves role IDs into the effective permission set — the
// role→codes half of ResolveWithVersion, also serving the root permission
// preview (X-Preview-Roles). Unknown role IDs contribute nothing.
func (uc *AuthUseCase) RolePerms(ctx context.Context, roleIDs []int64) (perm.Set, error) {
	var all []string
	for _, rid := range roleIDs {
		codes, ok := uc.cache.GetRoleCodes(ctx, rid)
		if !ok {
			var err error
			codes, err = uc.users.RolePermCodesByRole(ctx, rid)
			if err != nil {
				return nil, err
			}
			uc.cache.SetRoleCodes(ctx, rid, codes)
		}
		all = append(all, codes...)
	}
	return perm.Effective(toCodes(all)), nil
}

// toCodes converts repo/cache-layer []string codes to the strongly typed
// []perm.Code — the single conversion point at the biz boundary.
func toCodes(codes []string) []perm.Code {
	out := make([]perm.Code, len(codes))
	for i, c := range codes {
		out[i] = perm.Code(c)
	}
	return out
}
