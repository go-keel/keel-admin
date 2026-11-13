package data

import (
	"context"
	"errors"

	"gorm.io/gorm"
	"gorm.io/gorm/clause"

	"github.com/go-keel/keel-admin/internal/core/sys/biz"
)

func (r *userRepo) List(ctx context.Context, inDeptIDs []int64, f biz.UserFilter, offset, limit int) ([]*biz.User, int64, error) {
	q := r.data.DB.WithContext(ctx).Model(&sysUser{})
	if inDeptIDs != nil { // Data scope filter: the actor's manageable departments.
		q = q.Where("dept_id IN ?", inDeptIDs)
	}
	if f.DeptID > 0 {
		q = q.Where("dept_id = ?", f.DeptID)
	}
	if f.Keyword != "" {
		like := containsPattern(f.Keyword)
		q = q.Where("username LIKE ? OR nickname LIKE ? OR email LIKE ? OR phone LIKE ?", like, like, like, like)
	}
	if len(f.IDs) > 0 {
		q = q.Where("id IN ?", f.IDs)
	}
	q = whereTriState(q, "status", f.Status)
	q = whereTriState(q, "otp_bound", f.OTPBound)
	if f.RoleID > 0 {
		q = q.Where("id IN (?)", r.data.DB.Model(&sysUserRole{}).
			Select("sys_user_role.user_id").
			Joins(joinHeldRole("sys_role", "sys_user_role.role_id")).
			Where("sys_user_role.role_id = ?", f.RoleID))
	}
	var total int64
	if err := q.Count(&total).Error; err != nil {
		return nil, 0, err
	}
	var ms []sysUser
	if err := q.Order("id asc").Offset(offset).Limit(limit).Find(&ms).Error; err != nil {
		return nil, 0, err
	}
	out := make([]*biz.User, 0, len(ms))
	for i := range ms {
		out = append(out, ms[i].toBiz())
	}
	return out, total, nil
}

// checkDeptSelectable rejects a disabled department as a user's department (0, no department, is always allowed).
// It reads sys_dept without locking: the department chain is never locked together with the user chain.
func checkDeptSelectable(tx *gorm.DB, deptID int64) error {
	if deptID == 0 {
		return nil
	}
	var d sysDept
	err := tx.Select("status").Where("id = ?", deptID).First(&d).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return biz.ErrDeptNotFound
	}
	if err != nil {
		return err
	}
	if d.Status != 1 {
		return biz.ErrDeptDisabled
	}
	return nil
}

func (r *userRepo) Create(ctx context.Context, u *biz.User, roleIDs []int64) error {
	var isRoot int8
	if u.IsRoot {
		isRoot = 1
	}
	return r.data.DB.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := checkDeptSelectable(tx, u.DeptID); err != nil {
			return err
		}
		m := sysUser{
			Username: u.Username, Password: u.Password, Nickname: u.Nickname,
			Email: u.Email, Phone: u.Phone,
			Avatar: u.Avatar, DeptID: u.DeptID, IPWhitelist: u.IPWhitelist, Status: u.Status, IsRoot: isRoot,
		}
		if err := tx.Create(&m).Error; err != nil {
			// The only key this sys_user insert can collide on is uk_username (the primary key auto-increments), so a
			// collision maps straight to "username already exists"; under MySQL the translated error does not say
			// which key collided, so this premise is all there is. Only the main-row insert is mapped: a primary-key
			// collision when inserting sys_user_role means a role was given twice, not a duplicate username.
			if errors.Is(err, gorm.ErrDuplicatedKey) {
				return biz.ErrUserExists
			}
			return err
		}
		u.ID = m.ID
		return insertUserRoles(tx, m.ID, roleIDs)
	})
}

// UpdateAccount builds one UPDATE: the set columns and token_version + 1 are written in the same statement, so either
// all land or none do (biz.UserRepo.UpdateAccount).
func (r *userRepo) UpdateAccount(ctx context.Context, id int64, c biz.AccountChange) error {
	cols := map[string]any{}
	if c.PasswordHash != nil {
		cols["password"] = *c.PasswordHash
	}
	if c.Status != nil {
		cols["status"] = *c.Status
	}
	if c.IsRoot != nil {
		var v int8
		if *c.IsRoot {
			v = 1
		}
		cols["is_root"] = v
	}
	if c.RevokeSessions {
		cols["token_version"] = gorm.Expr("token_version + 1")
	}
	if len(cols) == 0 {
		return nil
	}
	return r.data.DB.WithContext(ctx).Model(&sysUser{}).Where("id = ?", id).Updates(cols).Error
}

// SetProfile updates the nickname, avatar, email and phone number. It uses a map rather than a struct because GORM
// skips a struct's zero values, and then the avatar and contact details could never be cleared.
func (r *userRepo) SetProfile(ctx context.Context, id int64, p biz.ProfileEdit) error {
	return r.data.DB.WithContext(ctx).Model(&sysUser{}).Where("id = ?", id).
		Updates(map[string]any{
			"nickname": p.Nickname, "avatar": p.Avatar, "email": p.Email, "phone": p.Phone,
		}).Error
}

// Edit writes the change set computed by biz.Edit in one transaction: like Create (which writes sys_user +
// sys_user_role in one transaction), an edit must land completely in one commit or not at all. changes carries only
// the fields that really changed, so profile, department and IP columns are built into a map as needed.
//
// The first step locks the sys_user row by primary key with FOR UPDATE: concurrent edits of the same user queue into
// two in turn, and a missing user is detected inside the lock too. When roles are replaced, the current roles are read
// only after locking and handed to changes.Roles to compute the final write set: to pin the roles the operator cannot
// see, it must look at what the user holds at the moment of writing. A plain read here sees the latest commit: every
// path that writes sys_user_role (this method, Create) locks or creates this row first, so other changes are either
// committed or still queued; and as the transaction's first plain read, coming after the locking read, it is where
// MySQL's repeatable-read snapshot is taken.
//
// Locks follow the kernel's user chain, sys_user → sys_user_role, never touching the role chain; no
// role row is locked (sys_role is read only to filter out deleted roles, with a plain read). There is no deadlock
// retry, for the same reason as roleRepo.Save.
func (r *userRepo) Edit(ctx context.Context, id int64, changes biz.UserEditChanges) error {
	return r.data.DB.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		var m sysUser
		err := tx.Clauses(clause.Locking{Strength: clause.LockingStrengthUpdate}).
			Select("id").Where("id = ?", id).First(&m).Error
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return biz.ErrInvalidCredential // The same error as FindByID's "not found".
		}
		if err != nil {
			return err
		}
		if changes.DeptID != nil {
			if err := checkDeptSelectable(tx, *changes.DeptID); err != nil {
				return err
			}
		}
		// Compute roles before writing: it may reject (empty write set), and checks that may return early come first.
		var finalRoles []int64
		var rolesChanged bool
		if changes.Roles != nil {
			current, err := roleIDsOf(tx, id)
			if err != nil {
				return err
			}
			if finalRoles, rolesChanged, err = changes.Roles(current); err != nil {
				return err
			}
		}
		fields := map[string]any{}
		if changes.Nickname != nil {
			fields["nickname"] = *changes.Nickname
		}
		if changes.Avatar != nil {
			fields["avatar"] = *changes.Avatar
		}
		if changes.Email != nil {
			fields["email"] = *changes.Email
		}
		if changes.Phone != nil {
			fields["phone"] = *changes.Phone
		}
		if changes.DeptID != nil {
			fields["dept_id"] = *changes.DeptID
		}
		if changes.IPWhitelist != nil {
			fields["ip_whitelist"] = *changes.IPWhitelist
		}
		if len(fields) > 0 {
			if err := tx.Model(&sysUser{}).Where("id = ?", id).Updates(fields).Error; err != nil {
				return err
			}
		}
		if !rolesChanged {
			return nil
		}
		// Replace with finalRoles as a whole: delete only what was removed and insert only what was added (see
		// replaceKeys). It compares every row in the table, so old rows on deleted roles are not in finalRoles and go.
		return replaceKeys(tx, "user_id", id, "role_id", finalRoles, func(rid int64) sysUserRole {
			return sysUserRole{UserID: id, RoleID: rid}
		})
	})
}

// RoleIDs returns the roles the user holds; the meaning is described at roleIDsOf.
func (r *userRepo) RoleIDs(ctx context.Context, id int64) ([]int64, error) {
	return roleIDsOf(r.data.DB.WithContext(ctx), id)
}

// roleIDsOf is the holding relation ("held", see role_join.go): when editing a user (inside Edit's lock, hence the tx
// parameter) it pins the existing roles the operator cannot see.
func roleIDsOf(db *gorm.DB, id int64) ([]int64, error) {
	var ids []int64
	err := db.Model(&sysUserRole{}).
		Joins(joinHeldRole("sys_role", "sys_user_role.role_id")).
		Where("sys_user_role.user_id = ?", id).
		Pluck("sys_user_role.role_id", &ids).Error
	return ids, err
}

// ForceOTPUsers shares forceOTPUsers (otp.go) with the login check.
func (r *userRepo) ForceOTPUsers(ctx context.Context, ids []int64) (map[int64]bool, error) {
	return forceOTPUsers(r.data.DB.WithContext(ctx), ids)
}

// RolesOf joins to get the names of the roles a user holds, sharing one query with the full-page version.
func (r *userRepo) RolesOf(ctx context.Context, id int64) ([]biz.UserRole, error) {
	m, err := r.RolesOfUsers(ctx, []int64{id})
	if err != nil {
		return nil, err
	}
	return m[id], nil
}

func (r *userRepo) RolesOfUsers(ctx context.Context, ids []int64) (map[int64][]biz.UserRole, error) {
	out := map[int64][]biz.UserRole{}
	if len(ids) == 0 {
		return out, nil
	}
	var rows []struct {
		UserID int64
		biz.UserRole
	}
	if err := r.data.DB.WithContext(ctx).Model(&sysUserRole{}).
		Select("sys_user_role.user_id AS user_id, sys_role.id AS id, sys_role.name AS name, sys_role.status <> 1 AS disabled").
		Joins(joinHeldRole("sys_role", "sys_user_role.role_id")).
		Where("sys_user_role.user_id IN ?", ids).
		Order("sys_user_role.user_id asc, sys_role.id asc").
		Scan(&rows).Error; err != nil {
		return nil, err
	}
	for _, row := range rows {
		out[row.UserID] = append(out[row.UserID], row.UserRole)
	}
	return out, nil
}

func insertUserRoles(tx *gorm.DB, userID int64, roleIDs []int64) error {
	if len(roleIDs) == 0 {
		return nil
	}
	rows := make([]sysUserRole, 0, len(roleIDs))
	for _, rid := range roleIDs {
		rows = append(rows, sysUserRole{UserID: userID, RoleID: rid})
	}
	return tx.Create(&rows).Error
}
