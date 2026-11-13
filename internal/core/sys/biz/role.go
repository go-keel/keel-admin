package biz

import (
	"context"
	"errors"
	"fmt"

	"github.com/go-keel/keel-admin/internal/core/perm"
)

// Role is a flat role.
//
// AssignableRoleIDs, ManageDepts and Scopes read back the three boundary dimensions and are filled by List and
// Get. Without them the configuration interface could not show the configured values, and since all of them are
// replaced as a whole, the interface would have to submit empty values, clearing them every time it is opened.
type Role struct {
	ID   int64
	Code string
	Name string
	// Sensitive means non-root cannot assign this role to other users (assignableRange removes it). That is its
	// only meaning: it does not stop root from editing or deleting the role.
	Sensitive int8
	ForceOTP  int8
	Status    int8
	Perms     []string

	AssignableRoleIDs []int64
	// ManageDepts lists the manageable department IDs; each implies its whole subtree (managing a department means
	// managing its whole line).
	ManageDepts []int64
	Scopes      []DataScopeInput
}

// RoleUpdate is a role's full mutable state — Update replaces all of it in one
// call. These fields have exactly the same gate (root-only, step-up and audit), so they are not split into several
// operations; see docs/adr/admin-0007-operation-split-and-step-up-placement.md.
//
// Perms, AssignableRoleIDs, ManageDepts and Scopes are all replaced as a whole, and an omitted one is cleared: the
// caller must Get the current values first and send them all back.
type RoleUpdate struct {
	Name              string
	ForceOTP          int8
	Status            int8
	Sensitive         int8
	Perms             []string
	AssignableRoleIDs []int64
	ManageDepts       []int64
	Scopes            []DataScopeInput
}

// RoleListRow is one row of the role management list.
//
// It is a separate type from Role rather than Role plus a PermCount: the list wants "how many codes", the detail
// wants "which codes". Sharing one struct would leave Perms and the three boundaries empty in the list, and an
// empty field is sooner or later read as "looked up, and empty".
type RoleListRow struct {
	ID        int64
	Code      string
	Name      string
	Sensitive int8
	ForceOTP  int8
	Status    int8
	PermCount int32
	CreatedAt string // UTC RFC3339, formatted by the data layer with timefmt
}

// RoleFilter is the ListRole query filter.
//
// The zero value filters nothing, and tri-state fields use the sentinels 1 = yes / 2 = no, the same convention as
// UserFilter; mixing in a second form would leave callers guessing.
type RoleFilter struct {
	// Keyword matches code or name.
	Keyword string
	// Status is 0 = all, 1 = enabled, 2 = disabled (sys_role.status 1/0).
	Status int32
	// Sensitive and ForceOTP are 0 = all, 1 = yes, 2 = no.
	Sensitive int32
	ForceOTP  int32
}

// RoleRepo is the role persistence port.
type RoleRepo interface {
	// List is the data source of the role management list: paging and filtering run in the database, returning the
	// rows of this page and the total. It cannot send everything for the frontend to filter: after paging the
	// frontend holds only the current page, so it would filter "the matches on this page", not "the first page of
	// matches".
	List(ctx context.Context, f RoleFilter, offset, limit int) ([]*RoleListRow, int64, error)
	// ListBrief reads only the identifying columns (id/code/name/sensitive/status) without the perm N+1. Selectors
	// and other places that need only names use it instead of dragging along each row's permission codes.
	ListBrief(ctx context.Context) ([]*Role, error)
	// Get returns ErrRoleNotFound when the role does not exist (deleted included) and other errors as they are: the
	// caller passes them up unchanged, because a database failure must not be reported as "role not found".
	Get(ctx context.Context, id int64) (*Role, error)
	GetByCode(ctx context.Context, code string) (*Role, error)
	// Save is the only write path of a role: the basic fields and all configuration (Perms, AssignableRoleIDs,
	// ManageDepts, Scopes) are written in one transaction that rolls back as a whole on failure. With r.ID == 0 it
	// creates the role and fills in the ID; otherwise it updates the role (code unchanged) and returns
	// ErrRoleNotFound when the role does not exist or is deleted. Each configuration is replaced as a whole, and an
	// empty value clears it.
	Save(ctx context.Context, r *Role) error
	Delete(ctx context.Context, id int64) error
	// DataScopes reads back the scope configuration of the given roles in batch (roleID -> items). Batch rather
	// than one by one: List already does an N+1 on perms, and reading the boundaries back need not add three more.
	DataScopes(ctx context.Context, roleIDs []int64) (map[int64][]DataScopeInput, error)
}

// DataScopeInput is one role×resource scope policy to persist.
// ResourceIDs is used only by RESOURCE_GRANT: the resource instance IDs explicitly granted to the role (stored in
// sys_role_resource_grant and replaced as a whole in the same transaction as the policy rows).
type DataScopeInput struct {
	ResourceType string
	ScopeType    ScopeType
	ResourceIDs  []int64
}

func validScopeType(s ScopeType) bool {
	switch s {
	case ScopeSelf, ScopeDept, ScopeAllSub, ScopeGrant, ScopeAll:
		return true
	}
	return false
}

// RoleUseCase manages roles (root-only operations enforced at transport).
type RoleUseCase struct {
	repo  RoleRepo
	arbac ARBACRepo
	cache CacheInvalidator
}

// NewRoleUseCase builds the role use case.
func NewRoleUseCase(repo RoleRepo, arbac ARBACRepo, cache CacheInvalidator) *RoleUseCase {
	return &RoleUseCase{repo: repo, arbac: arbac, cache: cache}
}

// List is the role management list: paging and filtering go to the repo, and it returns slim rows (see
// RoleListRow).
//
// It carries neither the three boundaries nor perms: those are the edit form's initial values and come from Get. If
// the list sent them, the next person would naturally prefill the form from a list row, which under whole
// replacement is a silent overwrite.
func (uc *RoleUseCase) List(ctx context.Context, f RoleFilter, offset, limit int) ([]*RoleListRow, int64, error) {
	if limit <= 0 { // a negative limit also counts as "not given": gorm treats a negative Limit as unlimited
		limit = defaultPageSize
	}
	return uc.repo.List(ctx, f, offset, limit)
}

// Options returns the role selector's option set (id/code/name only, see RoleOption).
//
// It serves dropdowns such as "assignable roles" and does not reuse List, the heavy payload with perms and the three
// boundaries. excludeID>0 excludes that role (the role itself when editing); excludeSensitive excludes sensitive
// roles: non-root can never assign them, so putting a sensitive role into a role's assignable set is dead
// configuration (assignableRange removes sensitive roles). Filtering sits at the endpoint, so the frontend neither
// filters by itself nor pulls the whole role list into the browser to pick from.
func (uc *RoleUseCase) Options(ctx context.Context, excludeID int64, excludeSensitive bool) ([]*Role, error) {
	rs, err := uc.repo.ListBrief(ctx)
	if err != nil {
		return nil, err
	}
	out := make([]*Role, 0, len(rs))
	for _, r := range rs {
		if r.ID == excludeID || (excludeSensitive && r.Sensitive == 1) {
			continue
		}
		out = append(out, r)
	}
	return out, nil
}

// Get is the source of the edit form's initial values: it carries all current values (perms and the three
// boundaries).
//
// The edit form must not prefill from a list row: that is a snapshot, these fields are replaced as a whole, and
// submitting stale values silently deletes configuration someone else just added. See
// docs/adr/admin-0006-form-initial-values-fetch-detail.md.
func (uc *RoleUseCase) Get(ctx context.Context, id int64) (*Role, error) {
	r, err := uc.repo.Get(ctx, id)
	if err != nil {
		return nil, err
	}
	if err := uc.fillBoundaries(ctx, []*Role{r}); err != nil {
		return nil, err
	}
	return r, nil
}

// PermissionsOf returns the function permission codes a role grants, for business administrators with the right
// to assign it: before handing a role to someone, one needs to see what it gives.
//
// Non-root can query only roles it can assign (assignableRange); otherwise this would be a hole for enumerating
// the permissions of any role, leaking sensitive roles too (which non-root should never see).
func (uc *RoleUseCase) PermissionsOf(ctx context.Context, actor *Actor, id int64) ([]string, error) {
	r, err := uc.repo.Get(ctx, id)
	if err != nil {
		return nil, err
	}
	allowed, all, err := assignableRange(ctx, uc.arbac, actor)
	if err != nil {
		return nil, err
	}
	if !all && !allowed[id] {
		return nil, ErrRoleOutOfRange
	}
	return r.Perms, nil
}

// fillBoundaries reads back the three boundaries in batch (one query per dimension, no per-row fan-out).
func (uc *RoleUseCase) fillBoundaries(ctx context.Context, rs []*Role) error {
	if len(rs) == 0 {
		return nil
	}
	ids := make([]int64, 0, len(rs))
	for _, r := range rs {
		ids = append(ids, r.ID)
	}
	assignable, err := uc.arbac.AssignableConfigs(ctx, ids)
	if err != nil {
		return err
	}
	mdepts, err := uc.arbac.ManageDeptConfigs(ctx, ids)
	if err != nil {
		return err
	}
	scopes, err := uc.repo.DataScopes(ctx, ids)
	if err != nil {
		return err
	}
	for _, r := range rs {
		r.AssignableRoleIDs = assignable[r.ID]
		r.ManageDepts = mdepts[r.ID]
		r.Scopes = scopes[r.ID]
	}
	return nil
}

// ListAssignable is the option set of the assignment dropdown: everything for root, and for non-root the roles
// assignableRange selects.
//
// An option set is not paged: the dropdown must reach any item. It uses ListBrief, since it never needs perms and
// has no reason to run an N+1 for them.
func (uc *RoleUseCase) ListAssignable(ctx context.Context, actor *Actor) ([]*Role, error) {
	rs, err := uc.repo.ListBrief(ctx)
	if err != nil {
		return nil, err
	}
	allowed, all, err := assignableRange(ctx, uc.arbac, actor)
	if err != nil || all {
		return rs, err
	}
	out := make([]*Role, 0, len(allowed))
	for _, r := range rs {
		if allowed[r.ID] {
			out = append(out, r)
		}
	}
	return out, nil
}

// Create creates a role with all editable fields at once (basic fields, permissions and the three boundaries),
// taking the same input struct RoleUpdate as Update; Status is stored as given (a role can be created disabled).
// The reason for merging is the same as for Update: the fields share one gate, so the frontend submits one form
// with one step-up.
//
// The checks that can fail run first (duplicate code, dead-code filtering, scope validity), then repo.Save writes
// in one transaction: the main row and the five configuration tables succeed or fail together, so no new role is
// left with partial configuration.
func (uc *RoleUseCase) Create(ctx context.Context, code string, in RoleUpdate) (*Role, error) {
	// This lookup only rejects early and cannot stop concurrent creation of the same code: sys_role.uk_code decides
	// in the end, and repo.Save returns ErrDuplicateRole on the key conflict. Only ErrRoleNotFound lets creation
	// continue; a failing query is returned as is and must not be taken as "does not exist".
	if _, err := uc.repo.GetByCode(ctx, code); err == nil {
		return nil, ErrDuplicateRole
	} else if !errors.Is(err, ErrRoleNotFound) {
		return nil, err
	}
	in.Perms = knownPerms(in.Perms)
	if err := validateScopes(ctx, in.Scopes); err != nil {
		return nil, err
	}
	r := &Role{
		Code: code, Name: in.Name, ForceOTP: in.ForceOTP, Sensitive: in.Sensitive, Status: in.Status,
		Perms: in.Perms, AssignableRoleIDs: in.AssignableRoleIDs, ManageDepts: in.ManageDepts, Scopes: in.Scopes,
	}
	if err := uc.repo.Save(ctx, r); err != nil {
		return nil, err
	}
	// Nobody holds the new role yet, so there is no perm cache to invalidate.
	return r, nil
}

// Update replaces a role's whole mutable state (basic fields, permissions and the three boundaries) in one
// submission (the reason for merging is in the comment on RoleUpdate).
//
// All input is checked first, then repo.Save writes it in one transaction that rolls back as a whole on failure.
// Save locks the role's main row inside the transaction before writing the configuration: when two people edit
// the same role at once, the later one waits for the earlier commit, and the database keeps the complete
// configuration of one submission rather than an interleaved mix of both.
func (uc *RoleUseCase) Update(ctx context.Context, id int64, in RoleUpdate) (*Role, error) {
	// Load all current values (boundaries included): the audit's before must tell what the role looked like before
	// the change, not just its name and status.
	r, err := uc.Get(ctx, id)
	if err != nil {
		return nil, err
	}
	// Drop function permission codes missing from the registry instead of failing: codes are fixed at coding time,
	// and after a code is removed from the code base a role in the database may still hold it. It is a dead code
	// (the effective expansion ignores unknown codes anyway) and must not fail the whole edit. A role edit replaces
	// everything, so it simply clears the dead codes along the way.
	in.Perms = knownPerms(in.Perms)
	if err := validateScopes(ctx, in.Scopes); err != nil {
		return nil, err
	}
	r.Name, r.ForceOTP, r.Status, r.Sensitive = in.Name, in.ForceOTP, in.Status, in.Sensitive
	r.Perms, r.AssignableRoleIDs = in.Perms, in.AssignableRoleIDs
	r.ManageDepts, r.Scopes = in.ManageDepts, in.Scopes
	if err := uc.repo.Save(ctx, r); err != nil {
		return nil, err
	}
	// Changing a role changes what it permits: which codes were granted or revoked is the thing in this model that
	// most needs a trace. The cache is invalidated after the transaction commits: invalidating before the commit
	// would let another request read the old configuration back into the cache before the commit.
	uc.cache.InvalidateRole(ctx, id)
	return r, nil
}

// Delete does not refuse because of Sensitive: that flag only governs "can non-root assign it", not whether root
// can delete it. The repo soft-deletes only the main record and keeps the related configuration; runtime queries
// anchor on active sys_role rows, so the revocation takes effect immediately and the historical configuration is
// not lost.
func (uc *RoleUseCase) Delete(ctx context.Context, id int64) error {
	// Get only checks existence: the audit records only the request body and cannot tell which codes the role had;
	// the trade-off is in the core/audit package comment.
	if _, err := uc.repo.Get(ctx, id); err != nil {
		return err
	}
	if err := uc.repo.Delete(ctx, id); err != nil {
		return err
	}
	uc.cache.InvalidateRole(ctx, id)
	return nil
}

// knownPerms keeps the function permission codes still in the registry and drops dead codes that were retired. A
// role's perms are replaced as a whole, so an edit simply clears dead codes instead of failing over a leftover code.
func knownPerms(codes []string) []string {
	reg := perm.All()
	out := make([]string, 0, len(codes))
	for _, c := range codes {
		if _, ok := reg[perm.Code(c)]; ok {
			out = append(out, c)
		}
	}
	return out
}

// validateScopes checks each role×resource scope policy against the resource
// registry and its declared anchors. ctx is for calling the resource's ListGrantable callback when checking
// RESOURCE_GRANT (instances are runtime business data, so the registry cannot answer "does this ID exist").
func validateScopes(ctx context.Context, items []DataScopeInput) error {
	seen := make(map[string]bool, len(items))
	for _, it := range items {
		if it.ResourceType == "" || !validScopeType(it.ScopeType) {
			return ErrInvalidArgument
		}
		// A resource has one policy (uk_role_res); a duplicate is rejected here instead of hitting the key on write.
		if seen[it.ResourceType] {
			return fmt.Errorf("%w: duplicate resource %s", ErrInvalidArgument, it.ResourceType)
		}
		seen[it.ResourceType] = true
		def, ok := perm.ResourceByType(perm.Resource(it.ResourceType))
		if !ok {
			return fmt.Errorf("%w: unknown resource %s", ErrInvalidArgument, it.ResourceType)
		}
		if err := scopeCompatible(ctx, def, it.ScopeType, it.ResourceIDs); err != nil {
			return err
		}
	}
	return nil
}

// scopeCompatible checks whether a scope policy can be configured for a resource: SELF needs AnchorSelf;
// DEPT/ALL_SUB need AnchorDept; RESOURCE_GRANT needs the resource to declare a ListGrantable callback, a non-empty
// instance set and every ID among the options (an empty instance set is a dead policy that always denies, so it
// must not exist); ALL is always valid. A non-GRANT policy must not carry instances. The frontend disables options
// from the same facts, but this is the boundary and does not trust the frontend.
func scopeCompatible(ctx context.Context, def perm.ResourceDef, st ScopeType, resourceIDs []int64) error {
	if st != ScopeGrant && len(resourceIDs) > 0 {
		return fmt.Errorf("%w: scope %s carries resource ids (RESOURCE_GRANT only)", ErrInvalidArgument, st)
	}
	switch st {
	case ScopeSelf:
		if def.Anchors&perm.AnchorSelf == 0 {
			return fmt.Errorf("%w: resource %s has no SELF anchor", ErrInvalidArgument, def.Res)
		}
	case ScopeDept, ScopeAllSub:
		if def.Anchors&perm.AnchorDept == 0 {
			return fmt.Errorf("%w: resource %s has no DEPT anchor", ErrInvalidArgument, def.Res)
		}
	case ScopeGrant:
		if def.ListGrantable == nil {
			return fmt.Errorf("%w: resource %s is not grantable", ErrInvalidArgument, def.Res)
		}
		if len(resourceIDs) == 0 {
			return fmt.Errorf("%w: resource %s grant needs at least one instance", ErrInvalidArgument, def.Res)
		}
		// perm.GrantOptions also enforces the MaxGrantOptions limit: a resource with too many options is refused on
		// the validation side too, by the same rule as the meta configuration side.
		opts, err := perm.GrantOptions(ctx, def)
		if err != nil {
			return err
		}
		known := make(map[int64]struct{}, len(opts))
		for _, o := range opts {
			known[o.ID] = struct{}{}
		}
		for _, id := range resourceIDs {
			if _, ok := known[id]; !ok {
				return fmt.Errorf("%w: resource %s has no grantable instance %d", ErrInvalidArgument, def.Res, id)
			}
		}
	}
	return nil
}
