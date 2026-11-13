package service

import (
	"context"

	v1 "github.com/go-keel/keel-admin/api/keel_admin/v1"
	"github.com/go-keel/keel-admin/internal/core/perm"
	"github.com/go-keel/keel-admin/internal/core/sys/biz"
	"google.golang.org/protobuf/types/known/emptypb"
)

func actorFrom(ctx context.Context) (*biz.Actor, error) {
	p, ok := perm.FromContext(ctx)
	if !ok {
		return nil, biz.ErrInvalidCredential
	}
	return &biz.Actor{UserID: p.UserID, IsRoot: p.IsRoot, RoleIDs: p.RoleIDs}, nil
}

// UserService implements api.keel_admin.v1.UserService.
type UserService struct {
	v1.UnsafeUserServiceServer
	uc   *biz.UserAdminUseCase
	repo biz.UserAdminRepo
	ref  biz.RefRepo
}

// NewUserService builds the user service handler.
func NewUserService(uc *biz.UserAdminUseCase, repo biz.UserAdminRepo, ref biz.RefRepo) *UserService {
	return &UserService{uc: uc, repo: repo, ref: ref}
}

// ResetOTP leaves both the decision and the write to biz.UserAdminUseCase.ResetOTP: the service only converts
// DTO↔DO and does not chain EnsureManageable + otp.Reset itself.
func (s *UserService) ResetOTP(ctx context.Context, req *v1.ResetOTPRequest) (*emptypb.Empty, error) {
	actor, err := actorFrom(ctx)
	if err != nil {
		return nil, err
	}
	if err := s.uc.ResetOTP(ctx, actor, req.GetId()); err != nil {
		return nil, err
	}
	return &emptypb.Empty{}, nil
}

func (s *UserService) ListUser(ctx context.Context, req *v1.ListUserRequest) (*v1.ListUserReply, error) {
	actor, err := actorFrom(ctx)
	if err != nil {
		return nil, err
	}
	f := biz.UserFilter{
		DeptID:   req.GetDeptId(),
		Keyword:  req.GetKeyword(),
		Status:   req.GetStatus(),
		OTPBound: req.GetOtpBound(),
		RoleID:   req.GetRoleId(),
	}
	us, total, err := s.uc.List(ctx, actor, f, int(req.GetOffset()), int(req.GetLimit()))
	if err != nil {
		return nil, err
	}
	refs, err := s.resolveUserRefs(ctx, us)
	if err != nil {
		return nil, err
	}
	items := make([]*v1.UserListItem, 0, len(us))
	for _, u := range us {
		items = append(items, toUserListItem(u, refs[u.ID]))
	}
	return &v1.ListUserReply{Items: items, Total: total}, nil
}

// ListUserOptions is the login log's user filter option set (fetched in batches; with ids it echoes only those).
func (s *UserService) ListUserOptions(ctx context.Context, req *v1.ListUserOptionsRequest) (*v1.ListUserOptionsReply, error) {
	f := biz.UserFilter{Keyword: req.GetKeyword(), IDs: req.GetIds()}
	us, total, err := s.uc.Options(ctx, f, int(req.GetOffset()), int(req.GetLimit()))
	if err != nil {
		return nil, err
	}
	items := make([]*v1.UserOption, 0, len(us))
	for _, u := range us {
		items = append(items, &v1.UserOption{Id: u.ID, Username: u.Username})
	}
	return &v1.ListUserOptionsReply{Items: items, Total: total}, nil
}

func (s *UserService) CreateUser(ctx context.Context, req *v1.CreateUserRequest) (*v1.UserInfo, error) {
	actor, err := actorFrom(ctx)
	if err != nil {
		return nil, err
	}
	u, err := s.uc.Create(ctx, actor, biz.UserCreate{
		Username: req.GetUsername(),
		Password: req.GetPassword(),
		Nickname: req.GetNickname(),
		Avatar:   req.GetAvatar(),
		Email:    req.GetEmail(),
		Phone:    req.GetPhone(),
		DeptID:   req.GetDeptId(),
		RoleIDs:  req.GetRoleIds(),
		// The initial IP allowlist belongs to create (as editing belongs to update, with no code of its own) and is
		// written together with the account.
		IPWhitelist: req.GetIpWhitelist(),
	})
	if err != nil {
		return nil, err
	}
	return s.toUserInfo(ctx, u), nil
}

// UpdateUser is the single entry point of "edit user": nickname, avatar, contacts, department, roles and IP are
// submitted once with one step-up (see perm.go). Change detection, privilege checks and the single transactional
// write all live in biz.UserAdminUseCase.Edit; this only converts DTO↔DO and does not orchestrate several biz calls,
// because then a range failure on one field would leave the earlier fields written (see Edit's "validate everything
// before writing").
//
// Whether the user is root is not here: it needs its own step-up and still goes through SetUserRoot (the frontend
// offers it separately under "Actions").
func (s *UserService) UpdateUser(ctx context.Context, req *v1.UpdateUserRequest) (*v1.UserInfo, error) {
	actor, err := actorFrom(ctx)
	if err != nil {
		return nil, err
	}
	u, err := s.uc.Edit(ctx, actor, req.GetId(), biz.UserEdit{
		Nickname:    req.GetNickname(),
		Avatar:      req.GetAvatar(),
		Email:       req.GetEmail(),
		Phone:       req.GetPhone(),
		DeptID:      req.GetDeptId(),
		RoleIDs:     req.GetRoleIds(),
		IPWhitelist: req.GetIpWhitelist(),
	})
	if err != nil {
		return nil, err
	}
	return s.toUserInfo(ctx, u), nil
}

func (s *UserService) DisableUser(ctx context.Context, req *v1.DisableUserRequest) (*emptypb.Empty, error) {
	actor, err := actorFrom(ctx)
	if err != nil {
		return nil, err
	}
	if err := s.uc.Disable(ctx, actor, req.GetId()); err != nil {
		return nil, err
	}
	return &emptypb.Empty{}, nil
}

func (s *UserService) EnableUser(ctx context.Context, req *v1.EnableUserRequest) (*emptypb.Empty, error) {
	actor, err := actorFrom(ctx)
	if err != nil {
		return nil, err
	}
	if err := s.uc.Enable(ctx, actor, req.GetId()); err != nil {
		return nil, err
	}
	return &emptypb.Empty{}, nil
}

func (s *UserService) ResetPassword(ctx context.Context, req *v1.ResetPasswordRequest) (*emptypb.Empty, error) {
	actor, err := actorFrom(ctx)
	if err != nil {
		return nil, err
	}
	if err := s.uc.ResetPassword(ctx, actor, req.GetId(), req.GetPassword()); err != nil {
		return nil, err
	}
	return &emptypb.Empty{}, nil
}

func (s *UserService) KickUser(ctx context.Context, req *v1.KickUserRequest) (*emptypb.Empty, error) {
	actor, err := actorFrom(ctx)
	if err != nil {
		return nil, err
	}
	if err := s.uc.Kick(ctx, actor, req.GetId()); err != nil {
		return nil, err
	}
	return &emptypb.Empty{}, nil
}

// GetUser supplies the initial values of the edit form (bound to sys:account:view, see perm.go).
func (s *UserService) GetUser(ctx context.Context, req *v1.GetUserRequest) (*v1.UserInfo, error) {
	actor, err := actorFrom(ctx)
	if err != nil {
		return nil, err
	}
	u, err := s.uc.Get(ctx, actor, req.GetId())
	if err != nil {
		return nil, err
	}
	return s.toUserInfo(ctx, u), nil
}

func (s *UserService) SetUserRoot(ctx context.Context, req *v1.SetUserRootRequest) (*emptypb.Empty, error) {
	actor, err := actorFrom(ctx)
	if err != nil {
		return nil, err
	}
	if err := s.uc.SetRoot(ctx, actor, req.GetId(), req.GetIsRoot()); err != nil {
		return nil, err
	}
	return &emptypb.Empty{}, nil
}

// userRefs holds a user's roles (with names), whether any role enforces OTP, and the department name. Role and
// department names are dynamic values that can change, so the backend resolves and returns them and the frontend
// does not look them up in a snapshot.
type userRefs struct {
	roles     []*v1.UserRoleRef
	otpForced bool
	deptName  string
}

// resolveUserRefs resolves userRefs for a batch of users at once: one query for roles and one for department names,
// so the number of queries does not grow with the rows.
func (s *UserService) resolveUserRefs(ctx context.Context, us []*biz.User) (map[int64]userRefs, error) {
	ids := make([]int64, 0, len(us))
	var deptIDs []int64
	seen := map[int64]bool{}
	for _, u := range us {
		ids = append(ids, u.ID)
		if u.DeptID != 0 && !seen[u.DeptID] {
			seen[u.DeptID] = true
			deptIDs = append(deptIDs, u.DeptID)
		}
	}
	roles, err := s.repo.RolesOfUsers(ctx, ids)
	if err != nil {
		return nil, err
	}
	forced, err := s.repo.ForceOTPUsers(ctx, ids)
	if err != nil {
		return nil, err
	}
	deptNames, err := s.ref.DeptNames(ctx, deptIDs)
	if err != nil {
		return nil, err
	}
	out := make(map[int64]userRefs, len(us))
	for _, u := range us {
		ur := roles[u.ID]
		r := userRefs{roles: make([]*v1.UserRoleRef, 0, len(ur)), deptName: deptNames[u.DeptID], otpForced: forced[u.ID]}
		for _, role := range ur {
			r.roles = append(r.roles, &v1.UserRoleRef{Id: role.ID, Name: role.Name, Disabled: role.Disabled})
		}
		out[u.ID] = r
	}
	return out, nil
}

// toUserInfo builds the reply of detail, create and edit. When name resolution fails the role and department names
// stay empty instead of returning an error: create and edit have already been stored, and an error would make the
// caller believe the write failed and retry.
func (s *UserService) toUserInfo(ctx context.Context, u *biz.User) *v1.UserInfo {
	refs, _ := s.resolveUserRefs(ctx, []*biz.User{u})
	r := refs[u.ID]
	return &v1.UserInfo{
		Id:          u.ID,
		Username:    u.Username,
		Nickname:    u.Nickname,
		DeptId:      u.DeptID,
		DeptName:    r.deptName,
		Status:      int32(u.Status),
		IsRoot:      u.IsRoot,
		Roles:       r.roles,
		OtpForced:   r.otpForced,
		IpWhitelist: u.IPWhitelist,
		OtpBound:    u.OTPBound,
		Avatar:      u.Avatar,
		CreatedAt:   u.CreatedAt,
		Email:       u.Email,
		Phone:       u.Phone,
	}
}

func toUserListItem(u *biz.User, r userRefs) *v1.UserListItem {
	return &v1.UserListItem{
		Id:             u.ID,
		Username:       u.Username,
		Nickname:       u.Nickname,
		Email:          u.Email,
		Phone:          u.Phone,
		DeptName:       r.deptName,
		Roles:          r.roles,
		Status:         int32(u.Status),
		IsRoot:         u.IsRoot,
		OtpBound:       u.OTPBound,
		OtpForced:      r.otpForced,
		HasIpWhitelist: u.IPWhitelist != "",
		CreatedAt:      u.CreatedAt,
	}
}
