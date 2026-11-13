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

func TestSealAcceptsOneLevelDerive(t *testing.T) {
	resetForTest()
	MustRegister(nil, nil, []ResourceDef{
		{Res: "order", Anchors: AnchorSelf},
		{Res: "order_item", Via: &Derive{Parent: "order", FK: "order_id"}},
	})
	if err := Seal(nil); err != nil {
		t.Fatalf("one level of derivation should pass: %v", err)
	}
}

func TestSealRejectsDeriveFromUnknownParent(t *testing.T) {
	resetForTest()
	MustRegister(nil, nil, []ResourceDef{
		{Res: "order_item", Via: &Derive{Parent: "ghost", FK: "order_id"}},
	})
	if err := Seal(nil); err == nil {
		t.Fatal("deriving from an unregistered resource should fail")
	}
}

func TestSealRejectsTwoLevelDerive(t *testing.T) {
	resetForTest()
	MustRegister(nil, nil, []ResourceDef{
		{Res: "order", Anchors: AnchorSelf},
		{Res: "order_item", Via: &Derive{Parent: "order", FK: "order_id"}},
		{Res: "order_item_note", Via: &Derive{Parent: "order_item", FK: "item_id"}},
	})
	if err := Seal(nil); err == nil {
		t.Fatal("two levels of derivation should be rejected (Via allows one level only)")
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
// reads), so Permission.RequireOTP must be derived from it rather than written by hand; otherwise the two declarations
// drift (the binding marks step-up while the Permission does not).
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
	if All()["ops:demo:danger"].RequireOTP != true {
		t.Fatal("a code referenced by a RequireOTP operation should derive RequireOTP")
	}
	if All()["ops:demo:view"].RequireOTP != false {
		t.Fatal("a code referenced by no step-up operation should not carry RequireOTP")
	}
}

// TestSealRejectsHandWrittenRequireOTP checks that a hand-written value is rejected. Allowing it would let it disagree
// with the bindings, and the UI would see only the hand-written one.
func TestSealRejectsHandWrittenRequireOTP(t *testing.T) {
	resetForTest()
	MustRegisterGroups(demoGroups)
	MustRegister([]Permission{
		{Code: "ops:demo:view", RequireOTP: true},
	}, nil, nil)
	if err := Seal(nil); err == nil {
		t.Fatal("a hand-written Permission.RequireOTP should be rejected by Seal")
	}
}
