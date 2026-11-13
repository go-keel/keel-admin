<!-- Shared file: carried verbatim; never localized outside its source repository.
     Checked by scripts/check-cross-repo-sync.sh; see docs/upgrading-protocol.md. -->

# Management boundary (ARBAC)

Read this before changing who may manage which users, departments and roles.

## 6. Dimension three: management boundary (ARBAC)

Answers "whom may an administrator manage". It applies to **account and department management operations** and is
guarded in the biz layer (the decision needs the target object, which the middleware layer does not have). The managing
subject is Actor = {UserID, IsRoot, RoleIDs}; root bypasses all of it.

| Guard | Meaning | Applied at |
|---|---|---|
| Self-operation | Non-root acting on themselves is always rejected; the lockout-prevention tier rejects even root | All management actions (through `target`); lockout-prevention tier: DisableUser / SetUserRoot / UpdateUser changing one's own IP allow list |
| Target manageable | Target is root → reject; target department ∉ my manageable range → reject | All management actions (through `target`) + GetUser |
| Department manageable | Target department ∈ the union of manage_dept over all my roles (subtrees expanded) | CreateUser / UpdateUser changing the department / Dept CRUD |
| Roles assignable | All must be ∈ union of my roles' assignable ∩ enabled ∖ sensitive (non-root can never assign a sensitive role); the same range as the assignment dropdown and viewing role permissions | CreateUser; UpdateUser changing roles checks the submitted set against the same range and pins roles outside it |

**Department CRUD boundary**: the target department of Create (with the parent department as target) / Update / Delete
must be inside the actor's manageable subtree, root excepted: symmetric with account management, otherwise the
`sys:dept:update` code alone would allow changing any department. A top-level department (parent 0) is created by root
only (`NOT_ROOT` otherwise), so the page shows the "new top-level department" entry to root only.

**Disabling or enabling a user is idempotent**: disabling a user who is already disabled, or enabling one who is already
enabled, succeeds and changes nothing.

**Disabling a department affects only new choices**: a disabled department cannot be chosen as a user's department
(`DEPT_DISABLED`; the picker greys it out), while its members still sign in and it still counts in data scope and
management ranges. Disabling should not lock a whole team out in passing; to cut people off, disable their accounts.

Boundary configuration (submitted together with the rest of the role configuration through CreateRole / UpdateRole, and
replaced as a whole in one transaction) is root only; `manage_dept` stores plain dept_ids, and every one always expands
to its whole subtree at decision time (managing a department = managing its whole line).

**Sensitive roles (a second safeguard for technical/system roles)**: non-root can never assign them; this is checked
before the assignable allow list, so even if root mistakenly puts one in the allow list it cannot be assigned. **This is
its only meaning**: it does not restrict root from changing or deleting the role, and `sensitive` is just an ordinary
field in the `UpdateRole` form. `CreateRole` can create a sensitive role.

> It deliberately has no second meaning such as "cannot be changed or deleted (even by root)": that would force a
> separate "unprotect" endpoint, and once the field could not be changed in the edit form, role editing would have to be
> split into two steps, for little gain: only root can change roles, with a one-time code and an audit record.

**The assignable role list and pinning**: `ListAssignableRoleOptions` (bound to `sys:account:update`: assigning roles is
part of editing a user and has no code of its own) returns the roles the operator may assign: everything for root; for
non-root = union of assignable ∩ enabled ∖ sensitive, and the frontend assignment dropdown is exactly this set. When
`UpdateUser` fully replaces roles, the part of the target's existing roles outside the operator's assignable range (such
as a sensitive role granted by root) is automatically pinned by the server and kept as is, closing the
"not visible in the dropdown → absent on submit → removed" privilege-stripping hole; root gets exactly what it submits,
no pinning. What to pin is computed from the roles the user holds at the moment of saving, so a role someone else just
added that this operator cannot see is not removed by this save.

**Filter dropdown ≠ assignment dropdown.** The set above serves only "assign". **Filtering** the user list by role uses
`ListRoleOptions` (all roles, bound to `sys:account:view`): the role tags in the list rows already show every user's
roles, and a filter offering only the assignable subset could not filter by roles that are visible. They must be two
data sources: merged into one, either the filter misses entries or the assignment dropdown offers options ARBAC
rejects.

**Selector candidates come from each domain's own `*Options` endpoint**: the URL follows resource ownership, the
permission binding follows **purpose**, and the two are decoupled (see the rule on selector/filter candidates in
`AGENTS.md`):

| Endpoint | Path | Purpose | Binding | Range |
|---|---|---|---|---|
| `DeptService.ListDeptOptions` | `/v1/sys/depts/options` | Department filter / owning department selection | `sys:account:view` | Filtered by manageable departments, everything for root (same criterion as the user list) |
| `RoleService.ListRoleOptions` | `/v1/sys/roles/options` | Role filter / "Assignable roles" configuration | `sys:account:view` | All roles |

The payload contains only identifying fields (department id/parent_id/name, role id/code/name), not management content
such as perms, the three boundary dimensions or department headcount: that is what allows the gate to be lower than that
of the management group it belongs to. Why not reuse `ListDept`: it is bound to `sys:dept:view` (department
**management** permission), and calling it from the user management page would make someone with only account view
permission hit 403 as soon as they open the page, while that data is needed for the page to render at all.

**Semantics of organization tree changes**: departments are the anchors of dimensions two and three, so changes are
strict:

- **No moving** (changing parent, relocating a subtree): `UpdateDept` only changes name / order / status. Reorganizing
  goes "create the new department → move users over one by one with
  `UpdateUser` → delete the emptied old department", every step
  within ARBAC boundaries and under audit;
- **Only empty departments can be deleted**: no child departments, no active users, no resource ownership (only
  resources that declare `DeptOccupancy` are checked); otherwise rejected. A deletion that passes only soft-deletes the
  department, with no cascading deletes and no resource transfer, fail-safe;
- Consequence: an `ancestors` path, once established, is never recomputed, so subtree expansion has no invalidation
  case.
