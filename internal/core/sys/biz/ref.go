package biz

import "context"

// RefRepo resolves IDs of mutable entities to display names in batches, so lists and details render names
// returned by the backend instead of the frontend looking them up with .find() in a snapshot loaded at mount (role
// names, department names and other values that can be added, changed or deleted). A missing (deleted) ID is
// absent from the returned map, and the caller falls back to showing #id.
type RefRepo interface {
	RoleNames(ctx context.Context, ids []int64) (map[int64]string, error)
	DeptNames(ctx context.Context, ids []int64) (map[int64]string, error)
	// UserNames uses the nickname as the display name, or the username when the nickname is empty.
	UserNames(ctx context.Context, ids []int64) (map[int64]string, error)
}

// UserRole is one role a user holds (disabled ones included), so lists and details render the role name resolved
// by the backend without a frontend lookup. The "binding required but not bound" hint is not computed from it: a
// disabled role is held but does not enforce anything; see UserAdminRepo.ForceOTPUsers.
type UserRole struct {
	ID       int64
	Name     string
	Disabled bool // the role is disabled: still held, grants nothing
}
