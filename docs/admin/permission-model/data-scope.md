<!-- Shared file: carried verbatim; never localized outside its source repository.
     Checked by scripts/check-cross-repo-sync.sh; see docs/upgrading-protocol.md. -->

# Data scope

Read this before making a business resource follow data scope, or writing a custom scope resolver.

## 5. Dimension two: data scope (role × resource scope)

### 5.1 Concepts

Answers "which rows of this kind of resource can this role see". Policies are configured per **role × resource_type**
(`sys_role_data_scope`, UNIQUE(role_id, resource_type)).

**The unified ownership model (coordinate system / resolution / enforcement separated)**:

> The ownership **coordinate system** is the admin's (`user_id`/`dept_id` as global IDs that any system's tables may
> store, the only reference `README.md` §1 allows); policy and **resolution** live in the admin (role × resource →
> ScopeSpec); **enforcement** lives with the resource's host: if the data is in this system's tables, one `ApplyScope`
> line; if the data is in an external business system, the admin **passes down the ScopeSpec** (or the already-resolved
> set of visible parent IDs) when it calls, the business side filters, and the business may also ignore what it receives
> and decide on its own (it is the final gatekeeper of its own data). The runtime dependency always points admin →
> business (see scenario S10).

**Resources come in two kinds by ownership shape** (declared at registration, `docs/admin/permission.md` step 2):

- **Anchored resources**: the table carries ownership columns directly (owner_user_id / owner_dept_id), such as
  merchants: the salesperson who opens one writes their own coordinates;
- **Derived resources**: ownership is derived through a parent resource, declared as `Via{Parent, FK}`, such as orders:
  "my department's orders" = "orders of merchants I can see", and the order table has zero ownership columns. In the
  same database this is a subquery (`merchant_id IN (SELECT id FROM merchant WHERE <scope>)`); across systems the set
  of visible parent IDs is passed down. **Via allows only one level** (multi-level chains make query cost and semantic
  complexity climb steeply; if one is really needed, flatten it with a redundant column). Derived vs redundant is the
  business's choice: a redundant ownership column queries fast but must be refreshed when the parent resource is handed
  over; derivation needs no maintenance but costs one more hop.

**resource_type** identifies one kind of business data under data scope control (usually one business table or one
aggregate). It is the typed `perm.Resource` (treated like `perm.Code`); **the constant is defined where the biz
function is implemented and registered through the third argument of `MustRegister`** (`ResourceDef`; code in
`docs/admin/permission.md` step 2).

The registration loop is closed: the meta endpoint exposes the list of resource types → the role management UI
**selects from a dropdown** (no free typing); `CreateRole` / `UpdateRole` check resource_type ∈ registry (a typo is
rejected at configuration time); the biz query side calls `scope.Resolve(ctx, actor, biz.ResArticle)` with the same
constant (a typo does not compile). Configuration, validation and query share one source.

### 5.2 The five scopes and how they resolve

| ScopeType | Meaning | Resolves to |
|---|---|---|
| SELF | Owned by me only (I am responsible) | `owner_user_id = userID` |
| DEPT | My department | `dept_id ∈ {my deptID}` |
| ALL_SUB | My department and its subtree | `dept_id ∈ subtree department set` (expanded from the ancestors path) |
| RESOURCE_GRANT | Explicit assignment (role × instance set) | The union of instances granted to all of the actor's roles in `sys_role_resource_grant` → `id ∈ instance set` |
| ALL | Everything | No filter |

Resolution rule: the **union** of the policies of all of the user's roles, conditions **OR-ed**; an ALL short-circuits
to everything; root is always ALL; **no condition at all → deny-all (`WHERE 1=0`)**. The output is
`ScopeSpec{All, SelfUserID, DeptIDs, ResourceIDs}`.

**The contract of explicit assignment is "role × resource type × set of instance ids"**: what an instance is (category,
brand group, store) the kernel does not interpret: it stores ids and returns ids, and their meaning and expansion belong
to the business. Grants hang on roles, not users (the same direction as the other policies: policy is configured only
on roles, a person is only a union of roles). A new instance is granted to no role automatically: a creator whose
roles hold only some instances does not see what they create until one of their roles is granted it, because
granting the creator automatically would bypass the grant step. Instance candidates come from the `ListGrantable`
callback declared when the resource is registered; configuration validation (every id ∈ candidates), the UI dropdown and
resolution share that one source. Where it applies: resources with few, stable instances (on the order of groups or
categories): the callback has no paging, and candidates have a **hard limit of 100** (`perm.MaxGrantOptions`, enforced
for both configuration and validation through `perm.GrantOptions`; **exceeding it is an error, not a truncation**:
truncation would have administrators configure grants from an incomplete list, while an error exposes the problem at
configuration time). Candidates approaching the limit mean the resource has crossed that boundary and the **design
should be reconsidered**: switch to ownership columns (owner columns) or business-built filtering, rather than finding a
way to raise the limit; resources with huge numbers of instances (customers, orders) should never be registered as
explicit assignment resources in the first place. Per-user point exceptions ("Zhang San follows up this one customer")
are outside the kernel's semantics: normal ownership uses owner columns + handover, and a business with a real
per-person exception dimension builds its own `ScopeResolver` (§5.6).

### 5.3 Business integration: one GORM helper

```go
// internal/data/<domain>.go (business repo, list/aggregate queries)
func (r *articleRepo) List(ctx context.Context, spec perm.ScopeSpec, ...) {
    q := r.db.WithContext(ctx).
        Scopes(permdata.ApplyScope(spec)).   // ← kernel helper, the deny-all fallback is inside
        Where("status = ?", 1)
    ...
}

// When column names differ from the convention (owner_user_id / owner_dept_id):
Scopes(permdata.ApplyScopeWith(spec, permdata.Cols{OwnerID: "salesman_id", DeptID: "org_id"}))

// Two ways to consume an explicit assignment (the same spec):
// 1. Filter the resource's own table: ApplyScope by default applies ResourceIDs to the primary key column id;
// 2. Gate a child table: apply the parent resource's (such as a category's) spec to the foreign key column of the
//    child table (such as articles), so child rows follow the parent instance grants; the two Scopes are AND-ed:
q.Scopes(permdata.ApplyScope(rowSpec),
    permdata.ApplyScopeWith(catSpec, permdata.Cols{PK: "category_id"}))
// 3. Or stay out of SQL: spec.ResourceIDs itself is the set of visible instances, use it directly to expand/fan out
//    (passing down across systems, see the unified ownership model in §5.1). Note: All=true must be materialized as
//    everything, and an empty set must be expressed explicitly as "call none": the fan-out side has no WHERE 1=0 fallback.
```

Convention: business tables taking part in scope carry the ownership columns `owner_user_id` / `owner_dept_id` per their
declared anchors (or map them explicitly with `Cols`). **Owner ≠ creator**: the owner can change (handover changes it),
created_by is an immutable audit fact and does not take part in scope (a business that needs the trail adds its own
column). The helper does not force a single query entry point (GORM keeps its freedom); "forgetting the helper" is an
accepted residual risk, constrained by the scenarios on this page + the code review checklist.

### 5.4 Configuration surface

`CreateRole` / `UpdateRole` (root only) submit the role's scope policies with the role and replace them as a whole
(the RESOURCE_GRANT instance rows are fully
replaced in the same transaction as the policy rows); checks resource_type ∈ registry, scope_type ∈ the five **and that
it matches the anchors the resource declared** (ALL can always be configured; a resource without AnchorSelf rejects
SELF; a resource without AnchorDept rejects DEPT/ALL_SUB; a resource without
ListGrantable rejects RESOURCE_GRANT; an explicit assignment instance set must not be empty and every id must be ∈
candidates, ruling out "dead policies that always deny all"). The operator UI selects from the meta resource list in a
dropdown, filters the options by anchors, and picks explicit assignment instances from the candidates (fetched from
ListGrantable on the spot); business code takes no part.

### 5.4b Ownership lifecycle

- **Ownership is fixed and does not follow the person**: a resource's `owner_user_id`/`owner_dept_id` are taken from the
  creator and their department at creation (initial values) and live independently afterwards. When a person moves, the
  resources stay in the original department (organizational assets do not leave with the employee).
- **Changing a user's department (`UpdateUser`) does not cascade**:
  moving a person to another department affects only the coordinates of resources they
  create afterwards and their own scope anchor; existing resources' ownership does not change.
- **Consequence for SELF**: a person who moved and has SELF scope still sees the existing resources owned by them
  (owner_user_id did not change); an organization that does not want that configures only DEPT, or hands the resources
  over, after which the changed owner makes them invisible.
- **Ownership changes through an explicit handover**: for anchored resources, the business should implement a "handover"
  operation (changing owner_user_id / owner_dept_id; **audit facts such as created_by never change**), checking that
  both departments are within the operator's ARBAC manageable range + `Audit: true`; the kernel migrates nothing
  automatically.
- **Derived resources follow the parent automatically**: when a merchant is handed over to another department, the
  visibility of its orders changes with it, with zero maintenance (the redundant column approach needs the business to
  refresh it during the handover).
- Closing the loop with department deletion: only empty departments (`management-boundary.md` §6), and "empty" includes
  the resource ownership check: hand resources over first.

### 5.5 Registering a business resource type

The steps (first deciding whether the module needs scope at all, then the table, the resource constant and anchors,
resolving the spec, filtering, ownership on write, registration, configuration and tests) are in
`docs/admin/permission.md` steps 0–10. The constraints those steps implement:

- **Scope is optional, not a mandatory shackle.** Global data uses dimension one only; data whose grant dimension is
  neither person / department nor an instance set (per-person point exceptions, algorithmic visibility) does not use
  the kernel scope: the business writes its own filter in the repo or builds its own `perm.ScopeResolver` (§5.6), with
  dimension one controlling the entry.
- **Filter on every path.** Scope applies not only to lists: List, Get, Update and Delete all carry `ApplyScope`.
  The business writes the filter, not the kernel: which queries filter is a business judgement (a duplicate-name
  check or the category picker of role configuration must see every category), and a kernel-enforced filter would
  need an escape hatch for exactly those.
  Filtering only List while Get/Update/Delete fetch by id directly leaves a back door in the scope control: this is the
  first item of the code review checklist. An id out of scope behaves exactly like a nonexistent one (404, not 403; an
  Update or Delete affecting 0 rows is a 404), so existence does not leak.
- **Ownership on write comes from the Principal, never from client input**: `owner_user_id = actor.UserID`,
  `owner_dept_id = actor.DeptID` as initial values (what happens to them afterwards: §5.4b).
- **The use case depends on ports, not on the kernel.** `perm.ScopeResolver` and `perm.GrantRevoker` are interfaces in
  core/perm, implemented by core/sys/biz.ScopeUseCase and injected through the business use case's constructor, so the
  business area does not import core/sys.

**The explicit assignment cleanup contract**: after the business deletes a grantable instance (a category or brand
group), it calls the kernel `perm.GrantRevoker` port to clean up the roles' grant rows for it: sys_* tables are read and
written only by the kernel, so the business **requests** the cleanup and does not delete directly (a dangling grant row
does not widen permissions, since the instance no longer exists and the resolved id matches no data; cleanup keeps the
role configuration from still showing a deleted instance). In a cross-database deployment the two steps share no
transaction: delete the instance first, then revoke the grants; a failed revocation affects only the display, and it
heals on retry or the next full replacement when the role is edited.

### 5.6 A custom ScopeResolver: the escape-hatch contract

`perm.ScopeResolver` is a **business extension point the kernel endorses**, not a way around the kernel: the
interface is defined in core/perm (the contract layer), and the kernel's `ScopeUseCase` is only one implementation (what
`docs/admin/permission.md` step 3 injects is the interface). When the grant dimension goes beyond the three kinds
"person / department / instance set" (per-person point exceptions, algorithmic visibility, decisions by an external
system), the business builds its own resolver, its own grant tables (**never touching sys_\* tables**) and its own
management UI, and injects the resolver into its UseCase; the `ApplyScope`/consumption logic on the repo side does not
notice.

A custom implementation must keep three invariants; missing any one means over-granting or under-granting:

1. **root short-circuit**: `actor.IsRoot → ScopeSpec{All: true}`, handled on the very first line (the kernel
   implementation does exactly this). Missing it = root is denied, which is found quickly; but if it is written as
   "allow by default, then exclude", what is missing becomes over-granting, which is found slowly. **Start from deny and
   add allowances; never the other way round.**
2. **The zero-value spec = deny-all is a convention across the whole chain**: with no grant, return the zero value
   `ScopeSpec{}`; never treat "not configured" as All. The SQL consumer `ApplyScope` has the `WHERE 1=0` fallback; the
   fan-out consumer has **no** fallback, and an empty set must be expressed explicitly as "call no downstream".
3. **The output's meaning matches the consumer**: `All=true` means "no filter" on the SQL side, but on the fan-out side
   it must be **materialized as the full target set**: fan-out has no notion of "no filter".

Composition is allowed: a custom resolver may hold the kernel resolver and delegate to it first (getting the `All`
short-circuit and a base scope, then adding its own dimensions); `Principal.RoleIDs` is already in the arguments, so
resolving its own grant tables by role needs no help from the kernel.
