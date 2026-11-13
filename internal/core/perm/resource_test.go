package perm

import (
	"context"
	"testing"
)

// TestResourcesWithDeptOccupancy: only resources registered with a non-nil
// DeptOccupancy callback are returned — resources without one (most
// AnchorSelf-only resources) are invisible to dept occupancy checks. This is
// the registry-side half of inverting the dept occupancy check (see
// biz.DeptUseCase.Delete, which iterates this slice instead of hardcoding a
// business table name).
func TestResourcesWithDeptOccupancy(t *testing.T) {
	ResetForTest()
	defer ResetForTest()

	withOcc := ResourceDef{
		Res:     Resource("with_occ"),
		Anchors: AnchorDept,
		DeptOccupancy: func(ctx context.Context, deptID int64) (bool, error) {
			return true, nil
		},
	}
	withoutOcc := ResourceDef{
		Res:     Resource("without_occ"),
		Anchors: AnchorSelf,
	}
	MustRegister(nil, nil, []ResourceDef{withOcc, withoutOcc})

	got := ResourcesWithDeptOccupancy()
	if len(got) != 1 {
		t.Fatalf("ResourcesWithDeptOccupancy() len = %d, want 1 (got %+v)", len(got), got)
	}
	if got[0].Res != withOcc.Res {
		t.Fatalf("ResourcesWithDeptOccupancy()[0].Res = %q, want %q", got[0].Res, withOcc.Res)
	}
	occupied, err := got[0].DeptOccupancy(context.Background(), 1)
	if err != nil || !occupied {
		t.Fatalf("DeptOccupancy callback = (%v, %v), want (true, nil)", occupied, err)
	}
}

// TestGrantOptionsLimit checks that GrantOptions enforces MaxGrantOptions: over the limit it errors instead of
// truncating (truncation would have administrators configure grants from incomplete options; an error exposes "the
// resource has crossed the explicit-assignment boundary" at configuration time). Exactly at the limit passes; an
// undeclared callback returns (nil, nil).
func TestGrantOptionsLimit(t *testing.T) {
	mk := func(n int) ResourceDef {
		return ResourceDef{
			Res: Resource("brand_group"),
			ListGrantable: func(context.Context) ([]GrantOption, error) {
				opts := make([]GrantOption, n)
				for i := range opts {
					opts[i] = GrantOption{ID: int64(i + 1)}
				}
				return opts, nil
			},
		}
	}

	if opts, err := GrantOptions(context.Background(), mk(MaxGrantOptions)); err != nil || len(opts) != MaxGrantOptions {
		t.Fatalf("at limit: (%d, %v), want (%d, nil)", len(opts), err, MaxGrantOptions)
	}
	if _, err := GrantOptions(context.Background(), mk(MaxGrantOptions+1)); err == nil {
		t.Fatal("over limit: expected error, got nil")
	}
	if opts, err := GrantOptions(context.Background(), ResourceDef{Res: "no_cb"}); err != nil || opts != nil {
		t.Fatalf("no callback: (%v, %v), want (nil, nil)", opts, err)
	}
}
