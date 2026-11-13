// Package biz is the business area's use-case layer, deliberately outside internal/core/sys: business domains always
// live here, never in the kernel.
package biz

import "errors"

// Example: delete after forking
// ErrNoticeNotFound: the id does not exist, or exists outside the actor's data scope (the two are indistinguishable,
// the same meaning as core/sys/biz.ErrRoleNotFound and the other not-found errors).
var ErrNoticeNotFound = errors.New("notice not found")

// Example: delete after forking
// ErrNoticeCategoryNotFound: the category id does not exist, or exists outside the actor's category grants (the two
// are indistinguishable, so the existence of a category does not leak to someone without access).
var ErrNoticeCategoryNotFound = errors.New("notice category not found")

// Example: delete after forking
// ErrNoticeCategoryInUse: the category still has notices and cannot be deleted (move or delete its notices first).
var ErrNoticeCategoryInUse = errors.New("notice category in use")

// Example: delete after forking
// ErrNoticeCategoryExists: duplicate category name. Categories are explicit-assignment options (a multi-select in
// role configuration), and two with the same name could not be told apart in the selector, so names are unique.
var ErrNoticeCategoryExists = errors.New("notice category exists")
