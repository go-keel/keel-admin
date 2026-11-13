<!-- Shared file: carried verbatim; never localized outside its source repository.
     Checked by scripts/check-cross-repo-sync.sh; see docs/upgrading-protocol.md. -->

# Function permissions (RBAC)

Read this before declaring permission codes, binding endpoints or changing what a role grants.

## 4. Dimension one: function permissions (RBAC)

### 4.1 Permission codes

Fixed in code (not stored in the DB, so no dirty data). A code has **exactly three segments**, and the segments are the
structure:

```
merchant : order : refund
business  function  function
domain    module    operation
```

```go
type Code string        // typed permission code: Requirement.Codes []Code, Deps []Code,
                        // hasPerm(Code) all the way through; a bare string needs an explicit conversion, visible in review

func (c Code) Domain() string    // segment 1: business domain
func (c Code) Module() string    // segment 2: function module
func (c Code) Action() string    // segment 3: function operation
func (c Code) ModuleKey() string // "domain:module", the identity of the module

type Permission struct {
    Code Code   // three-segment code, "merchant:order:refund"; the sys business domain is reserved for the kernel
    Deps []Code // prerequisite codes; may cross business domains and modules (automatic closure: granting refund includes view)

    RequireOTP bool // derived output of Seal, do not write it by hand (see below)
}

func (c Code) NameKey() string // i18n message ID of the display name, derived from the code
```

**There are no Domain/Module fields: the structure is derived from the code segments.** Hand-written fields could
contradict the code (nobody can catch a contradiction like `{Code: "sys:dept:view", Module: "account"}`); derived
segments cannot drift. Likewise `RequireOTP` is derived by `Seal()` from the bindings (`otp.md` §7.2) and writing it by
hand is an error: what actually decides step-up is `Requirement.RequireOTP`, the one place authz reads, and two
declarations would inevitably drift.

**There is no display-name field either: the display name is an i18n key, also derived from the code**
(`"sys:dept:view"` → `perm.sys.dept.view`; kernel code entries are in `internal/i18n/sys.*.toml`, business code entries
in `app.*.toml`; `ListPermissions` translates on the spot by `Accept-Language`). A hard-coded literal name could only be
one language (switching to the English UI would still show a screen full of Chinese), and an extra hand-written
`NameKey` field could again contradict the code. Completeness of the kernel entries is guarded by
`TestPermissionRegistryHasLocalizedNames` in `cmd/<app>/registration_test.go`: a missing entry turns the test red
instead of the UI silently showing a slug; whether the business entries are fully translated is the derived repo's
responsibility.

**Exactly three segments, not "2 or 3 optional"**: with a variable segment count the UI would grow branches of uneven
depth, and "which module does this code belong to" would have no single answer. When a business has only one level of
grouping, it still writes the module segment explicitly (`cms:article:view`, not `article:view`).

**Code constants are defined where the biz function is implemented** (the function and its codes in one file, evolving
together). Naming: the constant is `Perm<Domain><Action>` (such as `PermArticleCreate`), the value is
`<business domain>:<function module>:<function operation>` (`Seal()` checks the format with a regex). Registration,
bindings and the frontend `hasPerm` all reference this one constant.

### 4.1.1 Groups: business domain and function module

**Business domains and function modules are only organization, not permissions.** They cannot be granted and have no
`Permission` in the registry: the grantable surface always consists of leaves (function operations) only, so a reviewer
never has to wonder "what does granting this group grant". In the UI they are expandable containers and select-all
entry points.

They only register a slug and an order; the display name is likewise derived from the slug as an i18n key:

```go
type Group struct {
    Domain string // business domain slug, segment 1 of the code
    Module string // function module slug, segment 2 of the code; empty = this entry declares the business domain
    Sort   int    // order among siblings; ties fall back to display name, then slug
}

func (g Group) NameKey() string // "sys" → perm.sys; "sys:dept" → perm.sys.dept
```

The kernel declares its `sys` business domain with `Sort: 1_000_000_000`, so it lists after every business domain;
give business domains smaller values.

Groups are registered through `perm.MustRegisterGroups`, separately from permissions: groups carry no permission
semantics, and several business modules may hang their own modules under the same business domain without gathering
into one call (declaring the same group twice panics). `Seal()` requires **the business domain and function module of
every registered code to have a declared group**; a missing one is a startup error, not a slug silently shown in the UI.

Business domains and function modules are each sorted by their own `Sort` ascending, ties broken stably by display name
and then slug. Business extensions use ordinary small values such as `0, 10, 20`; `1_000_000_000` is the value reserved
for the kernel `sys` domain so that "System management" always shows after every business domain, and business modules
do not use it. A module's Sort is compared only within its business domain and never mixed with business domain Sort.

### 4.1.2 Deps is a DAG, not a tree

`Deps` is a slice and `Seal()` checks for cycles: a code may have several prerequisites and may depend in a chain. This
is **orthogonal** to the classification in §4.1.1; do not mix them:

- **Classification**: every function operation belongs to exactly one module and one business domain → a real tree.
- **deps**: the implied expansion at grant time → a DAG.

`Effective()` expands recursively along deps, and it is **AND**: granting a code grants its whole prerequisite closure.
So `:enable` depending on `[:add, :edit]` actually means "granting enable also grants add and edit", which is usually
not the intent; to say "enable goes one step beyond edit", write a chain (`:enable` → `:edit` → `:view`).

The UI therefore cannot draw deps as a tree: a tree node has only one parent, and picking one hides the other
prerequisites as if they did not exist. The UI expresses deps with automatic ticking + a "N prerequisites" note +
locking of upstream codes (§4.5).

### 4.2 Endpoint binding

Each RPC operation → an access requirement, also fixed in code:

```go
type Requirement struct {
    Codes      []Code   // dimension one permission codes (biz constants, a typo does not compile)
    AllOf      bool     // true = all required; false (default) = any one
    RootOnly   bool     // root only
    RequireOTP bool     // per-operation step-up OTP
    Audit      bool     // audit after success
}
```

**An unbound operation is always 403 (deny by default).** Anonymous access is not expressed in Requirement; its only
source is the proto annotation `option (keel.v1.auth) = { allow_anonymous: true }`: Keel auth and admin authz build the
anonymous set from the same proto descriptor at startup, so they cannot drift.

### 4.3 Registration

Each module carries its own permission declaration file and registers it explicitly; sys and business are treated the
same. The code is in `docs/admin/permission.md`: step 2 (code constants, groups, Perms metadata), step 5 (bindings) and
step 7 (registration in di.go). Groups are registered before permissions, because `Seal()` requires full coverage; the
kernel batch goes through `perm.MustRegisterKernel`, the only call that may use the `sys` business domain; business
modules use `perm.MustRegister`, one line per domain; `perm.Seal(perm.ContractOperations())` runs once after all
registration and returns an error, after which the registry is read-only.

Division of labour: **biz = code constants + Permission metadata + group names** (the list of domain capabilities,
transport-agnostic); **service = operation → code binding** (the transport mapping). Compile-time protection on both
sides: operations use constants generated by make proto and codes use biz constants, so a typo on either side fails the
build; renaming an RPC or deleting a code breaks every reference at compile time, forcing them to stay in sync.

### 4.3.1 What Seal() checks

`Seal()` is called once after all registration; a failure fails startup, so configuration errors surface before
runtime:

| Check | Rejects |
|---|---|
| Code format | Anything other than three segments `<business domain>:<function module>:<function operation>` |
| Reserved business domain | A business batch (`MustRegister`) registering codes in the `sys` business domain |
| Group coverage | A code whose business domain or function module has no declared group |
| Hand-written RequireOTP | `Permission.RequireOTP` set by hand (it is a derived output) |
| deps existence | Depending on an unregistered code |
| deps cycles | A dependency cycle |
| binding references | A binding that references an unregistered code |
| Resource derivation | A `Via` parent resource that does not exist, or a parent that is itself derived (only one level allowed) |
| Operation ownership | A proto contract operation with neither a binding nor `allow_anonymous`, or with both |

**Dependencies are not limited by business domain or function module.** The business domain and module of a
three-segment code organize groups; they are not dependency boundaries: any registered permission code may be a
prerequisite, including across business domains. `Seal()` still rejects unregistered prerequisites and cyclic
dependencies. A dependency means the whole closure is granted implicitly, not a temporary allowance only while the
source operation runs: a role granted the source code also gets every operation bound to the dependency codes, while the
independent constraints (data scope, management boundary, `RootOnly`, step-up) are still checked as usual. Dependencies
are declared explicitly by the business in code; the kernel does not decide whether one is allowed from domain or module
names.

Seal also performs one **derivation**: it propagates step-up from operations to the codes they reference
(`Requirement.RequireOTP` → `Permission.RequireOTP`), so the UI can mark high-risk codes.

### 4.3.2 How the grant UI presents the model

`GET /v1/sys/meta/permissions` (root-only) returns the function operations + group display names in one call (display
names already localized by `Accept-Language`). The UI expands business domain / function module / function
operation; ticking an operation ticks and locks its prerequisites, step-up codes are marked, and groups are select-all
containers, never codes.

The display order is also a kernel contract: within a module, `view` first, then `create/update` side by side in the
same tier, then the other operations, and `delete` always last; within a tier, sorted stably by the size of the deps
closure and then by the full permission code. The order only helps administrators scan; it does not change what deps
grant. A new standard CRUD permission uses the `view/create/update/delete` actions; do not control its position in the
UI by slice declaration order, display names or numeric prefixes.

**Rule: registration is written explicitly in `di.go`; no `init()` self-registration and no reflection scanning**, so
the assembly has one traceable source and `Seal()` runs at a known time; a missed registration fails startup with
`perm.operation-unowned`, naming every endpoint left without a binding.

A duplicate registration panics; everything else fails startup through `Seal()` (table in 4.3.1).

### 4.4 Decision order (authz middleware)

```
1. In the anonymous set          → allow
2. Operation has no binding      → 403 FORBIDDEN (deny by default)
3. No principal                  → 401 UNAUTHENTICATED
4. bind-scope token              → allow only BindOTP/ConfirmOTP, everything else 403
5. Requirement.RequireOTP        → check the X-OTP-Code header (root too)
6. IsRoot                        → allow (audit still applies)
7. RootOnly                      → 403 NOT_ROOT
8. Code match (AllOf/AnyOf)      → 403 FORBIDDEN if not satisfied
```

### 4.5 Grant data plane

User→role: `sys_user_role`; role→code: `sys_role_permission`. Operators configure them in the role management UI with no
code. principal resolves them on every request: role codes are aggregated through the cache (`sessions-and-audit.md` §8)
→ `Effective()` expands the dependency closure.

### 4.6 Policy object lifecycle

**Kernel master data is soft-deleted throughout**: `sys_user`, `sys_dept` and `sys_role` all have `deleted_at`.
Users have no delete endpoint and can only be disabled; the Delete endpoints of departments and roles only write a
tombstone, and normal List/Get hide it through GORM's default scope. A role code is a stable identity: after soft
deletion it still holds its unique slot and a new role may not reuse it.

**Deleting a role only revokes; it does not destroy the historical configuration**: after a role is soft-deleted, its
rows in `sys_user_role`, `sys_role_permission`, `sys_role_data_scope`, `sys_role_resource_grant`,
`sys_role_manage_dept` and both directions of `sys_role_assignable` are kept. Authorization, scope, ARBAC and the
role/department name lookups all anchor on active `sys_role`/`sys_dept` (`deleted_at IS NULL`), so the permissions stop
working immediately while the full configuration the deleted role once granted stays traceable. A sensitive role can be
deleted: that flag only governs "non-root cannot assign it".

**Disabling a role revokes and keeps the memberships**: a role with `status = 0` counts for nothing in the granting
queries (permission codes, data scope, resource grants, assignable roles, manageable departments, forced authenticator):
Changing
the status invalidates the cache through UpdateRole and takes effect on the next request; the users' memberships are
kept, re-enabling restores them, and editing a user still pins it as usual. User lists and details show such a role on
the user, marked as disabled.

The association tables above are relations or fully replaced configuration, not independent master data: a normal edit
of a user's roles, a role's permissions or its boundaries replaces the whole set, physically deleting rows no longer
wanted and inserting new ones. `sys_audit_log` and
`sys_login_log` are append-only logs with no delete. The lifecycle of business area tables (without the `sys_` prefix)
is defined by the business.

| `sys_*` category | Tables | Lifecycle |
|---|---|---|
| Master data | `sys_user` | Soft deletion supported in the DB; the product offers only disabling, no deletion |
| Master data | `sys_dept` | An empty department can be deleted; only `deleted_at` is written |
| Master data | `sys_role` | root can delete; only `deleted_at` is written, the associated configuration is kept and stops working at once |
| Relations/configuration | `sys_user_role`, `sys_role_permission`, `sys_role_data_scope`, `sys_role_assignable`, `sys_role_manage_dept`, `sys_role_resource_grant` | No separate delete UI; old rows may be physically deleted when the configuration is fully replaced |
| Append-only logs | `sys_audit_log`, `sys_login_log` | No delete endpoint, no soft deletion |

**Deleting or renaming a permission code**: once a code is deleted in code, existing
grants in the DB `sys_role_permission` become dangling codes (`Effective()` ignores unknown codes, so they **silently
stop working** without an error). Rule: deleting or renaming a code must come with migration SQL that cleans up or
replaces the existing grants, in the same commit as the code change; renaming = the new code goes live + SQL UPDATEs the
old code to the new one. Resource types (resource_type) likewise: deleting one comes with migration SQL that cleans up
the existing policies in `sys_role_data_scope`.

### 4.7 Permission preview (root reduced-permission preview)

root can enter "Permission preview" from the avatar menu to view the whole system as a set of roles. The mechanism is a
**request-header reduced-permission preview**: the frontend sends `X-Preview-Roles: 1,2` (CSV role IDs) on every
request, and after the principal middleware resolves the principal, it recomputes a non-root principal from those roles
**only when the real identity is root**.

- **Only reduces, never escalates**: the header is ignored for non-root, so the attack surface grows by nothing.
- **Stateless**: the preview state lives only in the frontend's localStorage; the backend stores nothing and has no new
  endpoint.
- **The whole chain really applies**: permission codes, RootOnly rejection and ScopeSpec data scope all run with the
  reduced principal; writes really execute with the selected roles' permissions, hence the warning button that stays at
  the top.
- `UserID`/`DeptID` stay root's own: audit attribution must be real, and the simulation is "only these roles", not
  becoming another user.
- Resolved on every request, so a changed or deleted role is followed at once; an unknown role ID resolves to no codes,
  which fails safe (fewer permissions).
- step-up OTP is unaffected: it comes before the root bypass and applies to everyone.

**Changing the role selection is not offered during preview**: `ListRoleOptions` needs `sys:account:view`, so previewing
as a role without that permission always gets 403. To change the selection, exit and choose again (by then the button
has turned back into the entry point).
