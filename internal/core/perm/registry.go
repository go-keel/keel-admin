package perm

import (
	"fmt"
	"regexp"
	"strings"
	"sync"

	"github.com/go-keel/keel-admin/internal/core/errs"
)

// Bindings maps a kratos operation (e.g. "/api.keel_admin.v1.RoleService/CreateRole")
// to its access requirement. Code-fixed (not proto annotations, not DB).
// Un-listed operations are denied by default.
type Bindings map[string]Requirement

var (
	regMu     sync.Mutex
	regPerms  = map[Code]Permission{}
	regKernel = map[Code]bool{}
	regBinds  = Bindings{}
	regRes    = map[Resource]ResourceDef{}
	regGroups = map[string]Group{}
	sealed    bool
)

// codeRe enforces three segments, `<business domain>:<module>:<action>`, such as "sys:account:create".
//
// Exactly three segments rather than "two or three" so that the code itself is the structure: with a variable count
// the UI would grow branches of uneven depth, and "which module does this code belong to" would have no single
// answer. When a business has only one level of grouping, write the module segment explicitly (such as
// cms:article:view); do not omit it.
var codeRe = regexp.MustCompile(`^[a-z][a-z0-9_]*:[a-z][a-z0-9_]*:[a-z][a-z0-9_]*$`)

// MustRegister registers one business domain's permission declarations.
// Duplicate codes/operations, or any call after Seal, panic — these are
// assembly-time (di.go) programming errors, not runtime conditions.
//
// The "sys:" code prefix is reserved for the kernel batch (see
// MustRegisterKernel); Seal rejects any "sys:"-prefixed code registered here.
func MustRegister(perms []Permission, b Bindings, res []ResourceDef) {
	mustRegister(perms, b, res, false)
}

// MustRegisterKernel registers the kernel's own permission declarations
// (core/sys). Unlike MustRegister, codes registered through this batch are
// permitted to use the reserved "sys:" prefix; non-"sys:" codes registered
// here remain legal too — the kernel batch is simply exempt from the
// reservation check.
func MustRegisterKernel(perms []Permission, b Bindings, res []ResourceDef) {
	mustRegister(perms, b, res, true)
}

func mustRegister(perms []Permission, b Bindings, res []ResourceDef, kernel bool) {
	regMu.Lock()
	defer regMu.Unlock()
	if sealed {
		panic(errs.Rules.New("perm.registered-after-seal", "a permission was registered after perm.Seal", "register every permission, binding and resource in di.go before calling perm.Seal"))
	}
	for _, p := range perms {
		if _, dup := regPerms[p.Code]; dup {
			panic(errs.Rules.New("perm.code-duplicate", fmt.Sprintf("permission code %s is registered twice", p.Code), "keep one Permission per code; register each business module once"))
		}
		regPerms[p.Code] = p
		regKernel[p.Code] = kernel
	}
	for op, r := range b {
		if _, dup := regBinds[op]; dup {
			panic(errs.Rules.New("perm.operation-duplicate", fmt.Sprintf("operation %s is bound twice", op), "bind each RPC operation in exactly one Bindings map"))
		}
		regBinds[op] = r
	}
	for _, d := range res {
		if _, dup := regRes[d.Res]; dup {
			panic(errs.Rules.New("perm.resource-duplicate", fmt.Sprintf("data-scope resource %s is registered twice", d.Res), "register each ResourceDef once"))
		}
		regRes[d.Res] = d
	}
}

// MustRegisterGroups registers the business-domain and module groups.
//
// It is separate from MustRegister because groups are pure organisation metadata with no permission semantics:
// several business modules can hang modules under the same business domain, each declaring only its own part,
// without gathering everything into one call. Declaring the same group twice panics (an assembly-time programming
// error).
func MustRegisterGroups(groups []Group) {
	regMu.Lock()
	defer regMu.Unlock()
	if sealed {
		panic(errs.Rules.New("perm.registered-after-seal", "a permission group was registered after perm.Seal", "register every permission, binding and resource in di.go before calling perm.Seal"))
	}
	for _, g := range groups {
		if g.Domain == "" {
			panic(errs.Rules.New("perm.group-domain-missing", fmt.Sprintf("permission group %+v has no Domain", g), "set Domain on every Group passed to perm.MustRegisterGroups"))
		}
		if _, dup := regGroups[g.key()]; dup {
			panic(errs.Rules.New("perm.group-duplicate", fmt.Sprintf("permission group %s is registered twice", g.key()), "declare each business-domain or module group once"))
		}
		regGroups[g.key()] = g
	}
}

// Seal validates the full registry (code format, deps, cycles, binding
// references, every contract operation owned) and freezes it. Called once from di.go after all domains have
// registered, with ContractOperations(); a non-nil error should abort startup.
//
// contract lists the operations that must each be either bound or annotated allow_anonymous, never both and never
// neither: an unowned operation would answer 403 to everyone, and a binding on an anonymous operation never runs. A
// unit test that registers only the bindings it exercises passes nil.
func Seal(contract []string) error {
	regMu.Lock()
	defer regMu.Unlock()
	for code, p := range regPerms {
		if !codeRe.MatchString(string(code)) {
			return errs.Rules.New("perm.code-format", fmt.Sprintf("permission code %q is not <domain>:<module>:<action>", code), "rename the code to lowercase <domain>:<module>:<action>")
		}
		if strings.HasPrefix(string(code), "sys:") && !regKernel[code] {
			return errs.Rules.New("perm.code-reserved-prefix", fmt.Sprintf("business permission code %q uses the kernel prefix sys:", code), "pick a business domain other than sys")
		}
		if p.RequireOTP {
			return errs.Rules.New("perm.require-otp-manual", fmt.Sprintf("permission %q sets RequireOTP itself", code), "remove it from the Permission and set RequireOTP on the Requirement in its bindings; the permission derives it")
		}
		if _, ok := regGroups[code.Domain()]; !ok {
			return errs.Rules.New("perm.domain-group-missing", fmt.Sprintf("permission %q belongs to business domain %q, which has no group", code, code.Domain()), "declare the domain group with perm.MustRegisterGroups")
		}
		if _, ok := regGroups[code.ModuleKey()]; !ok {
			return errs.Rules.New("perm.module-group-missing", fmt.Sprintf("permission %q belongs to module %q, which has no group", code, code.ModuleKey()), "declare the module group with perm.MustRegisterGroups")
		}
		for _, dep := range p.Deps {
			if _, ok := regPerms[dep]; !ok {
				return errs.Rules.New("perm.dep-unknown", fmt.Sprintf("permission %s depends on %s, which is not registered", code, dep), fmt.Sprintf("register %s or remove it from Deps", dep))
			}
		}
	}
	if err := detectCycle(regPerms); err != nil {
		return err
	}
	for op, r := range regBinds {
		for _, c := range r.Codes {
			if _, ok := regPerms[c]; !ok {
				return errs.Rules.New("perm.binding-unknown-code", fmt.Sprintf("operation %s is bound to permission %s, which is not registered", op, c), "register the permission or fix the code in the binding")
			}
		}
	}
	// Derive RequireOTP: once the bindings are validated (every Codes reference is known), propagate step-up from
	// each operation to the codes it references. It runs here rather than in the loop above because it relies on
	// "every referenced code exists".
	for _, r := range regBinds {
		if !r.RequireOTP {
			continue
		}
		for _, c := range r.Codes {
			p := regPerms[c]
			p.RequireOTP = true
			regPerms[c] = p
		}
	}
	for res, d := range regRes {
		if d.Via == nil {
			continue
		}
		parent, ok := regRes[d.Via.Parent]
		if !ok {
			return errs.Rules.New("perm.resource-parent-unknown", fmt.Sprintf("resource %q derives its scope from %q, which is not registered", res, d.Via.Parent), "register the parent resource or fix Via.Parent")
		}
		if parent.Via != nil {
			return errs.Rules.New("perm.resource-nested-derivation", fmt.Sprintf("resource %q derives from %q, which itself derives from another resource", res, d.Via.Parent), "derive from a resource that has its own scope; only one Via level is allowed")
		}
	}
	if err := checkOwnership(contract); err != nil {
		return err
	}
	sealed = true
	return nil
}

// checkOwnership reports every contract operation that is neither bound nor anonymous, and every one that is both.
func checkOwnership(contract []string) error {
	var unowned, both []string
	for _, op := range contract {
		_, bound := regBinds[op]
		anon := IsAnonymous(op)
		switch {
		case !bound && !anon:
			unowned = append(unowned, op)
		case bound && anon:
			both = append(both, op)
		}
	}
	if len(unowned) > 0 {
		return errs.Rules.New("perm.operation-unowned", fmt.Sprintf("operations with neither a binding nor allow_anonymous: %s", strings.Join(unowned, ", ")), "add each to the module's perm.Bindings (Codes, or empty Codes for any signed-in user), or annotate it (keel.v1.auth).allow_anonymous in the proto")
	}
	if len(both) > 0 {
		return errs.Rules.New("perm.operation-anonymous-bound", fmt.Sprintf("operations bound in perm.Bindings and also annotated allow_anonymous: %s", strings.Join(both, ", ")), "remove the binding (anonymous operations skip authorization, so it never runs) or remove allow_anonymous")
	}
	return nil
}

// All returns the registered permission set (read-only usage expected).
func All() map[Code]Permission { return regPerms }

// Groups returns the registered business-domain and module group metadata.
func Groups() []Group {
	out := make([]Group, 0, len(regGroups))
	for _, g := range regGroups {
		out = append(out, g)
	}
	return out
}

// OperationRequirement returns the access requirement for an operation.
// ok=false means the operation has no binding (deny by default).
func OperationRequirement(op string) (Requirement, bool) { r, ok := regBinds[op]; return r, ok }

// Audited returns every operation registered with Audit: true (order
// unspecified). The completeness test of the audit action-name entries iterates over it.
func Audited() []string {
	ops := make([]string, 0, len(regBinds))
	for op, r := range regBinds {
		if r.Audit {
			ops = append(ops, op)
		}
	}
	return ops
}

func detectCycle(reg map[Code]Permission) error {
	const (
		white = 0
		gray  = 1
		black = 2
	)
	color := make(map[Code]int, len(reg))
	var visit func(code Code) error
	visit = func(code Code) error {
		color[code] = gray
		for _, dep := range reg[code].Deps {
			switch color[dep] {
			case gray:
				return errs.Rules.New("perm.dep-cycle", fmt.Sprintf("permission dependencies form a cycle at %s -> %s", code, dep), "remove one of the Deps on the cycle")
			case white:
				if err := visit(dep); err != nil {
					return err
				}
			}
		}
		color[code] = black
		return nil
	}
	for code := range reg {
		if color[code] == white {
			if err := visit(code); err != nil {
				return err
			}
		}
	}
	return nil
}

// ResetForTest is a test-only hook to clear the registry between test
// cases. Exported (unlike this package's own resetForTest) because
// the entry cmd's kernel+business registration smoke test needs to clear the
// global registry from outside package perm — see captcha.ResetForTest for
// the established precedent of exporting a reset hook for cross-package
// registration tests.
func ResetForTest() {
	regMu.Lock()
	defer regMu.Unlock()
	regPerms = map[Code]Permission{}
	regKernel = map[Code]bool{}
	regBinds = Bindings{}
	regRes = map[Resource]ResourceDef{}
	regGroups = map[string]Group{}
	sealed = false
}

// resetForTest is package-internal sugar for this package's own tests.
func resetForTest() { ResetForTest() }
