# Upgrading guide

This file holds entries only, one per change that actually happened; how to write entries and how to follow
upstream: `docs/upgrading-protocol.md`.

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
