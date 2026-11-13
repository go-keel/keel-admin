package biz

import (
	"context"
	"fmt"

	"github.com/go-keel/keel-admin/internal/core/perm"
)

// Dept is a department tree node.
type Dept struct {
	ID        int64
	ParentID  int64
	Ancestors string // materialized path "0,1,5,"
	Name      string
	Sort      int
	Status    int8
	// UserCount is filled only by List (one GROUP BY for the batch), so the interface knows whether a department
	// can be deleted before the delete is clicked.
	UserCount int
}

// DeptRepo is the department persistence port.
type DeptRepo interface {
	List(ctx context.Context) ([]*Dept, error)
	Get(ctx context.Context, id int64) (*Dept, error)
	Create(ctx context.Context, d *Dept) error
	Update(ctx context.Context, d *Dept) error
	Delete(ctx context.Context, id int64) error
	HasChildren(ctx context.Context, id int64) (bool, error)
	HasUsers(ctx context.Context, id int64) (bool, error)
}

// DeptUseCase manages the department tree under ARBAC management boundaries.
type DeptUseCase struct {
	repo  DeptRepo
	arbac *ARBACUseCase
}

// NewDeptUseCase builds the dept use case.
func NewDeptUseCase(repo DeptRepo, arbac *ARBACUseCase) *DeptUseCase {
	return &DeptUseCase{repo: repo, arbac: arbac}
}

// List filters by the actor's manageable departments (everything for root), the same range rule as the user list,
// so nothing is "visible but not editable". No manageable departments means an empty set. A non-root actor gets
// only its manageable subtrees, so a mid-level department shows up as a tree root (its ancestors are not in the
// result); that is acceptable.
func (uc *DeptUseCase) List(ctx context.Context, actor *Actor) ([]*Dept, error) {
	all, err := uc.repo.List(ctx)
	if err != nil {
		return nil, err
	}
	ids, isAll, err := uc.arbac.ManageableDeptIDs(ctx, actor)
	if err != nil {
		return nil, err
	}
	if isAll {
		return all, nil
	}
	if len(ids) == 0 {
		return []*Dept{}, nil
	}
	set := toSet(ids)
	out := make([]*Dept, 0, len(ids))
	for _, d := range all {
		if set[d.ID] {
			out = append(out, d)
		}
	}
	return out, nil
}

// Create checks the management boundary against parentID: the actor must be able to manage the parent department
// (root passes inside the check).
func (uc *DeptUseCase) Create(ctx context.Context, actor *Actor, parentID int64, name string, sort int) (*Dept, error) {
	// Only root creates a root department (parentID=0): the top level hangs under no manageable department, so no
	// non-root actor can reach that position. A non-root actor creates departments under a parent it can manage.
	if parentID == 0 {
		if !actor.IsRoot {
			return nil, ErrNotRoot
		}
	} else if ok, err := uc.arbac.CanManageDept(ctx, actor, parentID); err != nil {
		return nil, err
	} else if !ok {
		return nil, ErrDeptOutOfRange
	}

	ancestors := "0,"
	if parentID != 0 {
		parent, err := uc.repo.Get(ctx, parentID)
		if err != nil {
			return nil, ErrDeptNotFound
		}
		ancestors = fmt.Sprintf("%s%d,", parent.Ancestors, parent.ID)
	}
	d := &Dept{ParentID: parentID, Ancestors: ancestors, Name: name, Sort: sort, Status: 1}
	if err := uc.repo.Create(ctx, d); err != nil {
		return nil, err
	}
	return d, nil
}

// Get is the source of the edit form's initial values: it admits only departments the actor can manage (the same
// range as the list).
func (uc *DeptUseCase) Get(ctx context.Context, actor *Actor, id int64) (*Dept, error) {
	if ok, err := uc.arbac.CanManageDept(ctx, actor, id); err != nil {
		return nil, err
	} else if !ok {
		return nil, ErrDeptOutOfRange
	}
	d, err := uc.repo.Get(ctx, id)
	if err != nil {
		return nil, ErrDeptNotFound
	}
	return d, nil
}

// Update checks the management boundary against the department's own ID. The organization tree does not support
// moving a department (changing its parent): proto UpdateDeptRequest has no parent_id field, and reparenting would
// need its own migration and endpoint (see docs/admin/permission-model/management-boundary.md §6).
func (uc *DeptUseCase) Update(ctx context.Context, actor *Actor, id int64, name string, sort int, status int8) (*Dept, error) {
	if ok, err := uc.arbac.CanManageDept(ctx, actor, id); err != nil {
		return nil, err
	} else if !ok {
		return nil, ErrDeptOutOfRange
	}

	d, err := uc.repo.Get(ctx, id)
	if err != nil {
		return nil, ErrDeptNotFound
	}
	d.Name = name
	d.Sort = sort
	d.Status = status
	if err := uc.repo.Update(ctx, d); err != nil {
		return nil, err
	}
	return d, nil
}

// Delete checks the management boundary against the department's own ID. Only an empty department can be deleted,
// and the delete only writes deleted_at. The order is the semantics: first the permission boundary, then child
// departments, users and resource ownership in turn; any non-empty one rejects (always ErrDeptNotEmpty).
// The resource ownership check hardcodes no business table name: it calls, one by one, the DeptOccupancy callbacks
// that business domains register at startup, as listed by perm.ResourcesWithDeptOccupancy(). dept (kernel
// core/sys) depends only on the perm package's registry interface and imports no business package (dependency
// direction in the doc of perm.ResourceDef.DeptOccupancy).
func (uc *DeptUseCase) Delete(ctx context.Context, actor *Actor, id int64) error {
	if ok, err := uc.arbac.CanManageDept(ctx, actor, id); err != nil {
		return err
	} else if !ok {
		return ErrDeptOutOfRange
	}

	if hc, err := uc.repo.HasChildren(ctx, id); err != nil {
		return err
	} else if hc {
		return ErrDeptNotEmpty
	}
	if hu, err := uc.repo.HasUsers(ctx, id); err != nil {
		return err
	} else if hu {
		return ErrDeptNotEmpty
	}
	for _, res := range perm.ResourcesWithDeptOccupancy() {
		occupied, err := res.DeptOccupancy(ctx, id)
		if err != nil {
			return err
		}
		if occupied {
			return ErrDeptNotEmpty
		}
	}
	return uc.repo.Delete(ctx, id)
}
