package data

import (
	"context"
	"time"

	"gorm.io/gorm"

	coredata "github.com/go-keel/keel-admin/internal/core/data"
	"github.com/go-keel/keel-admin/internal/core/sys/biz"
)

var _ biz.OTPRepo = (*otpRepo)(nil)

type otpRepo struct {
	data *coredata.Data
}

// NewOTPRepo builds the OTP persistence repository.
func NewOTPRepo(data *coredata.Data) biz.OTPRepo { return &otpRepo{data: data} }

func (r *otpRepo) GetOTP(ctx context.Context, userID int64) ([]byte, bool, error) {
	var m sysUser
	if err := r.data.DB.WithContext(ctx).Select("otp_secret_enc", "otp_bound").
		Where("id = ?", userID).First(&m).Error; err != nil {
		return nil, false, err
	}
	return m.OTPSecretEnc, m.OTPBound == 1, nil
}

func (r *otpRepo) SetOTPSecret(ctx context.Context, userID int64, enc []byte) error {
	return r.data.DB.WithContext(ctx).Model(&sysUser{}).Where("id = ?", userID).
		Updates(map[string]any{"otp_secret_enc": enc, "otp_bound": 0}).Error
}

func (r *otpRepo) SetOTPBound(ctx context.Context, userID int64) error {
	return r.data.DB.WithContext(ctx).Model(&sysUser{}).Where("id = ?", userID).
		Updates(map[string]any{"otp_bound": 1, "otp_bound_at": time.Now()}).Error
}

func (r *otpRepo) ClearOTP(ctx context.Context, userID int64) error {
	return r.data.DB.WithContext(ctx).Model(&sysUser{}).Where("id = ?", userID).
		Updates(map[string]any{"otp_secret_enc": nil, "otp_bound": 0, "otp_bound_at": nil}).Error
}

func (r *otpRepo) UserForceOTP(ctx context.Context, userID int64) (bool, error) {
	forced, err := forceOTPUsers(r.data.DB.WithContext(ctx), []int64{userID})
	return forced[userID], err
}

// forceOTPUsers is the one definition of "must bind an authenticator" through roles: the user holds an active role
// with force_otp. The login check (UserForceOTP) and the user list's "forced but not bound" hint (ForceOTPUsers) both
// use it, so one cannot count disabled roles while the other does not. root's system-wide requirement is not here; see
// biz/auth.go.
func forceOTPUsers(db *gorm.DB, ids []int64) (map[int64]bool, error) {
	out := map[int64]bool{}
	if len(ids) == 0 {
		return out, nil
	}
	var users []int64
	if err := db.Model(&sysUserRole{}).
		Joins(joinActiveRole("sys_role", "sys_user_role.role_id")).
		Where("sys_user_role.user_id IN ? AND sys_role.force_otp = 1", ids).
		Distinct().Pluck("sys_user_role.user_id", &users).Error; err != nil {
		return nil, err
	}
	for _, id := range users {
		out[id] = true
	}
	return out, nil
}
