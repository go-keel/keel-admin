<!-- Shared file: carried verbatim; never localized outside its source repository.
     Checked by scripts/check-cross-repo-sync.sh; see docs/upgrading-protocol.md. -->

# Assembly: how the kernel and the business area connect

Read this before changing `cmd/<app>/di.go`, adding a cmd or touching anything the kernel owns, together with
`docs/guides/assembly.md`.

The guide covers the general di.go rules (construction order, LIFO cleanup, named instances, adding a cmd); this page
covers only what an admin built on keel-admin adds. `<app>` is the entry directory name, `<module>` is the module
path in this repository's `go.mod`.

## Shape

- **One entry**: the single binary `cmd/<app>` serves the proto HTTP endpoints and the embedded React SPA (`web/`;
  `make build-web` builds it into `web/dist`, and `server.RegisterSPA` mounts it on the same HTTP server, where API
  routes take precedence). There is no gRPC, event consumer or async task entry; when you need one, add it per
  "Adding a cmd" below, so the admin binary stays one deployable unit.
- **The kernel is read-only for derived repos**: on the backend `internal/core/` (the permission registry `perm`,
  data scope `permdata`, the `principal` / `authz` / `audit` middleware, the system domain `sys`, TOTP) and
  `internal/server/` (kernel HTTP registration, SPA and brand mounting, the providers), plus the admin database's
  complete schema `migrations/admin/`; on the frontend the kernel
  zone listed in `docs/admin/shell.md` "Frontend zones". An upgrade overwrites these directories whole and then
  reruns `make rename MODULE=<module>`, so anything a derived repo puts inside them is wiped. The package-by-package
  map is in `docs/admin/permission-model/structure.md`.
- **Business modules (permission codes, data-scope resources, new domains) live in the business area**
  `internal/biz|data|service/`, never in the kernel, for the same reason.
- **The assembly seam is `cmd/<app>/di.go`**: business construction, permission registration and HTTP registration
  are **appended** there, without changing the kernel (the full list of assembly seams is in `GLOSSARY.md`
  "Assembly seam"). The HTTP registration functions and the raw-route registration functions of business services
  are written in `internal/service/http.go` and appended by `cmd/<app>/di.go` after the kernel's list, never in
  `internal/server/`.

## Fixed construction order

On top of the guide's order, the kernel and permission registration sit between the infrastructure and the server:

```
kconfig.NewConfig → kotel.Setup → keel.SetupLogger → keel.NewAppFactory
  → infrastructure: kdb.NewNamedDB(config, "admin") + kdb.NewDB(config) + kredis.NewRedis
  → kernel data (internal/core/data, internal/core/sys/data)
  → kernel config: server.ProvideSysConf / ProvideBrand / ProvideOTPCipher … (sys.* in app.yaml, read once at startup)
  → kernel biz → kernel service
  → business area data → biz → service (internal/data|biz|service)
  → permission registration: perm.MustRegisterGroups → MustRegisterKernel / MustRegister → perm.Seal(perm.ContractOperations())
  → HTTP registration: server.KernelHTTPServers(...) + business appends → server.NewHTTPServerRegister
  → newApp(...) → factory.New(servers...)
```

- **`perm.Seal()` runs once, after every `MustRegister*`**: it freezes the registry and checks that every code's
  business domain and feature module have a declared group, and that every endpoint of the proto is bound or
  allow_anonymous; a missing one fails startup on the spot.
- **The business area uses `perm.MustRegister`, not `MustRegisterKernel`**: the `sys:` business domain is reserved
  for the kernel batch. How to declare permission codes, groups and resources is in `docs/admin/permission.md`.
- **Both databases are required**: `kdb.NewNamedDB(config, "admin")` is the admin database (all `sys_*` tables),
  `kdb.NewDB(config)` is the business database (`database.default`, the business area's own tables). **A business
  repo holds the `Data` of the business area's own `internal/data` (built from the business database), never the
  kernel's `Data` from `internal/core/data`**: borrowing it silently breaks splitting the databases. The trade-offs
  are in `docs/adr/admin-0002-two-databases-and-default-ownership.md`.
- The business area's package names (`biz` / `data` / `service`) collide with those under `internal/core/sys`, so
  di.go always imports them with aliases (by convention `nbiz` / `ndata` / `nsvc`).

## HTTP registration: the kernel's list first, the business area appends

```go
// cmd/<app>/di.go (excerpt)
httpRegisters := server.KernelHTTPServers(authService, deptService, roleService,
    userService, metaService, auditService)
httpRegisters = append(httpRegisters, nsvc.ArticleHTTP(articleService)) // one line per business module
httpServerRegister := server.NewHTTPServerRegister(httpRegisters...)
sentinelGroup := nsvc.RegisterErrors(service.NewErrorSentinel()) // kernel mappings underneath, business on top
```

The business registration functions live in the business area's `internal/service/http.go`, one per service:

```go
// internal/service/http.go (a fictional ArticleService)
func ArticleHTTP(svc *ArticleService) api.HTTPServerRegister {
    return func(s *api.HTTPServer) { v1.RegisterArticleServiceHTTPServer(s, svc) }
}
```

`internal/server/http.go` is kernel code; adding a business module never touches it. Registering raw routes
(webhooks, downloads) is covered in `docs/admin/raw-http.md`.

## newApp: three more links in the chain, the SPA on the same server

`newApp` lives in `cmd/<app>/main.go`. `mw.Default(c.Config)` reserves the authentication slot and
`api.NewHTTPServer` fills it; the admin appends, in order, `principal` (verified claims → principal) → `authz`
(permission-code bindings in code, and step-up) → `audit` (write-operation audit);
locale sits right after metadata and sentinel comes last.
`principal` is appended at the current end of the chain, so it naturally runs after the authentication slot.
`server.RegisterSPA(brand)` mounts the SPA as a stream register on the same HTTP server, always after the business
raw routes (reason in `docs/admin/raw-http.md`). `cmd/<app>/chain_order_test.go` locks in that authentication runs
before these three links.

## Adding a cmd

When the project grows to need gRPC, event consumption or async tasks, follow `docs/guides/assembly.md` "Adding a
cmd", plus three rules:

- The constructor for the new kind of server goes in **this cmd's own directory**, not in `internal/server/` (kernel).
- `Makefile` gets a separate build target, **not merged into `build`**: `build` produces only the binary with the
  embedded SPA.
- If the new cmd also runs the permission middleware, its own `wireApp` runs the full set of `perm.MustRegister*` +
  `Seal`: the registry is per process and not shared across processes.

For message queues or async tasks, read `docs/guides/mq.md` and `docs/guides/task.md`.

## Kernel boundaries

The kernel behaves as follows in every derived repo. The business area does not open side doors around these
rules; if one seems wrong, change it in keel-admin, so that every admin keeps the same guarantees.

- **Only the kernel reads and writes the `sys_*` tables**: the business area never changes them with its own SQL; it
  goes through the kernel's ports (for example `perm.GrantRevoker` to clear grants after deleting an instance).
- **Kernel master data is never physically deleted**: `sys_user`, `sys_dept` and `sys_role` carry `deleted_at`;
  users can only be disabled, deleting a department or role only writes a tombstone, and the audit and login logs
  are append-only (`docs/admin/permission-model/function-permissions.md` §4.6).
- **Soft-deleting a role revokes what it granted, and a disabled role (`status = 0`) grants nothing**: grant queries
  count only roles that are not deleted and are enabled, while the membership relation "which roles a user holds"
  ignores status (same section, §4.6).
- **The kernel's `view` is one code for everything** (list and detail need the same right), and **the whole role
  domain is root-only** (whoever can see roles sees who can do what, and whoever can change them can raise their own
  privileges), except the two endpoints needed for assigning roles, which are bound to `sys:account:update`, and
  `ListRoleOptions` (role filter options), bound to `sys:account:view`.
- **User management cannot edit oneself**: every write a non-root user makes on their own account is refused, and
  self-service changes go through the profile page; root cannot disable itself, set its own IP allowlist or change
  its own root status either, so no one can lock themselves out or widen their own rights in passing.
- **A sensitive role has a single meaning: a non-root user cannot assign it to others**; it does not restrict root
  from editing or deleting it: only root can change roles at all, with a one-time code and an audit record.
- **Kernel writes require re-entering a one-time code (OTP) on the spot** (step-up: managing users, departments and
  roles, and changing one's own password), enforced by the `authz` middleware; a role can force its members to bind
  an authenticator (`force_otp`) (`docs/adr/admin-0007-operation-split-and-step-up-placement.md`).
- **OTP secrets are stored encrypted** (AES-GCM, key `sys.otp.aes_key`; the repository holds only a development
  placeholder).
- **The kernel tables have a fixed lock order of their own**, kept by the kernel; the order for business tables is the
  derived repo's own, and no transaction spans the two databases.

**Dependencies point one way: admin → business system.** Following `GLOSSARY.md`, a **business area / business
domain** (a management module inside the admin) and a **business system** (a service outside the admin that the
admin manages) are different things. A business system never reads the admin's `sys_*` tables or calls the admin's
endpoints; when the admin calls a business system it uses separate service-to-service credentials and never forwards
its own session token, so neither side's security depends on the other's internals.

## Anti-patterns

- ❌ Leaving out `perm.Seal()`, or calling it before some `MustRegister*`: the registry is not frozen, and a missing
  group only shows up when someone opens that page.
- ❌ Registering business codes through `perm.MustRegisterKernel`.
- ❌ A business repo borrowing the kernel's `Data` from `internal/core/data`.
- ❌ Writing business registration functions or new server constructors in `internal/server/`: the whole-directory
  overwrite wipes them.
- ❌ Changing `internal/core/` to wire a new dependency: the assembly seam is `cmd/<app>/di.go`.
- ❌ Business code writing `sys_*` tables directly, or a business system reading them.
