package main

import (
	"testing"

	goi18n "github.com/nicksnyder/go-i18n/v2/i18n"

	nbiz "github.com/go-keel/keel-admin/internal/biz"
	"github.com/go-keel/keel-admin/internal/core/audit"
	"github.com/go-keel/keel-admin/internal/core/perm"
	"github.com/go-keel/keel-admin/internal/core/sys"
	"github.com/go-keel/keel-admin/internal/core/sys/service"
	"github.com/go-keel/keel-admin/internal/i18n"
	"github.com/go-keel/keel/pkg/redact"
)

// registerAll runs di.go's own registration.
func registerAll() {
	// Example: delete after forking (the argument only). A nil repo is safe: Seal never calls DeptOccupancy.
	registerPermissions(nil)
}

// registerKernel registers only the kernel batch: message completeness is checked only for the kernel; whether
// business entries are fully translated is the derived repo's own concern.
func registerKernel() {
	registerKernelPermissions()
}

// TestKernelAndBusinessRegistrationSeals runs the real kernel and business declarations through perm.Seal
// together. The unit tests of perm and of each module test only one side, so a binding with a wrong code
// (unregistered, cross-module dependency) would still pass `go test ./...` and panic only when the binary starts.
func TestKernelAndBusinessRegistrationSeals(t *testing.T) {
	t.Run("real kernel+business composition seals cleanly", func(t *testing.T) {
		perm.ResetForTest()
		defer perm.ResetForTest()

		registerAll()
		if err := perm.Seal(perm.ContractOperations()); err != nil {
			t.Fatalf("Seal on the real di.go registration order = %v, want nil", err)
		}
	})

	// Example: delete after forking (the whole subtest)
	t.Run("business binding referencing an unregistered code is rejected", func(t *testing.T) {
		perm.ResetForTest()
		defer perm.ResetForTest()

		perm.MustRegisterGroups(sys.Groups)
		perm.MustRegisterGroups(nbiz.NoticeGroups)
		perm.MustRegisterKernel(sys.Perms, service.Bindings, sys.Resources)
		// Same shape as nsvc.NoticeBindings, but one binding uses a code NoticePerms does not register: a mistyped
		// code or a missed rename must fail here in Seal, not compile and then panic at startup.
		brokenNoticeBindings := perm.Bindings{
			"/api.keel_admin.v1.NoticeService/ListNotice": {Codes: []perm.Code{"notice:ghost"}},
		}
		perm.MustRegister(nbiz.NoticePerms, brokenNoticeBindings, nbiz.NoticeResourceDefs(nil))

		if err := perm.Seal(nil); err == nil {
			t.Fatal("Seal with a NoticeBindings entry referencing an unregistered code = nil, want error")
		}
	})
}

// TestPermissionRegistryHasLocalizedNames locks the convention for permission tree display names: every code, group
// and data scope resource the kernel registers needs an entry in both sys.en.toml and sys.zh-CN.toml. The display
// name's i18n key derives from the slug (perm.Code.NameKey / perm.Group.NameKey), so a missing entry is the only
// failure mode; a missing one fails here instead of the UI silently showing a slug.
//
// Asserting per language works for the same reason as in the audit test below: go-i18n always returns
// MessageNotFoundErr for "this language lacks the entry", even though it can fall back to the default language.
func TestPermissionRegistryHasLocalizedNames(t *testing.T) {
	perm.ResetForTest()
	defer perm.ResetForTest()

	registerKernel()

	ids := make([]string, 0)
	for code := range perm.All() {
		ids = append(ids, code.NameKey())
	}
	for _, g := range perm.Groups() {
		ids = append(ids, g.NameKey())
	}
	// Resource display names use the hand-written NameKey field (not derived); empty means the frontend falls back to
	// the slug. Every resource registered here fills it in, and this locks that too, so a new resource does not forget
	// its entry.
	for _, d := range perm.Resources() {
		if d.NameKey == "" {
			t.Errorf("resource %q declares no NameKey; the permission configuration UI can only show the slug", d.Res)
			continue
		}
		ids = append(ids, d.NameKey)
	}

	bundle := i18n.NewBundle()
	for _, lang := range []string{"en", "zh-CN"} {
		loc := goi18n.NewLocalizer(bundle.Bundle, lang)
		for _, id := range ids {
			if _, err := loc.Localize(&goi18n.LocalizeConfig{MessageID: id}); err != nil {
				t.Errorf("lang %s: missing i18n entry [%s] in internal/i18n/sys.%s.toml", lang, id, lang)
			}
		}
	}
}

// TestAuditedOperationsHaveLocalizedNames locks the convention for audit display names: every Audit: true operation of
// the kernel needs audit.act.* (action) and audit.tt.* (target type) entries in both sys.en.toml and sys.zh-CN.toml;
// a missing one fails here instead of the UI showing a bare "XxxService.DoThing".
// Asserting per language works because go-i18n always returns MessageNotFoundErr for "this language lacks the entry",
// even though it can fall back to the default language (the runtime fallback is a feature; entry completeness is
// guaranteed by this test).
func TestAuditedOperationsHaveLocalizedNames(t *testing.T) {
	perm.ResetForTest()
	defer perm.ResetForTest()

	registerKernel()

	bundle := i18n.NewBundle()
	for _, lang := range []string{"en", "zh-CN"} {
		loc := goi18n.NewLocalizer(bundle.Bundle, lang)
		for _, op := range perm.Audited() {
			ids := []string{
				audit.ActionNameKey(audit.ActionOf(op)),
				audit.TargetNameKey(audit.TargetOf(op)),
			}
			for _, id := range ids {
				if _, err := loc.Localize(&goi18n.LocalizeConfig{MessageID: id}); err != nil {
					t.Errorf("lang %s: audited op %s is missing i18n entry [%s] in internal/i18n/sys.%s.toml",
						lang, op, id, lang)
				}
			}
		}
	}
}

// TestRedactAnnotationsValidate moves Keel's startup check of redaction annotations (run by mw.Default) forward to
// CI: a field annotated on the wrong type aborts startup, which means a deployment rollback; this test fails at commit
// time instead.
//
// In this process the global registry already holds every proto of this repo (registered by the generated code's
// init); the api package is imported transitively by the tests above.
func TestRedactAnnotationsValidate(t *testing.T) {
	if err := redact.Validate(); err != nil {
		t.Fatalf("invalid redact annotation in proto; the service would refuse to start: %v", err)
	}
}
