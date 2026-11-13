package service

import (
	"context"
	"strings"
	"time"

	v1 "github.com/go-keel/keel-admin/api/keel_admin/v1"
	"github.com/go-keel/keel-admin/internal/core/errs"
	"github.com/go-keel/keel-admin/internal/core/perm"
	"github.com/go-keel/keel-admin/internal/core/sys/biz"
	kauth "github.com/go-keel/keel/middleware/auth"
	"github.com/go-keel/keel/middleware/metadata"
	"google.golang.org/protobuf/types/known/emptypb"
)

// AuthService implements api.keel_admin.v1.AuthService.
type AuthService struct {
	v1.UnsafeAuthServiceServer
	uc           *biz.AuthUseCase
	otp          *biz.OTPUseCase
	users        biz.UserAdminRepo
	ref          biz.RefRepo
	sessionTTL   time.Duration
	cookieName   string
	cookieSecure bool
}

// NewAuthService builds the auth service handler. cookieName is shared with the authentication middleware
// (server.http.default.auth.cookie_name); sessionTTL and cookieSecure come from sys.*. users and ref are used only by
// GetMyAccount to resolve role and department names (as UserService does).
func NewAuthService(uc *biz.AuthUseCase, otp *biz.OTPUseCase, users biz.UserAdminRepo, ref biz.RefRepo, sessionTTL time.Duration, cookieName string, cookieSecure bool) (*AuthService, error) {
	cookieName = strings.TrimSpace(cookieName)
	if cookieName == "" {
		return nil, errs.Rules.New("sys.cookie-name-missing", "server.http.default.auth.cookie_name is empty",
			"set it in configs/config.yaml; the session cookie and the authentication middleware share that name")
	}
	return &AuthService{
		uc:           uc,
		otp:          otp,
		users:        users,
		ref:          ref,
		sessionTTL:   sessionTTL,
		cookieName:   cookieName,
		cookieSecure: cookieSecure,
	}, nil
}

func (s *AuthService) Login(ctx context.Context, req *v1.LoginRequest) (*v1.LoginReply, error) {
	ip := metadata.GetRemoteIP(ctx)
	token, bindToken, u, err := s.uc.Login(ctx, req.GetUsername(), req.GetPassword(), req.GetOtpCode(), req.GetCaptchaId(), req.GetCaptchaAnswer(), ip)
	if err != nil {
		return nil, err
	}
	// The token is sent in an httpOnly cookie, not in the response body: the frontend never touches it, so XSS cannot
	// steal it. bindToken is the exception: it is the restricted credential of the bootstrap flow, and the frontend
	// passes it explicitly to BindOTP/ConfirmOTP.
	if token != "" {
		setSessionCookie(ctx, s.cookieName, token, s.sessionTTL, req.GetRemember(), s.cookieSecure)
	}
	return &v1.LoginReply{
		UserId:    u.ID,
		Username:  u.Username,
		IsRoot:    u.IsRoot,
		BindToken: bindToken, // Non-empty means OTP binding must be completed first (no session cookie was issued).
	}, nil
}

// Logout clears the session cookie. The cookie is httpOnly, so the frontend cannot delete it and only the server
// can. It allows anonymous access: someone whose session has expired must still be able to clear the cookie,
// otherwise the browser keeps sending a dead ticket and every request runs into a pointless 401.
func (s *AuthService) Logout(ctx context.Context, _ *v1.LogoutRequest) (*emptypb.Empty, error) {
	clearSessionCookie(ctx, s.cookieName, s.cookieSecure)
	return &emptypb.Empty{}, nil
}

// LoginMeta is read by the login page before it starts: whether captcha is on and which provider it uses. It allows
// anonymous access (proto allow_anonymous).
func (s *AuthService) LoginMeta(ctx context.Context, _ *v1.LoginMetaRequest) (*v1.LoginMetaReply, error) {
	m := s.uc.LoginMeta(ctx)
	return &v1.LoginMetaReply{
		CaptchaEnabled:  m.CaptchaEnabled,
		CaptchaProvider: m.CaptchaProvider,
		CaptchaSiteKey:  m.CaptchaSiteKey,
	}, nil
}

// UpdateMyProfile lets users change their own nickname, avatar, email and phone number.
func (s *AuthService) UpdateMyProfile(ctx context.Context, req *v1.UpdateMyProfileRequest) (*emptypb.Empty, error) {
	p, ok := perm.FromContext(ctx)
	if !ok {
		return nil, biz.ErrInvalidCredential
	}
	if err := s.uc.UpdateMyProfile(ctx, p.UserID, biz.ProfileEdit{
		Nickname: req.GetNickname(),
		Avatar:   req.GetAvatar(),
		Email:    req.GetEmail(),
		Phone:    req.GetPhone(),
	}); err != nil {
		return nil, err
	}
	return &emptypb.Empty{}, nil
}

// ChangeMyPassword lets users change their own password (the old password is verified).
func (s *AuthService) ChangeMyPassword(ctx context.Context, req *v1.ChangeMyPasswordRequest) (*emptypb.Empty, error) {
	p, ok := perm.FromContext(ctx)
	if !ok {
		return nil, biz.ErrInvalidCredential
	}
	if err := s.uc.ChangeMyPassword(ctx, p.UserID, req.GetOldPassword(), req.GetNewPassword()); err != nil {
		return nil, err
	}
	return &emptypb.Empty{}, nil
}

func (s *AuthService) BindOTP(ctx context.Context, _ *v1.BindOTPRequest) (*v1.BindOTPReply, error) {
	p, ok := perm.FromContext(ctx)
	if !ok {
		return nil, biz.ErrInvalidCredential
	}
	u, err := s.uc.Profile(ctx, p.UserID)
	if err != nil {
		return nil, err
	}
	uri, err := s.otp.Bind(ctx, p.UserID, u.Username)
	if err != nil {
		return nil, err
	}
	return &v1.BindOTPReply{ProvisioningUri: uri}, nil
}

// UnbindOTP unbinds the user's own authenticator and requires the password again (see biz.OTPUseCase.Unbind).
func (s *AuthService) UnbindOTP(ctx context.Context, req *v1.UnbindOTPRequest) (*emptypb.Empty, error) {
	p, ok := perm.FromContext(ctx)
	if !ok {
		return nil, biz.ErrInvalidCredential
	}
	// Password verification belongs to auth (it owns the account) and clearing the binding to otp; each does its own
	// part, so the password hash is not passed around in the service layer.
	if err := s.uc.VerifyPassword(ctx, p.UserID, req.GetPassword()); err != nil {
		return nil, err
	}
	if err := s.otp.Unbind(ctx, p.UserID); err != nil {
		return nil, err
	}
	return &emptypb.Empty{}, nil
}

// ConfirmOTP verifies the first code and activates the binding.
//
// In the bootstrap flow (bind-scope token) it also issues the access token right away: the password was verified
// when the bind token was issued and the OTP has just been verified, so both factors are present (see
// biz.CompleteOTPBootstrap). A logged-in user rebinding on their own gets no token (the caller already holds one), so
// the distinction is by scope, not by whether a token is present.
func (s *AuthService) ConfirmOTP(ctx context.Context, req *v1.ConfirmOTPRequest) (*v1.ConfirmOTPReply, error) {
	p, ok := perm.FromContext(ctx)
	if !ok {
		return nil, biz.ErrInvalidCredential
	}
	if err := s.otp.Confirm(ctx, p.UserID, req.GetCode()); err != nil {
		return nil, err
	}
	if !isBindScope(ctx) {
		return &v1.ConfirmOTPReply{}, nil
	}
	token, err := s.uc.CompleteOTPBootstrap(ctx, p.UserID, metadata.GetRemoteIP(ctx))
	if err != nil {
		return nil, err
	}
	setSessionCookie(ctx, s.cookieName, token, s.sessionTTL, req.GetRemember(), s.cookieSecure)
	return &v1.ConfirmOTPReply{LoggedIn: true}, nil
}

// isBindScope reports whether this call uses the restricted token of the OTP binding bootstrap (the scope allowlist
// is in middleware/authz.go).
func isBindScope(ctx context.Context) bool {
	claims, ok := kauth.FromContext(ctx)
	if !ok {
		return false
	}
	scope, _ := claims["scope"].(string)
	return scope == biz.BindScope
}

// IssueCaptcha issues a fresh challenge from the currently configured
// captcha provider. A provider that needs no server-issued challenge, or an
// unregistered/misconfigured provider name, both surface as an empty reply
// (captcha_id="") rather than an error.
func (s *AuthService) IssueCaptcha(ctx context.Context, _ *v1.IssueCaptchaRequest) (*v1.IssueCaptchaReply, error) {
	ch, err := s.uc.IssueCaptcha(ctx)
	if err != nil {
		return nil, err
	}
	if ch == nil {
		return &v1.IssueCaptchaReply{}, nil
	}
	return &v1.IssueCaptchaReply{CaptchaId: ch.ID, ImageBase64: ch.ImageBase64}, nil
}

func (s *AuthService) Profile(ctx context.Context, _ *v1.ProfileRequest) (*v1.ProfileReply, error) {
	p, ok := perm.FromContext(ctx)
	if !ok {
		return nil, biz.ErrInvalidCredential
	}
	u, err := s.uc.Profile(ctx, p.UserID)
	if err != nil {
		return nil, err
	}
	perms := make([]string, 0, len(p.Perms))
	for c := range p.Perms {
		perms = append(perms, string(c))
	}
	return &v1.ProfileReply{
		UserId:   u.ID,
		Username: u.Username,
		Nickname: u.Nickname,
		DeptId:   u.DeptID,
		// Take the principal rather than u: the principal is this request's **effective** identity, and in a
		// reduced-permission preview root has been downgraded (see middleware.Principal). Using u.IsRoot would make
		// the frontend's access check allow everything for root, so the menu would not narrow and the user would only
		// hit the backend's 403 after clicking in.
		IsRoot:   p.IsRoot,
		Perms:    perms,
		Avatar:   u.Avatar,
		OtpBound: u.OTPBound,
		Settings: settingsReply(&u.Settings),
	}, nil
}

// recentLoginCount is the number of recent logins listed in the personal center, matching "Recent logins" in the
// user detail drawer.
const recentLoginCount = 5

// GetMyAccount returns the user's own account details (for the personal center). Role and department names are
// dynamic values that can change, so the backend resolves and returns them, as UserService.toUserInfo does.
func (s *AuthService) GetMyAccount(ctx context.Context, _ *v1.GetMyAccountRequest) (*v1.MyAccountReply, error) {
	p, ok := perm.FromContext(ctx)
	if !ok {
		return nil, biz.ErrInvalidCredential
	}
	u, err := s.uc.Profile(ctx, p.UserID)
	if err != nil {
		return nil, err
	}
	userRoles, err := s.users.RolesOf(ctx, u.ID)
	if err != nil {
		return nil, err
	}
	roles := make([]*v1.NamedRef, 0, len(userRoles))
	for _, r := range userRoles {
		roles = append(roles, &v1.NamedRef{Id: r.ID, Name: r.Name})
	}
	var deptName string
	if u.DeptID != 0 {
		names, err := s.ref.DeptNames(ctx, []int64{u.DeptID})
		if err != nil {
			return nil, err
		}
		deptName = names[u.DeptID]
	}
	logs, err := s.uc.RecentLogins(ctx, u.ID, recentLoginCount)
	if err != nil {
		return nil, err
	}
	recent := make([]*v1.RecentLogin, 0, len(logs))
	for _, l := range logs {
		recent = append(recent, &v1.RecentLogin{
			Id: l.ID, Success: l.Success, Ip: l.IP, CreatedAt: l.CreatedAt,
		})
	}
	return &v1.MyAccountReply{
		Username:     u.Username,
		Nickname:     u.Nickname,
		Avatar:       u.Avatar,
		Email:        u.Email,
		Phone:        u.Phone,
		DeptName:     deptName,
		Roles:        roles,
		CreatedAt:    u.CreatedAt,
		OtpBound:     u.OTPBound,
		OtpBoundAt:   u.OTPBoundAt,
		IpWhitelist:  u.IPWhitelist,
		RecentLogins: recent,
	}, nil
}

// AddFavorite / RemoveFavorite add and remove menus in the user's own favorites and return the full settings
// afterwards. biz guarantees idempotency; the repo's row lock keeps two concurrent changes from overwriting each
// other.
func (s *AuthService) AddFavorite(ctx context.Context, req *v1.AddFavoriteRequest) (*v1.UserSettings, error) {
	p, ok := perm.FromContext(ctx)
	if !ok {
		return nil, biz.ErrInvalidCredential
	}
	st, err := s.uc.AddFavorite(ctx, p.UserID, req.GetPath())
	if err != nil {
		return nil, err
	}
	return settingsReply(st), nil
}

func (s *AuthService) RemoveFavorite(ctx context.Context, req *v1.RemoveFavoriteRequest) (*v1.UserSettings, error) {
	p, ok := perm.FromContext(ctx)
	if !ok {
		return nil, biz.ErrInvalidCredential
	}
	st, err := s.uc.RemoveFavorite(ctx, p.UserID, req.GetPath())
	if err != nil {
		return nil, err
	}
	return settingsReply(st), nil
}

// ReorderFavorites reorders the user's own favorites. It overwrites the whole order but accepts only a permutation of
// the current set; why it is not a plain full overwrite is explained on biz.ReorderFavorites.
func (s *AuthService) ReorderFavorites(ctx context.Context, req *v1.ReorderFavoritesRequest) (*v1.UserSettings, error) {
	p, ok := perm.FromContext(ctx)
	if !ok {
		return nil, biz.ErrInvalidCredential
	}
	st, err := s.uc.ReorderFavorites(ctx, p.UserID, req.GetPaths())
	if err != nil {
		return nil, err
	}
	return settingsReply(st), nil
}

func settingsReply(s *biz.UserSettings) *v1.UserSettings {
	if s == nil {
		return &v1.UserSettings{}
	}
	return &v1.UserSettings{FavoriteMenus: s.FavoriteMenus}
}
