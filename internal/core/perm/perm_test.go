package perm

import (
	"maps"
	"strings"
	"testing"
)

// groups declares the test groups, because Seal requires a display name for every code's business domain and module.
func groups(gs ...Group) { MustRegisterGroups(gs) }

var testGroups = []Group{
	{Domain: "ops"},
	{Domain: "ops", Module: "dept"},
	{Domain: "ops", Module: "account"},
	{Domain: "ops", Module: "order"},
	{Domain: "ops", Module: "user"},
	{Domain: "ops", Module: "a"},
}

func TestSeal_ValidRegistrySucceeds(t *testing.T) {
	resetForTest()
	groups(testGroups...)
	MustRegister([]Permission{
		{Code: "ops:dept:view"},
		{Code: "ops:dept:update", Deps: []Code{"ops:dept:view"}},
		{Code: "ops:account:view"},
	}, nil, nil)
	if err := Seal(nil); err != nil {
		t.Fatalf("valid registry should seal cleanly: %v", err)
	}
}

// Every contract operation needs an owner: a binding (possibly with empty Codes) or allow_anonymous. The error lists
// each unowned operation so one startup run reports them all.
func TestSeal_ContractOperationUnowned(t *testing.T) {
	resetForTest()
	groups(testGroups...)
	MustRegister([]Permission{{Code: "ops:dept:view"}}, Bindings{
		"/api.test.v1.DeptService/ListDept": {Codes: []Code{"ops:dept:view"}},
		"/api.test.v1.DeptService/Profile":  {},
	}, nil)
	err := Seal([]string{
		"/api.test.v1.DeptService/ListDept",
		"/api.test.v1.DeptService/Profile",
		"/api.test.v1.DeptService/DeleteDept",
		"/api.test.v1.DeptService/MoveDept",
	})
	if err == nil || !strings.Contains(err.Error(), "perm.operation-unowned") ||
		!strings.Contains(err.Error(), "DeleteDept") || !strings.Contains(err.Error(), "MoveDept") ||
		strings.Contains(err.Error(), "ListDept") {
		t.Fatalf("Seal = %v, want perm.operation-unowned naming DeleteDept and MoveDept only", err)
	}
}

func TestSeal_UnknownDep(t *testing.T) {
	resetForTest()
	groups(testGroups...)
	MustRegister([]Permission{
		{Code: "ops:order:edit", Deps: []Code{"ops:order:view"}},
	}, nil, nil)
	if err := Seal(nil); err == nil {
		t.Fatal("expected unknown-dep error")
	}
}

// Groups are only organisation and do not restrict permission dependencies; Seal and Effective must accept the same
// dependency graph.
func TestSeal_DependencyBoundaries(t *testing.T) {
	for _, tc := range []struct {
		name   string
		target Code
		domain string
		module string
	}{
		{"same module", "ops:order:view", "ops", "order"},
		{"cross module", "ops:user:view", "ops", "user"},
		{"cross domain same module", "merchant:order:view", "merchant", "order"},
		{"cross domain and module", "merchant:finance:view", "merchant", "finance"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			resetForTest()
			groups(testGroups...)
			if tc.domain != "ops" {
				groups(Group{Domain: tc.domain}, Group{Domain: tc.domain, Module: tc.module})
			}
			MustRegister([]Permission{
				{Code: "ops:order:edit", Deps: []Code{tc.target}},
				{Code: tc.target, Deps: []Code{"ops:dept:view"}},
				{Code: "ops:dept:view"},
				{Code: "ops:account:view"},
			}, nil, nil)
			if err := Seal(nil); err != nil {
				t.Fatalf("valid dependency should seal cleanly: %v", err)
			}
			want := Set{"ops:order:edit": true, tc.target: true, "ops:dept:view": true}
			if got := Effective([]Code{"ops:order:edit"}); !maps.Equal(got, want) {
				t.Fatalf("Effective = %v, want %v", got, want)
			}
		})
	}
}

func TestSeal_Cycle(t *testing.T) {
	for _, target := range []Code{"ops:a:x", "ops:a:y", "ops:user:view", "merchant:finance:view"} {
		t.Run(string(target), func(t *testing.T) {
			resetForTest()
			groups(testGroups...)
			groups(Group{Domain: "merchant"}, Group{Domain: "merchant", Module: "finance"})
			permissions := []Permission{{Code: "ops:a:x", Deps: []Code{target}}}
			if target != "ops:a:x" {
				permissions = append(permissions, Permission{Code: target, Deps: []Code{"ops:a:x"}})
			}
			MustRegister(permissions, nil, nil)
			if err := Seal(nil); err == nil {
				t.Fatal("expected cycle error")
			}
		})
	}
}

// TestSeal_RejectsTwoSegmentCode checks that a code must have three segments (business domain:module:action). With a
// variable count the organisation is not unique: "which module does this code belong to" has no answer.
func TestSeal_RejectsTwoSegmentCode(t *testing.T) {
	resetForTest()
	groups(testGroups...)
	MustRegister([]Permission{{Code: "ops:view"}}, nil, nil)
	if err := Seal(nil); err == nil {
		t.Fatal("a two-segment code should be rejected")
	}
}

// TestSeal_RequiresGroupNames checks that a missing group declaration fails at startup instead of the UI silently
// showing a slug.
func TestSeal_RequiresGroupNames(t *testing.T) {
	resetForTest()
	MustRegisterGroups([]Group{{Domain: "ops"}}) // Business domain only; the module is missing.
	MustRegister([]Permission{{Code: "ops:dept:view"}}, nil, nil)
	if err := Seal(nil); err == nil {
		t.Fatal("an undeclared module group should fail")
	}

	resetForTest()
	MustRegisterGroups([]Group{{Domain: "ops", Module: "dept"}}) // The business domain is missing.
	MustRegister([]Permission{{Code: "ops:dept:view"}}, nil, nil)
	if err := Seal(nil); err == nil {
		t.Fatal("an undeclared business-domain group should fail")
	}
}

// TestCodeSegments checks the segment accessors, the only source of the organisation, so no hand-written Module field
// can drift from them.
func TestCodeSegments(t *testing.T) {
	const c Code = "merchant:order:refund"
	if c.Domain() != "merchant" || c.Module() != "order" || c.Action() != "refund" {
		t.Fatalf("wrong segments: %s/%s/%s", c.Domain(), c.Module(), c.Action())
	}
	if c.ModuleKey() != "merchant:order" {
		t.Fatalf("ModuleKey = %s, want merchant:order", c.ModuleKey())
	}
	// A malformed code returns empty strings instead of panicking, because it may be called before Seal.
	const bad Code = "nope"
	if bad.Domain() != "" || bad.Module() != "" || bad.Action() != "" {
		t.Fatal("a malformed code should return empty strings")
	}
}

func TestEffective_DepClosure(t *testing.T) {
	resetForTest()
	groups(testGroups...)
	MustRegister([]Permission{
		{Code: "ops:dept:view"},
		{Code: "ops:dept:update", Deps: []Code{"ops:dept:view"}},
	}, nil, nil)
	got := Effective([]Code{"ops:dept:update"})
	if !got["ops:dept:view"] {
		t.Fatalf("expected dep closure to include ops:dept:view, got %v", got)
	}
	if !got["ops:dept:update"] {
		t.Fatal("expected granted code present")
	}
}

func TestCodeTypeFlow(t *testing.T) {
	resetForTest()
	groups(testGroups...)
	MustRegister([]Permission{{Code: "ops:dept:view"}}, nil, nil)
	const c Code = "ops:dept:view"
	s := Effective([]Code{c})
	if !s.Has(c) {
		t.Fatalf("Effective/Has should work with the typed Code")
	}
}

func TestEffective_UnionAndUnknownIgnored(t *testing.T) {
	resetForTest()
	groups(testGroups...)
	MustRegister([]Permission{
		{Code: "ops:dept:view"},
		{Code: "ops:account:view"},
	}, nil, nil)
	got := Effective([]Code{"ops:dept:view", "ops:account:view", "bogus:x:y"})
	if !got["ops:dept:view"] || !got["ops:account:view"] {
		t.Fatal("union missing entries")
	}
	if got["bogus:x:y"] {
		t.Fatal("unknown code must be ignored")
	}
}
