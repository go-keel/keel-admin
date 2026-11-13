<!-- Shared file: carried verbatim; never localized outside its source repository.
     Checked by scripts/check-cross-repo-sync.sh; see docs/upgrading-protocol.md. -->

# keel-admin permission model

Read this to find the page for a permission question, and for the positioning and the overview every page assumes.

> These pages are the **permission specification** shared by keel-admin and its derived repos: the full semantics of
> the permission model, the layering of the project, the business integration contract and the scenarios. Business
> code that plugs into the permission system follows them; the step-by-step guide is `docs/admin/permission.md`. The
> business code and scenarios in them are fictional examples (articles, categories, customers, orders) and need not
> exist in the repository; `<pkg>` is the proto package directory under `api/`, `<app>` is the entry directory name.

| Page | Read it |
|---|---|
| [structure.md](structure.md) | before adding a file, to know whether its directory is kernel (read-only), business or an append-only seam |
| [function-permissions.md](function-permissions.md) | before declaring permission codes, binding endpoints or changing what a role grants |
| [data-scope.md](data-scope.md) | before making a business resource follow data scope, or writing a custom scope resolver |
| [management-boundary.md](management-boundary.md) | before changing who may manage which users, departments and roles |
| [otp.md](otp.md) | before adding a step-up operation, or touching OTP login admission or the login captcha |
| [sessions-and-audit.md](sessions-and-audit.md) | before relying on when a permission change takes effect, kicking sessions, or adding an audited action |
| [scenarios.md](scenarios.md) | when a permission need does not obviously fit one dimension: each scenario shows which dimensions combine |
| [deployment-and-config.md](deployment-and-config.md) | before deploying an admin, or changing the `sys:` section of app.yaml |
| [frontend.md](frontend.md) | before gating frontend pages or controls by permission codes |

## 1. Positioning and layering principles

keel-admin is an admin back-office **template** built on the Keel framework and the keel-layout conventions. A business
team forks it into a **derived repo** and builds its own admin there. The core asset of the template is the
**four-dimension permission model**. The layering principle in one sentence:

> **`internal/core/` is read-only as a whole: a derived repo does not change it and overwrites the whole directory on
> upgrade; business code goes into the three business layer directories biz/data/service, plus a few assembly seams
> (the list is the "Assembly seam" entry in `GLOSSARY.md`).**

"Overwrite the whole directory" is not free: a derived repo changed the module name when it was set up, and
`internal/core/` imports this module path throughout, so after overwriting you must **rerun `make rename MODULE=<this
repo's module name>`** before it compiles (the command is idempotent and only touches files that still carry the
upstream module name). Whether and when to follow upstream is the derived repo's decision: the template ships no upgrade
tool and no incremental migration scripts (schema changes likewise; see the matching entry in `UPGRADING.md`). Details
in `docs/adr/layout-0001-template-upgrade-strategy.md`.

| Area | Contents | Business development |
|---|---|---|
| Kernel `internal/core/` | The four-dimension permission engine + the system domain (accounts, roles, departments, audit, OTP) | Read-only; upgrade = overwrite the whole directory + rerun `make rename` |
| Business area `internal/biz|data|service/` | Business domain code (layout layers first) + each domain's permission declarations | The only place you write code |
| Assembly seams | For permissions: registration and wiring in `cmd/<app>/di.go`, config fields in `internal/conf/app.proto`, business proto in `api/<pkg>/v1`; **the full list is the "Assembly seam" entry in `GLOSSARY.md`**, not repeated here | Append to the list; do not change what is there |
| Generated artifacts / third-party components | `*.pb.go`, `openapi.yaml`, `web/src/generated/` and so on; `web/src/components/ui/` | See the entries of the same name in `GLOSSARY.md` |

**Positioning boundary (a principle that overrides the layering)**: the admin does only the admin dashboard's own work
and carries no business logic. **Dependencies point one way: admin → business** (admin management pages call business
system APIs or read business data to build management screens); **a business system must not depend on anything built
into the admin**: it does not read the admin's tables, does not call the admin's endpoints, and its runtime behaviour is
not controlled by the admin's configuration. Consequences:

- **The only admin data a business system may reference is the two ownership coordinate IDs, `user_id` and
  `dept_id`** (stored as ownership columns of business tables, see the unified ownership model in `data-scope.md` §5;
  they are ID references only and create no runtime dependency). Everything else (kernel config, roles, permission
  codes, internal tables) is internal to the admin;
- The kernel config (the `sys:` section of app.yaml, `deployment-and-config.md` §13) governs only the admin's own
  behaviour; runtime parameters of external business services do not go there;
- **No built-in data dictionary**: dictionaries are consumed by business forms and business data, so a business that
  needs one builds it in the business system (and the admin manages it through the business API); the admin's own
  enumerations are all closed proto enums and need none;
- "Business area / business domain" on this page means the **management modules developed inside the admin** after the
  fork; they are part of the admin and may use the kernel facilities. It does not mean business services outside the
  admin.

The idea: **the admin is the operators' terminal and entry point, not the core of the business**:

- **The source of truth is the business system**: business state machines and invariants belong to the business
  services; the admin is one of their clients that happens to look like a console;
- **The admin is replaceable and may be absent**: when the admin is down the business keeps running (operators just
  cannot act for a while); swapping in another back office needs zero changes to the business systems;
- **Managing external business data: writes go through the business API first** (RPC, HTTP or MQ), so the invariants
  are guarded on the business side; **read-only queries may connect to the business database directly** (a replica or
  a read-only account); writing the business database directly is neither recommended nor forbidden by the template,
  and after the fork the business team owns the invariants (see scenario S10);
- **Exception: for the admin's own domains, the admin is the source of truth**: accounts, roles, permission policies,
  audit and in-app messages are "the back office's own business" and the admin holds them directly, as it should.

Business area code **must follow the keel-layout conventions** (biz/data/service layering, one file per domain,
sentinel errors, proto-first, no AutoMigrate), described in the pages under `docs/guides/`. The admin adds only one
convention on top: permission declarations come in two parts: **code constants + Permission metadata are defined where
the biz function is implemented** (`biz/{domain}.go`, the function and its codes in one file), and **the
operation→code binding goes in `service/{domain}_perm.go`** (the transport mapping). Both ends are compile-time
constants (codes are typed biz constants, operations are constants generated by make proto), so a typo fails the build.

## 3. Permission model overview

The mental model in one sentence:

> **Roles decide what you can do, how much you can see and whom you can manage; the department decides where you stand
> and where data belongs.**
> User = the union of several roles (policy) × one department (coordinate).
> Every policy (function codes, data scope, management boundary, force_otp) is configured on **roles**;
> the department is not a controller but the **coordinate system** for data ownership and scope calculation
> (DEPT/ALL_SUB start from the user's department).

**Single-department constraint**: a user belongs to exactly one department. Cross-department visibility comes from
multiple roles (union of policies) + RESOURCE_GRANT (explicit assignment: role × instance set), not from multiple
department memberships.

Four dimensions, each answering one question:

| Dimension | Question | Mechanism | Storage |
|---|---|---|---|
| One: function permissions | **May you call** this endpoint | RBAC: permission codes + endpoint bindings | Codes/bindings in code; user→role→code in the DB |
| Two: data scope | **Which rows** can you see | role×resource scope policies | `sys_role_data_scope` |
| Three: management boundary | **Whom may you manage** | ARBAC: manageable departments + assignable roles + anti-escalation | `sys_role_manage_dept`, `sys_role_assignable` |
| Four: OTP | Must you **prove it is you again** for a critical action | Login admission + per-operation step-up | `sys_user.otp_secret_enc`, role `force_otp` |

**The four dimensions are orthogonal and must not be inferred from each other.** In particular, you cannot tell from
the function codes a role has ticked whether its data scope or management boundary "will take effect". Two reasons:
dimensions two and three apply over the **union of all of the user's roles** so one role alone cannot answer the
question; and "which use cases read a given boundary" is an internal decision of the biz layer, so copying it into a
criterion (especially in the frontend) is bound to drift from the implementation. A hint such as "this boundary has
no effect" would also discourage the modelling the orthogonal design encourages: one role that sets only a boundary,
combined with another that sets only permission codes.

The request decision chain (the middleware order is fixed and assembled by the kernel):

```
keel auth ── JWT signature check (HS256, secret = server.http.default.auth.jwt_secret)
   │           anonymous allow list: proto (keel.v1.auth).allow_anonymous (the only source)
   ▼
principal ── claims.sub → load user (status≠1 → no principal) → roles → permission codes (cached)
   │           → expand dependency closure → Principal{UserID, IsRoot, Perms, DeptID, RoleIDs}
   ▼
authz ────── dimension one decision (see `function-permissions.md` §4.4) + bind-scope gate + step-up OTP
   ▼
audit ────── operations bound with Audit:true write sys_audit_log after success
   ▼
biz layer ── dimension three ARBAC guards (account/department management)
             dimension two scope filtering (data queries, repos use ApplyScope)
```

The anonymous annotation and the JWT configuration are Keel's: `keel:docs/auth.md#anonymous-endpoints`.

**root** (`is_root=1`): bypasses dimensions one to three; step-up OTP and audit **still apply**; a non-root user cannot
manage it (TargetProtected).

**root is created only in the table creation script; code does no automatic seeding**: the end of
`migrations/admin/0_schema.sql` idempotently INSERTs the root department and the root user, the password column holds
the argon2id hash of the initial password admin888, and a comment states it; on every new deployment root signs in and
changes it in the personal center right away. A database without root is a deployment error, and startup does not
cover it up.

**The JWT carries identity only**: claims = `sub`/`iat`/`exp`/`ver` (a bind token adds `scope=otp_bind`). Permissions
and roles never go into the token: principal resolves them on every request, so revocation takes effect at once (a hard
constraint). `ver` is session invalidation metadata (see token_version in `sessions-and-audit.md` §8), not authorization
data, and does not break that constraint.

## 10. Business integration contract

Business code follows the keel-layout conventions. The steps for adding a domain, with a checklist, are in
`docs/admin/permission.md`; after them, operators or root configure role → permission codes and role × resource scope
in the role management UI, with no code.

The kernel provides exactly: the decision chain middleware, registration and validation, ScopeSpec resolution +
ApplyScope, and the OTP/audit facilities. **The business implements no decision logic.**
