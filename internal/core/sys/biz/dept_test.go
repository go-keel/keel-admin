package biz

import (
	"context"
	"errors"
	"testing"
)

// fakeDeptRepo is a hand-written fake: biz tests depend only on the repo interface and need no infrastructure.
type fakeDeptRepo struct {
	depts    map[int64]*Dept
	nextID   int64
	children map[int64]bool
	users    map[int64]bool
}

var _ DeptRepo = (*fakeDeptRepo)(nil)

func newFakeDeptRepo() *fakeDeptRepo {
	return &fakeDeptRepo{
		depts:    map[int64]*Dept{},
		nextID:   1,
		children: map[int64]bool{},
		users:    map[int64]bool{},
	}
}

func (f *fakeDeptRepo) List(context.Context) ([]*Dept, error) { return nil, nil }
func (f *fakeDeptRepo) Get(_ context.Context, id int64) (*Dept, error) {
	d, ok := f.depts[id]
	if !ok {
		return nil, errors.New("record not found")
	}
	return d, nil
}
func (f *fakeDeptRepo) Create(_ context.Context, d *Dept) error {
	d.ID = f.nextID
	f.nextID++
	f.depts[d.ID] = d
	return nil
}
func (f *fakeDeptRepo) Update(_ context.Context, d *Dept) error { f.depts[d.ID] = d; return nil }
func (f *fakeDeptRepo) Delete(_ context.Context, id int64) error {
	delete(f.depts, id)
	return nil
}
func (f *fakeDeptRepo) HasChildren(_ context.Context, id int64) (bool, error) {
	return f.children[id], nil
}
func (f *fakeDeptRepo) HasUsers(_ context.Context, id int64) (bool, error) {
	return f.users[id], nil
}

// deptOccupancyTestFlag backs the "fake_dept_resource" entry TestMain
// (role_test.go) registers once for the whole package — perm's registry is
// sealed a single time for this test binary, so individual tests can't
// perm.ResetForTest()/MustRegister a resource of their own without breaking
// every other test that depends on the sealed registry (e.g. role_test.go's
// data-scope validation tests). Toggling this flag instead of touching the registry
// is how dept-layer tests drive perm.ResourcesWithDeptOccupancy's outcome.
var deptOccupancyTestFlag bool

// setDeptOccupancy toggles deptOccupancyTestFlag for the duration of the
// test — this is how dept-layer tests exercise the occupancy check now that
// it's dependency-inverted through perm.ResourcesWithDeptOccupancy instead
// of a DeptRepo method.
func setDeptOccupancy(t *testing.T, occupied bool) {
	t.Helper()
	deptOccupancyTestFlag = occupied
	t.Cleanup(func() { deptOccupancyTestFlag = false })
}

// rootActor / boundedActor: dept tests all go through the real ARBACUseCase + fakeARBACRepoDept (the same
// fixture as user_admin_arbac_test.go) and add no extra mock interface.
func rootActor() *Actor { return &Actor{UserID: 1, IsRoot: true} }

func newDeptUC(repo DeptRepo, manageableDeptIDs []int64) *DeptUseCase {
	arbac := NewARBACUseCase(&fakeARBACRepoDept{deptIDs: manageableDeptIDs}, nil)
	return NewDeptUseCase(repo, arbac)
}

func TestDeptUseCase_Create_AncestorsPath(t *testing.T) {
	repo := newFakeDeptRepo()
	uc := newDeptUC(repo, nil)
	ctx := context.Background()
	actor := rootActor()

	root, err := uc.Create(ctx, actor, 0, "Root", 0)
	if err != nil {
		t.Fatalf("create root: %v", err)
	}
	if root.Ancestors != "0," {
		t.Fatalf("root ancestors = %q, want \"0,\"", root.Ancestors)
	}

	child, err := uc.Create(ctx, actor, root.ID, "Child", 0)
	if err != nil {
		t.Fatalf("create child: %v", err)
	}
	if want := "0,1,"; child.Ancestors != want {
		t.Fatalf("child ancestors = %q, want %q", child.Ancestors, want)
	}
}

func TestDeptUseCase_Create_ParentNotFound(t *testing.T) {
	uc := newDeptUC(newFakeDeptRepo(), nil)
	if _, err := uc.Create(context.Background(), rootActor(), 404, "X", 0); !errors.Is(err, ErrDeptNotFound) {
		t.Fatalf("expected ErrDeptNotFound, got %v", err)
	}
}

func TestDeptUseCase_Delete_Guards(t *testing.T) {
	repo := newFakeDeptRepo()
	uc := newDeptUC(repo, nil)
	ctx := context.Background()
	actor := rootActor()

	setDeptOccupancy(t, false)

	d, _ := uc.Create(ctx, actor, 0, "D", 0)

	repo.children[d.ID] = true
	if err := uc.Delete(ctx, actor, d.ID); !errors.Is(err, ErrDeptNotEmpty) {
		t.Fatalf("expected ErrDeptNotEmpty (children), got %v", err)
	}

	repo.children[d.ID] = false
	repo.users[d.ID] = true
	if err := uc.Delete(ctx, actor, d.ID); !errors.Is(err, ErrDeptNotEmpty) {
		t.Fatalf("expected ErrDeptNotEmpty (users), got %v", err)
	}

	repo.users[d.ID] = false
	setDeptOccupancy(t, true)
	if err := uc.Delete(ctx, actor, d.ID); !errors.Is(err, ErrDeptNotEmpty) {
		t.Fatalf("expected ErrDeptNotEmpty (resources, via perm.ResourcesWithDeptOccupancy), got %v", err)
	}

	setDeptOccupancy(t, false)
	if err := uc.Delete(ctx, actor, d.ID); err != nil {
		t.Fatalf("delete should pass: %v", err)
	}
}

// TestDeptCreateOutOfRange: a non-root actor whose manageable departments do not include the target parent
// department gets ErrDeptOutOfRange (Create's manageable-boundary target is parentID).
func TestDeptCreateOutOfRange(t *testing.T) {
	uc := newDeptUC(newFakeDeptRepo(), nil) // the manageable set is empty
	actor := &Actor{UserID: 7, RoleIDs: []int64{2}}
	if _, err := uc.Create(context.Background(), actor, 2, "x", 0); !errors.Is(err, ErrDeptOutOfRange) {
		t.Fatalf("a parent department out of range should be rejected: %v", err)
	}
}

// TestDeptUpdateOutOfRange: Update's manageable-boundary target is the department's own id.
func TestDeptUpdateOutOfRange(t *testing.T) {
	uc := newDeptUC(newFakeDeptRepo(), nil)
	actor := &Actor{UserID: 7, RoleIDs: []int64{2}}
	if _, err := uc.Update(context.Background(), actor, 2, "x", 0, 1); !errors.Is(err, ErrDeptOutOfRange) {
		t.Fatalf("a department out of range should be rejected: %v", err)
	}
}

// TestDeptDeleteOutOfRange: the ARBAC boundary comes before the emptiness check: out of range is rejected even
// when the department is empty.
func TestDeptDeleteOutOfRange(t *testing.T) {
	uc := newDeptUC(newFakeDeptRepo(), nil)
	actor := &Actor{UserID: 7, RoleIDs: []int64{2}}
	if err := uc.Delete(context.Background(), actor, 2); !errors.Is(err, ErrDeptOutOfRange) {
		t.Fatalf("a department out of range should be rejected: %v", err)
	}
}

// TestDeptDeleteNonEmpty: within the manageable range but the department still has users, so ErrDeptNotEmpty.
func TestDeptDeleteNonEmpty(t *testing.T) {
	repo := newFakeDeptRepo()
	repo.users[2] = true
	uc := newDeptUC(repo, []int64{2})
	actor := &Actor{UserID: 7, RoleIDs: []int64{2}}
	if err := uc.Delete(context.Background(), actor, 2); !errors.Is(err, ErrDeptNotEmpty) {
		t.Fatalf("deleting a non-empty department should be rejected: %v", err)
	}
}
