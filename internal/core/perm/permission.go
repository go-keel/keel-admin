package perm

import "strings"

// Code is a stable, code-fixed permission code.
//
// It always has three segments, `<business domain>:<module>:<action>`, such as "sys:account:create". The three
// segments are the three levels of organisation, and Seal enforces them with codeRe. **The code itself is the
// structure**, so there are no separate Domain/Module fields: a hand-written field can disagree with the code and
// nothing would check it, while derived segments cannot drift.
//
// Business domains and modules **are only groups, not permissions**: they cannot be granted and have no Permission in
// the registry. They are registered only as a Group, which the UI uses as a container with select-all. Only leaves
// can be granted.
type Code string

// Domain returns the code's first segment (the business domain), "" if malformed.
func (c Code) Domain() string { return segment(c, 0) }

// Module returns the code's second segment (the module), "" if malformed.
func (c Code) Module() string { return segment(c, 1) }

// Action returns the code's third segment (the action), "" if malformed.
func (c Code) Action() string { return segment(c, 2) }

// ModuleKey returns the module's identity: business domain + module.
//
// A module's identity must include its business domain, because "merchant:finance" and "user:finance" are different
// groups. Group identity does not restrict dependencies between permission codes.
func (c Code) ModuleKey() string { return c.Domain() + ":" + c.Module() }

// NameKey is the i18n message id for this code's display name.
//
// The key is derived from the code ("sys:dept:view" → "perm.sys.dept.view") and there is no display-name field: a
// literal name could only be in one language (an English UI would still show Chinese), and one more hand-written
// field could disagree with the code. It has the same shape as audit.ActionNameKey. The entries live in
// internal/i18n/*.toml, and their completeness is guarded by the registration test under the entry cmd.
func (c Code) NameKey() string { return nameKey(string(c)) }

// nameKey turns a ":"-separated registration key into the "."-separated i18n namespace.
func nameKey(key string) string { return "perm." + strings.ReplaceAll(key, ":", ".") }

func segment(c Code, i int) string {
	parts := strings.Split(string(c), ":")
	if len(parts) != 3 {
		return ""
	}
	return parts[i]
}

// Resource identifies a data-scope resource type (dimension-two scoping).
type Resource string

// Permission is a code-fixed functional capability (the code's third segment is the action).
type Permission struct {
	Code Code   // Three-segment code such as "sys:account:create"; domain and module derive from the segments.
	Deps []Code // Prerequisites; may cross domains and modules, must be registered and acyclic (Seal checks).

	// RequireOTP is an output derived by Seal; do not write it by hand, because Seal rejects a declaration that sets it.
	//
	// What actually decides whether a step-up is needed is Requirement.RequireOTP (the only place authz reads). A
	// second hand-written copy here would drift: a binding marked step-up while this struct is not would hide a
	// high-risk permission once the field is sent to the frontend. So Seal derives it from the bindings: a code
	// referenced by any RequireOTP operation contains a step-up operation. It means "this permission includes an
	// operation that needs a second verification", and the UI uses it as a marker.
	RequireOTP bool
}

// Group is the grouping metadata of one business domain or module: it has only an order and no permission semantics.
//
// Groups cannot be granted: in the UI they are containers (expand + select-all) and match no code. What can be
// granted is always made of leaves (Permission) only, so a review never has to ask "what does granting this group
// give".
//
// Seal requires the business domain and module of every registered code to be declared here: a missing declaration is
// a startup error instead of a bare slug silently shown in the UI. The display name is likewise derived from the slug
// (see NameKey).
type Group struct {
	Domain string // Business-domain slug, the code's first segment.
	Module string // Module slug, the code's second segment; empty means this entry declares the business domain.
	// Sort orders siblings; ties sort by display name, then slug. Business entries must not use the large fallback sort
	// values the kernel uses.
	Sort int
}

// NameKey is the i18n message id for this group's display name (same as Code.NameKey).
func (g Group) NameKey() string { return nameKey(g.key()) }

// key is the group's registration key: "<domain>" for a business domain, "<domain>:<module>" for a module.
func (g Group) key() string {
	if g.Module == "" {
		return g.Domain
	}
	return g.Domain + ":" + g.Module
}
