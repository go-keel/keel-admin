package service_test

import (
	"context"
	"testing"
	"time"

	"github.com/go-keel/keel-admin/internal/core/perm"
	"github.com/go-keel/keel-admin/internal/core/sys"
	"github.com/go-keel/keel-admin/internal/core/sys/service"
)

// TestSysRegistrationSeals guards the sys.Perms/service.Bindings migration:
// the real registration must pass perm.Seal (valid deps, no cycles, every
// binding code known) exactly like it will at boot in the entry cmd's di.go.
func TestSysRegistrationSeals(t *testing.T) {
	// Simulate a user plugging in an ordinary business domain, locking the metadata order "business first, kernel
	// sys last"; the business permissions themselves do not affect the sys.Perms count asserted below.
	perm.MustRegisterGroups([]perm.Group{
		{Domain: "ops", Sort: 10},
		{Domain: "ops", Module: "notice"},
	})
	perm.MustRegisterGroups(sys.Groups)
	perm.MustRegister([]perm.Permission{
		{Code: "ops:notice:view"},
	}, nil, nil)
	perm.MustRegisterKernel(sys.Perms, service.Bindings, sys.Resources)
	if err := perm.Seal(nil); err != nil {
		t.Fatalf("sys.Perms/service.Bindings failed to seal: %v", err)
	}
	meta, err := service.NewMetaService(time.UTC).ListPermissions(context.Background(), nil)
	if err != nil {
		t.Fatalf("ListPermissions: %v", err)
	}
	gotGroups := make([]string, 0, len(meta.Groups))
	for _, g := range meta.Groups {
		gotGroups = append(gotGroups, g.GetDomain()+":"+g.GetModule())
	}
	wantGroups := []string{"ops:", "ops:notice", "sys:", "sys:account", "sys:dept", "sys:audit", "sys:login_log"}
	if len(gotGroups) != len(wantGroups) {
		t.Fatalf("groups = %v, want %v", gotGroups, wantGroups)
	}
	for i := range wantGroups {
		if gotGroups[i] != wantGroups[i] {
			t.Fatalf("groups = %v, want %v", gotGroups, wantGroups)
		}
	}
	// Assigning roles and departments and setting the IP allowlist are part of "edit user" (sys:account:update) with
	// no codes of their own; the audit log and the login log have one view code each (see admin-0017). The role
	// domain is entirely RootOnly and uses no codes.
	if got, want := len(sys.Perms), 14; got != want {
		t.Fatalf("sys.Perms count = %d, want %d (migration should not drop/add codes silently)", got, want)
	}
	// 43 = 38 − 3 (AssignRole/AssignDept/SetIPWhitelist merged into UpdateUser, the single entry point of editing a
	// user, whose handler now checks the finer permission codes field by field)
	//    + 1 DeptService.ListDeptOptions (a picker's option set bound to sys:account:view: the URL is in the
	//      department domain but the bar follows the purpose, see dept.proto)
	//    + 3 AuthService favorites calls (add / remove / reorder, login only)
	//    + 1 AuthService.GetMyAccount (the user's own account details, login only)
	//    + 1 AuditService.GetAudit (audit details, sys:audit:view like the list).
	if got, want := len(service.Bindings), 43; got != want {
		t.Fatalf("service.Bindings count = %d, want %d (migration should not drop/add operations silently)", got, want)
	}
}
