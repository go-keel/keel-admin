package perm

import "testing"

// demoGroups exists because Seal requires a display name for every code's business domain and module.
var demoGroups = []Group{
	{Domain: "ops"},
	{Domain: "ops", Module: "demo"},
	{Domain: "ops", Module: "ghost"},
}

func TestRegistryLifecycle(t *testing.T) {
	resetForTest() // Test hook: empties the registry.
	MustRegisterGroups(demoGroups)
	MustRegister([]Permission{{Code: "ops:demo:view"}},
		Bindings{"/x.v1.Demo/View": {Codes: []Code{"ops:demo:view"}}}, nil)
	if err := Seal(nil); err != nil {
		t.Fatalf("Seal: %v", err)
	}
	if _, ok := OperationRequirement("/x.v1.Demo/View"); !ok {
		t.Fatal("binding not registered")
	}
	defer func() {
		if recover() == nil {
			t.Fatal("registering after Seal should panic")
		}
	}()
	MustRegister([]Permission{{Code: "ops:demo:x"}}, nil, nil)
}

func TestSealRejectsUnknownBindingCode(t *testing.T) {
	resetForTest()
	MustRegister(nil, Bindings{"/x.v1.Demo/View": {Codes: []Code{"ops:ghost:code"}}}, nil)
	if err := Seal(nil); err == nil {
		t.Fatal("referencing an unregistered code should fail")
	}
}

func TestSysPrefixReservedForKernel(t *testing.T) {
	resetForTest()
	MustRegisterGroups([]Group{{Domain: "sys"}, {Domain: "sys", Module: "evil"}})
	MustRegister([]Permission{{Code: "sys:evil:x"}}, nil, nil)
	if err := Seal(nil); err == nil {
		t.Fatal("a business batch registering a sys: code should be rejected by Seal")
	}
	resetForTest()
	MustRegisterGroups([]Group{{Domain: "sys"}, {Domain: "sys", Module: "dept"}})
	MustRegisterKernel([]Permission{{Code: "sys:dept:view"}}, nil, nil)
	if err := Seal(nil); err != nil {
		t.Fatalf("a kernel batch with sys: codes should pass: %v", err)
	}
}

// TestSealDerivesRequireOTPFromBindings locks the derivation: step-up is a fact on Requirement (the only place authz
// reads), and RequiresOTP is derived from it, so the marker the UI shows cannot drift from the real decision.
func TestSealDerivesRequireOTPFromBindings(t *testing.T) {
	resetForTest()
	MustRegisterGroups(demoGroups)
	MustRegister([]Permission{
		{Code: "ops:demo:view"},
		{Code: "ops:demo:danger"},
	}, Bindings{
		"/x.v1.Demo/View":   {Codes: []Code{"ops:demo:view"}},
		"/x.v1.Demo/Danger": {Codes: []Code{"ops:demo:danger"}, RequireOTP: true},
	}, nil)
	if err := Seal(nil); err != nil {
		t.Fatalf("Seal: %v", err)
	}
	if !RequiresOTP("ops:demo:danger") {
		t.Fatal("a code referenced by a RequireOTP operation should derive RequireOTP")
	}
	if RequiresOTP("ops:demo:view") {
		t.Fatal("a code referenced by no step-up operation should not carry RequireOTP")
	}
}
