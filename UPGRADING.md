# Upgrading guide

This file holds entries only, one per change that actually happened; how to write entries and how to follow
upstream: `docs/upgrading-protocol.md`.

## 2026-11-25 · 11

**What changed.** The kernel assembles itself. `server.NewKernel(config, sysConf, adminDB, redis)` builds the whole
kernel (sys data, use cases and services, the OTP cipher, captcha) and returns a `*server.Kernel` with a cleanup; its
exported fields `ScopeUseCase`, `RefRepo`, `BusinessZone` and `Brand` are what the business area takes.
`kernel.Middleware(chain, sentinelGroup)` inserts principal / authz / audit, locale and sentinel into a chain from
`mw.Default`; `kernel.NewHTTPServer(chain, businessServices, rawRoutes...)` seals the permission registry against the
kernel's and the business's services and builds the HTTP server, SPA last; `server.RegisterKernelPermissions()`
registers the kernel batch; `server.ServedOperations(businessServices)` lists what Seal checks. Gone from
`internal/server`: `KernelHTTPServers`, `ServiceNames`, `ProvideBusinessZone`, `ProvideAuthUseCase`,
`ProvideOTPCipher`, `ProvideOTPUseCase` and `NewCaptchaProvider` (entry 10's call moves into the kernel). `newApp`
leaves `cmd/<app>/main.go`; `cmd/<app>/chain_order_test.go` moves to `internal/server/` (kernel).

**Why.** About 100 lines of kernel wiring lived in each derived repository's `di.go` and `main.go`, and every kernel
change had to be copied there by hand.

**How to apply.**

1. Take the kernel (`internal/core/`, `internal/server/`, including the new `kernel.go` and `chain_order_test.go`);
   delete `cmd/<app>/chain_order_test.go`.
2. `cmd/<app>/main.go`: delete `newApp` and the imports only it used. Keep `time.Local = time.UTC` as the first line
   of `main`, the driver blank imports, loading `configs/app.yaml`, `wireApp` and `app.Run()`.
3. `cmd/<app>/di.go`, in `wireApp`: keep everything up to and including `kredis.NewRedis` (config, otel, logger,
   `appFactory`, both databases, Redis, and any infrastructure of your own). Replace everything from the kernel data
   (`coredata.NewData(db)`) through the kernel services, `refRepo`, `scopeUseCase`, `brand`, `businessZone` and the
   captcha provider with:

   ```go
   kernel, kernelCleanup, err := server.NewKernel(config, bootstrap.Get().Sys, db, redis)
   cleanups = append(cleanups, kernelCleanup)
   if err != nil {
       return nil, cleanup, err
   }
   ```

   In your business wiring, use `kernel.ScopeUseCase`, `kernel.RefRepo`, `kernel.BusinessZone` and `kernel.Brand`
   where you used `scopeUseCase`, `refRepo`, `businessZone` and `brand`. Keep `registerPermissions(...)` where it was.
4. Replace `services := httpServices(...)`, the sentinel group and the `newApp(...)` call at the end of `wireApp` with:

   ```go
   chain := mw.Default(config.Config)
   kernel.Middleware(chain, nsvc.RegisterErrors(service.NewErrorSentinel()))
   // your own chain.InsertBefore / InsertAfter calls, if newApp had any, go here
   httpSrv, err := kernel.NewHTTPServer(chain, httpServices(
       /* your business services */
   ) /* , the raw routes you passed to newApp */)
   if err != nil {
       return nil, cleanup, err
   }
   app, err := appFactory.New(httpSrv)
   if err != nil {
       return nil, cleanup, err
   }
   return app, cleanup, nil
   ```

5. `httpServices` takes only your business services and returns only theirs: drop the six kernel parameters and the
   `server.KernelHTTPServers(...)` line, start from `var services []server.HTTPService`.
6. In `registerPermissions`, replace `registerKernelPermissions()` with `server.RegisterKernelPermissions()` and
   delete the `registerKernelPermissions` function.
7. `cmd/<app>/registration_test.go`: in `servedOperations`, call
   `server.ServedOperations(httpServices(/* nil per business parameter */))` instead of
   `perm.ServedOperations(server.ServiceNames(httpServices(...))...)`; in `registerKernel`, call
   `server.RegisterKernelPermissions()`.

## 2026-11-25 · 10

**What changed.** The captcha provider is built from config instead of looked up in a global registry:
`captcha.MustRegister`, `captcha.Get`, `captcha.Names` and `captcha.ResetForTest` are gone, and so are
`biz.CaptchaGate`, `biz.NewCaptchaGate`, `biz.AuthConfig.CaptchaEnabled` and the startup errors
`captcha.provider-name-empty` / `captcha.provider-duplicate`. `server.NewCaptchaProvider(sysConf.GetCaptcha(), redis)`
returns the selected provider (with a cleanup), or nil when `sys.captcha.enabled` is false; `biz.NewAuthUseCase` and
`server.ProvideAuthUseCase` take that `captcha.Provider` as their last argument. With captcha off, the anonymous
`POST /v1/sys/auth/captcha` no longer writes a challenge to Redis.

**Why.** Both providers were registered on every start, and the anonymous captcha endpoint kept issuing challenges
with captcha off.

**How to apply.**

1. Take the kernel (`internal/core/`, `internal/server/`).
2. In `cmd/<app>/di.go`, replace the two `captcha.MustRegister` calls, the turnstile cleanup, the `cap.GetEnabled()`
   self-check block and `biz.NewCaptchaGate(...)` with:

   ```go
   captchaProvider, captchaCleanup, err := server.NewCaptchaProvider(sysConf.GetCaptcha(), redis)
   if err != nil {
       return nil, cleanup, err
   }
   cleanups = append(cleanups, captchaCleanup)
   ```

   and pass `captchaProvider` where `captchaGate` went.

## 2026-11-25 · 9

**What changed.** The required `sys.*` keys are protovalidate rules in `internal/conf/app.proto`: `sys`, `sys.brand`
and `sys.otp` are required messages, `sys.brand.title`, `sys.business_time_zone` and `sys.otp.aes_key` required
strings. A missing one now fails at config load as `keel/kconfig.invalid` naming the field; the startup errors
`sys.brand-title-missing`, `sys.business-zone-missing` and `sys.otp-key-missing` are gone, and so is
`server.ProvideBrand` (use `sysConf.GetBrand()`). `make config` compiles `app.proto` with
`third_party/protovalidate` as a second module of an inline buf workspace.

**Why.** The config's value constraints belong on the proto, checked once at load, not in Go constructors.

**How to apply.**

1. Take the kernel (`internal/server/`).
2. Copy the three `required` rules, the `buf/validate/validate.proto` import and the field comments into your
   `internal/conf/app.proto`; replace the `config` target and its two variables `CONF_BUF_CONFIG` /
   `CONF_GEN_TEMPLATE` in your `Makefile` with upstream's; run `make config`.
3. In `cmd/<app>/di.go`, replace `brand, err := server.ProvideBrand(sysConf)` and its error check with
   `brand := sysConf.GetBrand()`.
4. Anything matching the three removed IDs (tests, alerts) matches `keel/kconfig.invalid` instead.

## 2026-11-25 · 8

**What changed.** The operator has one type, `*perm.Principal`: `biz.Actor` (kernel `internal/core/sys/biz`) is gone
and the kernel use cases take `*perm.Principal`. `perm.Current(ctx)` returns the request's principal, or
`perm.ErrNoPrincipal`, which the kernel's sentinel group maps to 401 `UNAUTHENTICATED` ("sign in first") instead of the
login page's "invalid username or password". The business area's `actorFromContext` and `errNoPrincipal` in
`internal/service/errors.go` are gone.

**Why.** Two types carried the same three fields, and a call without a principal answered with a login error.

**How to apply.**

1. Take the kernel (`internal/core/`).
2. In `internal/service/`: replace `actorFromContext(ctx)` with `perm.Current(ctx)` and delete `actorFromContext`,
   `errNoPrincipal` and `var _ = actorFromContext` from `errors.go`.
3. In your own code: replace `*biz.Actor` / `&biz.Actor{...}` with `*perm.Principal` / `&perm.Principal{...}`.

## 2026-11-25 · 7

**What changed.** Derived data-scope resources are gone: `perm.Derive`, `perm.ResourceDef.Via`, the startup errors
`perm.resource-parent-unknown` and `perm.resource-nested-derivation`, and the `via_parent` / `via_fk` fields of
`ResourceDef` in `api/keel_admin/v1/meta.proto` (now `reserved 4, 5`; the frontend type loses `viaParent` / `viaFk`).

**Why.** `Via` was only checked at `Seal`; the data scope never read it, so it promised a mechanism that did not
exist. A child table without ownership columns registers no resource: its query takes the parent resource's scope and
filters by the foreign key (`docs/admin/permission-model/data-scope.md` §5.1).

**How to apply.** Take the kernel (`internal/core/`, `api/keel_admin/v1/`, then `make all`). Delete each `ResourceDef`
that sets `Via`, together with its i18n name entry; where a query resolved that resource, resolve the parent resource
instead and filter by the foreign key.

## 2026-11-25 · 6

**What changed.** `perm.Permission` loses its `RequireOTP` field, and the startup error `perm.require-otp-manual` is
gone; `perm.RequiresOTP(code)` answers whether a code covers a step-up operation (derived by `Seal` from the bindings).

**Why.** The field was an output of `Seal` sitting in an input struct, kept honest only by a startup rule.

**How to apply.** Take the kernel (`internal/core/`). Replace a read of `perm.All()[code].RequireOTP` in your own
code with `perm.RequiresOTP(code)`; step-up is still declared only by `RequireOTP` on the `Requirement` in your
bindings.

## 2026-11-25 · 5

**What changed.** Five pass-through constructors are gone: `server.ProvideSysConf`, `server.ProvideOTPGate`,
`server.ProvideAuditRecorder`, `data.NewUserRepo` and `data.NewCacheInvalidator` (kernel `internal/core/sys/data`);
`biz.AuthUseCase.Resolve` is gone too (`ResolveWithVersion` stays). The LIKE keyword escaper is now
`coredata.ContainsPattern` in `internal/core/data`, and the sentinel helper `localed` is now `i18n.Localed` in the new
kernel file `internal/i18n/localed.go`: one copy shared by the kernel and the business area. On the frontend,
`RowActions` and its types moved from `components/data-table.tsx` to `components/row-actions.tsx`; `PasswordInput`,
`OtpInput`, `OtpField` and `OTP_LENGTH` from `components/containers.tsx` to `components/credential-inputs.tsx`;
`deptOptionsQueryOptions` to `lib/depts.ts` (the duplicate `deptTreeQueryOptions` in `lib/roles.ts` is gone);
`roleOptionsQueryOptions` from `lib/query.ts` to `lib/roles.ts`; `getTimeOfDay` and `urlToFilterValues` are gone.

**Why.** Each removed constructor only returned its argument; passing the value directly says the same in di.go.
The escaper and `localed` were copied between the kernel and the business area.

**How to apply.**

1. Take the kernel (`internal/core/`, `internal/server/`, `internal/i18n/localed.go`, the frontend kernel).
2. In `cmd/<app>/di.go`: `sysConf := bootstrap.Get().Sys`; pass `userAdminRepo` where `userRepo` went, `otpUseCase`
   where `otpGate` went, `permCache` where `cacheInvalidator` went and `auditUseCase` where `auditRecorder` went, and
   delete those four variables.
3. In your business code: replace a local LIKE escaper with `coredata.ContainsPattern(keyword)` (import
   `coredata "<module>/internal/core/data"`) and your copy of `localed` in `internal/service/errors.go` with
   `i18n.Localed`; keep `var _ = actorFromContext` if nothing else calls it yet. Fix frontend imports of the moved
   names as `pnpm lint` reports them.

## 2026-11-25 · 4

> Follows keel-layout 679fd27

**What changed.** The `Makefile` no longer sets `VERSION` or passes `-X main.Version=...` to the linker (no `main.go`
declares that variable; the version a service reports is `app.version` in `configs/app.yaml`), and `make generate`
runs only `go mod tidy` (the repository has no `//go:generate` directives). `internal/testdb` exports
`Root(t testing.TB)`, the repository root; `e2e/harness/harness.go` uses it instead of its own `harness.Root`, which
is gone, and so imports `internal/testdb` under this repository's module path: its line in
`scripts/cross-repo-sync.list` becomes a `norm` entry. `internal/i18n/bundle.go` uses `errors.AsType`.

**Why.** The linker flag and `go generate` did nothing; the root lookup was written twice in two shared files.

**How to apply.**

1. In your `Makefile`, delete the `VERSION?=` line, ` -X main.Version=$(VERSION)` from `GO_BUILD` and
   `go generate ./...` from `generate`, unless you added a `Version` variable or `//go:generate` directives yourself.
2. Change `keel-admin e2e/harness/harness.go` in `scripts/cross-repo-sync.list` to end in ` norm`, then take both
   shared files with `make sync-upstream`; replace any `harness.Root(t)` of your own with `testdb.Root(t)`.

## 2026-11-25 · 3

**What changed.** `make e2e` arrives: `e2e/` (build tag `e2e`) builds `cmd/admin`, runs it as a process against the
test servers and tests it over HTTP, once per row of the table `rounds` in `e2e/main_test.go` (template: both
databases on MySQL, both on PostgreSQL). `configs/config.yaml` gains placeholders, each defaulting to its old value:
`${LOGGER_FORMAT}`, `${HTTP_ADDR}`, `${DB_DEFAULT_DRIVER}`, `${DB_DEFAULT_SOURCE}`, `${DB_ADMIN_DRIVER}`,
`${DB_ADMIN_SOURCE}`, `${REDIS_ADDR}`. `docs/admin/testing.md` gains "End-to-end tests".

**Why.** The end-to-end layer runs the binary that ships on every database round, replacing the interface part of the
manual test run.

**How to apply.**

1. Add the placeholders to your `configs/config.yaml`, keeping your values as the defaults; `make e2e` fills them
   through `KEEL_` variables.
2. Take `e2e/` and the `e2e` target of the `Makefile`; edit `rounds` to the drivers you deploy (the admin and business
   databases may differ), and run one round with `make e2e ARGS='-run /<name>'`.
3. The kernel cases use only kernel codes and read the cookie name, brand title and business time zone from your
   config. The cases about notices, data scope (the kernel registers no resource) and the Japanese UI carry example
   markers; if you deleted the examples, leave the marked parts out. Then run `make e2e`.

## 2026-11-25 · 2

> Follows keel-layout 3e227cf

**What changed.** The shared end-to-end harness arrives: `e2e/harness/harness.go` (a shared file) builds `cmd/*`, runs
the binaries as processes with only the committed configs and `KEEL_` variables, collects their JSON log and stops
them with SIGTERM; `internal/testdb` gains `Create(t, database, dialect, name)`, which builds one named database and
returns its DSN; `docs/guides/testing.md` describes the end-to-end layer in full. This repository's own `e2e/` cases,
round table and `make e2e` are still to come.

**Why.** The end-to-end layer runs the binary that ships, on every database round, instead of a manual test run.

**How to apply.**

1. Add `keel-admin e2e/harness/harness.go` to `scripts/cross-repo-sync.list` (with your direct upstream in the first
   field) and take it and the new `internal/testdb/testdb.go` with `make sync-upstream`.
2. Nothing else until this repository's `make e2e` lands.

## 2026-11-25 · 1

> Follows keel-layout 3d9374f

**What changed.** Tests come in three layers, one command each: unit tests (`make test`, Go and the frontend's
vitest), narrow integration tests (`make integration`) and end-to-end tests (`make e2e`, still to come); the terms are
in the shared segment of `GLOSSARY.md` and the rules in `docs/guides/testing.md`. `make check` runs static checks only,
no test (still including the frontend's `pnpm lint` and `pnpm build`), and vets the `integration` and `e2e` build tags
too. `make integration` needs all four of `TEST_MYSQL_DSN`, `TEST_POSTGRES_DSN`, `TEST_REDIS_ADDR` and `TEST_NATS_URL`,
pointing at servers for tests only; `TEST_REDIS_ADDR` is new to the kernel's tests: the login lockout and the
authenticator code replay guard (`internal/core/sys/data/`) are now narrow integration tests on a real Redis, database
0, and the `github.com/alicebob/miniredis/v2` dependency is gone. The test commands build with `CGO_ENABLED=0`, and add
`-race` only on macOS. The kernel's `internal/core/testdb` moved to the shared package `internal/testdb` (build tags
`integration || e2e`), same `testdb.ForEachDB`. The admin database and the business database pick their drivers
independently (`docs/admin/domain-model.md`, "Schema"), and `cmd/admin` blank-imports both `kdb/mysql` and
`kdb/postgres`, so switching either database is a config change.

**Why.** One command per layer: static checks and unit tests need no server and run before every commit, the slower
layers run when a change calls for them. Real servers instead of emulators, because an emulator differs from the real
server in details. Nothing joins across the two databases, so they need not share a dialect.

**How to apply.**

1. Wherever only `make check` ran (before commits, in CI), run `make check test`.
2. Set `TEST_REDIS_ADDR` and `TEST_NATS_URL` next to the two DSNs for `make integration`, all on servers for tests only.
3. `make sync-upstream`: it writes the kernel and `internal/testdb/testdb.go` (add
   `keel-layout internal/testdb/testdb.go` to `scripts/cross-repo-sync.list` first, with your direct upstream in the
   first field). Delete `internal/core/testdb/`, and in your own `*_integration_test.go` files (at least
   `internal/data/testdb_integration_test.go`) change the import to `<module>/internal/testdb`.
4. Add `_ "github.com/go-keel/keel/core/kdb/postgres"` next to `kdb/mysql` in `cmd/<app>/main.go`, then
   `go mod tidy`; or keep only the drivers your two databases use.
5. Replace emulators in your own tests (SQLite, miniredis, an embedded NATS server) with the real test servers.

## 2026-11-24 · 1

> Follows keel-layout 6ae3cd9

**What changed.** The admin runs on MySQL or PostgreSQL. Schema scripts moved into one directory per dialect:
`migrations/admin/{mysql,postgres}/0_schema.sql` (kernel) and `migrations/business/{mysql,postgres}/0_schema.sql`. Data
tests no longer use in-memory sqlite: every data-layer test is a `*_integration_test.go` on real databases, through the
kernel's new `internal/core/testdb` (`testdb.ForEachDB`), once per dialect every `migrations/<database>/` has; the
`github.com/glebarez/sqlite` dependency is gone. Each package's `openTestDB(t)` became `forEachDB(t, fn)`.

**Why.** Keel's database component supports both databases, but the admin only ever ran on MySQL, and sqlite hid
MySQL behaviour from the data tests (reserved words, locking, case-insensitive usernames). Testing on the dialects a
repository actually runs catches it.

**How to apply.**

1. `make sync-upstream`: it writes the kernel (`internal/core/`, including `internal/core/testdb/`), both dialect
   directories of `migrations/admin/`, and the shared docs.
2. Move `migrations/business/*.sql` (and any other business database's scripts) into `migrations/<database>/mysql/`,
   or `postgres/` if you run on PostgreSQL. Tests run on the dialects every database directory has, so keep the other
   dialect only if you mean to support it.
3. Replace `internal/data/testdb_integration_test.go` with upstream's (a `forEachDB` wrapping `testdb.ForEachDB`), and
   in each business `*_integration_test.go` wrap the body as
   `forEachDB(t, func(t *testing.T, db *gorm.DB) { … })` instead of `db := openTestDB(t)`. Business tests that ran on
   sqlite move the same way, renamed to `*_integration_test.go` with `//go:build integration`; seed rows then need
   every `NOT NULL` column without a default.
4. `go mod tidy`, set `TEST_POSTGRES_DSN` if you kept PostgreSQL, and run `make integration`.

## 2026-11-19 · 3

**What changed.** `UserSettings` (`api/keel_admin/v1/auth.proto`) belongs to the kernel alone; field numbers from 100
are no longer given to derived repositories.

**Why.** The message is in a kernel file that following upstream overwrites, and the kernel's settings code reads and
writes only its own fields, so every favorite change dropped a field a derived repository had added.

**How to apply.** If you added fields to `UserSettings`, move them to a business table keyed by user ID with per-item
endpoints of your own (`docs/admin/auth.md`, "User settings"), copying existing values out of `sys_user.settings`
before following this version. Otherwise nothing.

## 2026-11-19 · 2

**What changed.** The kernel's contracts have a package of their own that no rename touches: `api/keel_admin/v1/` holds
`auth`, `user`, `role`, `dept`, `audit`, `meta`, the kernel's `error_reason.proto` and `types.proto`, the same in every
repository and registered in the sync list. Business contracts live in the business package `api/<pkg>/v1/` with an
`error_reason.proto` of their own; reasons no longer share one enum and no longer follow number ranges, but a business
reason's name must not reuse a kernel reason's. The template's business package is `api/admin_app/`, and `make rename`
renames only it. `scripts/gen-error-reason.mjs` reads every package's `error_reason.proto`.

**Why.** The kernel contracts sat in the package the rename gives each repository, so they could not be taken from
upstream like the rest of the kernel, and the kernel's operation names differed per repository.

**How to apply** (in your repository `<pkg>` is the business package, so far shared with the kernel):

1. **If `<pkg>` is `keel_admin`** (the old rename gave it to a module ending in `keel-admin`), move the business
   contracts out first, before anything is synced: pick a new package name, `git mv` every proto in `api/keel_admin/v1/`
   except `auth`, `user`, `role`, `dept`, `audit` and `meta` to `api/<new>/v1/`; change their `package`, `go_package`
   and imports, and the Go and frontend code using them. From here on `<pkg>` is the new name. Skipping this loses the
   business reasons when the next step overwrites the kernel's `error_reason.proto`. HTTP paths come from the proto
   annotations and stay; if other programs call your business services over gRPC, their full names change to
   `api.<new>.v1.…`, so those callers switch to clients generated from the new package in the same release.
2. Enable the new `api/keel_admin/v1/*.proto` lines of upstream's `scripts/cross-repo-sync.list` in yours and run
   `make sync-upstream`: it writes the kernel contracts into `api/keel_admin/v1/` and the kernel code importing them.
3. Delete `auth`, `user`, `role`, `dept`, `audit` and `meta` from `api/<pkg>/v1/` (not in this repository when step 1
   ran). Keep `api/<pkg>/v1/types.proto`: it is now the business package's own, `NamedRef` included, and business
   protos keep importing it; delete it only if no business proto imports it. Business protos never import
   `keel_admin/v1/` (`docs/admin/api-endpoint.md`). In `api/<pkg>/v1/error_reason.proto` keep only your business
   reasons (with a zero value such as `REASON_UNSPECIFIED = 0;`), and rename any whose name a kernel reason uses.
4. Business Go and frontend code keeps using your own package; only code that used a kernel endpoint's types imports
   `<module>/api/keel_admin/v1` or writes `api.keel_admin.v1.<Type>`.
5. If your repository will itself be forked, take upstream's `scripts/rename.sh` and set `TEMPLATE_MODULE`,
   `TEMPLATE_CMD` and `TEMPLATE_DOMAIN` at its top to your own values (`README.md`); the old script finds both
   `api/<pkg>/` and `api/keel_admin/` and stops.
6. `make all`, then `make check`.

Data: the kernel's operation names change from `/api.<pkg>.v1.…` to `/api.keel_admin.v1.…`. Audit logs store the
action without the package (`RoleService.CreateRole`) and permission bindings are code, so nothing stored changes; check
only code of your own that stored full operation names.

Callers: a Go program that links its own kernel (another admin) and imports another admin's
`<module>/api/keel_admin/v1` client panics before `main` (`keel_admin/v1/… is already registered`). It uses its own
`api/keel_admin/v1` client, pointed at the other admin's address, instead; the kernel contracts are the same while both
follow the same upstream.

## 2026-11-19 · 1

> Follows keel-layout 48afc26

**What changed.** Following upstream no longer reruns `make rename`. `make sync-upstream` writes upstream's shared
files and `norm` kernel trees into this repository, already carrying its module path (the same normalization
`make check-sync` compares with), deletes what upstream deleted in a directory entry, and only reports
shared segments. `make rename` runs once, when the repository is created, and refuses to rename a repository that
already has its own module path.

**Why.** Rerunning the rename over the whole repository rewrote the repository's own files by text on every upgrade;
only the files taken from upstream ever need the names changed.

**How to apply.**

1. Take `scripts/check-cross-repo-sync.sh` from upstream by hand this once (it carries no names); step 3 writes the
   other shared files, `docs/upgrading-protocol.md` included.
2. Add the `sync-upstream` target to your `Makefile`, as upstream's: `@WRITE=1 scripts/check-cross-repo-sync.sh`.
3. From now on follow with `make sync-upstream` instead of copying trees and rerunning `make rename`.

## 2026-11-18 · 2

> Follows keel-layout 48afc26

**What changed.** A repository may vendor `.proto` files Keel does not ship into directories of its own under
`third_party/`. `scripts/sync-proto.sh` now replaces only the four directories it owns (`keel`, `protovalidate`,
`googleapis`, `kratos`) and leaves any other directory there alone, and `buf.yaml` is no longer a shared file: each
repository adds a module per directory of its own.

**Why.** A contract importing, say, `google/type/money.proto` had nowhere to put it: `make proto` deleted all of
`third_party/`, and `buf.yaml`, where the new directory must be declared, had to match upstream verbatim.

**How to apply.**

1. Take `scripts/sync-proto.sh` and the shared docs from upstream (`make check-sync` lists them).
2. Delete the `keel-admin buf.yaml` line from `scripts/cross-repo-sync.list`. Keep `buf.yaml` as it is; from now on
   it is yours, and later upstream changes to it arrive as ledger entries.
3. For each extra `.proto` your contract imports, put it at a fixed version under `third_party/<name>/` and add a module
   for that directory to `buf.yaml`, with the same lint and breaking ignores as the others.

## 2026-11-18 · 1

**What changed.** The permission registry is sealed against every operation of the services mounted on the HTTP
server, not against every `api.*` service linked into the binary. The HTTP registration list now names its services:
`server.KernelHTTPServers` and each business entry return `server.HTTPService{Name, Register}`, with `Name` taken from
the generated `<Service>_ServiceDesc.ServiceName`. `perm.ContractOperations()` is replaced by
`perm.ServedOperations(names...)`, and `perm.Seal` moved from `di.go` into `newApp` in `cmd/<app>/main.go`, which now
takes `[]server.HTTPService` instead of an `api.HTTPServerRegister`.

**Why.** A repository that calls another service links that service's proto in, and its operations were demanded a
binding: startup failed with `perm.operation-unowned` for operations this repository does not serve.

**How to apply.**

1. Take the kernel trees (`internal/core/`, `internal/server/`) from upstream, and change `cmd/<app>/main.go` the way
   upstream's does: it is not in the sync list, because its directory carries your entry name.
2. In `internal/service/http.go`, make each business function return `server.HTTPService`: `Name` from
   `v1.<Service>_ServiceDesc.ServiceName`, `Register` the old registration function.
3. In `cmd/<app>/di.go`, delete the `perm.Seal(perm.ContractOperations())` block; keep `registerPermissions(...)`.
   Replace the HTTP registration list with a function `httpServices(...)` that takes the services and returns
   `[]server.HTTPService` (kernel list from `server.KernelHTTPServers`, then one `append` per business module), and
   pass its result to `newApp`, as upstream's `di.go` does.
4. In `cmd/<app>/registration_test.go`, seal with `servedOperations(t)` as upstream does: it passes the names of
   `httpServices` (called with nil services) to `perm.ServedOperations`. Each business module passes `nil` there for
   each parameter it adds to `httpServices` or `registerPermissions`.
5. Delete any local workaround that filtered `perm.ContractOperations()` for called-only packages.
