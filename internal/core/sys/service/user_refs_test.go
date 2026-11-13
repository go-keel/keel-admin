package service

import (
	"context"
	"errors"
	"slices"
	"testing"

	"github.com/go-keel/keel-admin/internal/core/sys/biz"
)

// refsUserRepo implements only the methods resolveUserRefs uses; any other method panics through the embedded nil
// interface.
type refsUserRepo struct {
	biz.UserAdminRepo
	calls  int
	ids    []int64
	roles  map[int64][]biz.UserRole
	forced map[int64]bool
	err    error
}

func (f *refsUserRepo) ForceOTPUsers(context.Context, []int64) (map[int64]bool, error) {
	return f.forced, nil
}

func (f *refsUserRepo) RolesOfUsers(_ context.Context, ids []int64) (map[int64][]biz.UserRole, error) {
	f.calls++
	f.ids = ids
	return f.roles, f.err
}

type refsRefRepo struct {
	biz.RefRepo
	calls int
	ids   []int64
	names map[int64]string
}

func (f *refsRefRepo) DeptNames(_ context.Context, ids []int64) (map[int64]string, error) {
	f.calls++
	f.ids = ids
	return f.names, nil
}

// Several users on one page: roles and department names are queried once each, department ids are deduplicated,
// and the result matches resolving one by one.
func TestResolveUserRefs_OneQueryPerPage(t *testing.T) {
	users := &refsUserRepo{roles: map[int64][]biz.UserRole{
		1: {{ID: 3, Name: "R3"}, {ID: 7, Name: "R7"}},
		2: {{ID: 7, Name: "R7"}},
	}, forced: map[int64]bool{1: true}}
	refs := &refsRefRepo{names: map[int64]string{10: "R&D", 20: "Ops"}}
	s := &UserService{repo: users, ref: refs}

	got, err := s.resolveUserRefs(context.Background(), []*biz.User{
		{ID: 1, DeptID: 10}, {ID: 2, DeptID: 20}, {ID: 3, DeptID: 10}, {ID: 4},
	})
	if err != nil {
		t.Fatalf("resolveUserRefs: %v", err)
	}
	if users.calls != 1 || refs.calls != 1 {
		t.Fatalf("want one roles query and one dept-name query, got %d and %d", users.calls, refs.calls)
	}
	if !slices.Equal(users.ids, []int64{1, 2, 3, 4}) || !slices.Equal(refs.ids, []int64{10, 20}) {
		t.Fatalf("query ids: users %v, depts %v", users.ids, refs.ids)
	}
	if r := got[1]; len(r.roles) != 2 || r.roles[0].GetName() != "R3" || !r.otpForced || r.deptName != "R&D" {
		t.Fatalf("user 1 = %+v", r)
	}
	if r := got[2]; len(r.roles) != 1 || r.otpForced || r.deptName != "Ops" {
		t.Fatalf("user 2 = %+v", r)
	}
	if r := got[3]; len(r.roles) != 0 || r.deptName != "R&D" {
		t.Fatalf("user 3 = %+v", r)
	}
	if r := got[4]; len(r.roles) != 0 || r.deptName != "" {
		t.Fatalf("user 4 = %+v", r)
	}
}

// A query error fails the whole page instead of returning "no roles" as if it were true.
func TestResolveUserRefs_PropagatesError(t *testing.T) {
	boom := errors.New("db down")
	s := &UserService{repo: &refsUserRepo{err: boom}, ref: &refsRefRepo{}}
	if _, err := s.resolveUserRefs(context.Background(), []*biz.User{{ID: 1}}); !errors.Is(err, boom) {
		t.Fatalf("want %v, got %v", boom, err)
	}
}
