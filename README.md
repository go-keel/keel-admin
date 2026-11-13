# Keel Admin

An admin back-office template (fork it, rename, and start): a Go backend and a React SPA shipped as one binary,
built on the Keel framework. It is an admin specialization **with keel-layout as its base template**: all of
keel-layout's layering contracts and practices still apply here, and this repository only adds what an admin needs.

Built in:

- **Authentication**: JWT session (`api.NewHTTPServer` builds it from the server's own authentication config, with no
  separate mounting) + TOTP two-step verification (bind/verify/forced by role, secrets stored AES-GCM encrypted)
- **Permissions**: RBAC (department tree + roles + permission bindings in code, `internal/core/perm`) + data scope
  (role × resource scope, one `ApplyScope` line per query; including explicit assignment, role × a set of resource
  instances such as grants per category or group, with notice_category as the complete example) + assignable-role
  and manageable-department boundaries (ARBAC)
- **Login protection**: brute-force lockout + optional human verification (two built-in providers, an image code and
  Cloudflare Turnstile, configured in the `sys:` node of app.yaml, read once at startup, restart after a change)
- **Audit**: audit log of changes + login log (recorded automatically by the `audit` middleware)
- **Errors**: biz sentinel errors → `error_reason.proto` enum → sentinel translation (i18n en/zh), so the frontend
  gets a meaningful code/reason
- **SPA hosting**: `web/dist` is mounted on the same HTTP server through streamRegister (embedded with
  `-tags embed_ui`); API routes take precedence
- **A re-skinnable shell**: appearance presets (named bundles of CSS variables) and color mode switch at runtime and
  can be locked by a derived repo; the menu list, shell switches and the login page image are each an assembly seam
  changed in one place

## Project structure

```
├── api/keel_admin/v1/    # proto contracts (including error_reason)
├── cmd/admin/            # entry main.go + hand-written assembly di.go
├── configs/              # config.yaml (infrastructure) + app.yaml (the kernel's sys: node): the real production
│                         #   config; the repo commits development placeholders, the deployment layer injects real
│                         #   values; *.dev.yaml are local overrides only
├── migrations/           # .sql table scripts run by hand, one directory per database: admin/, business/
├── internal/
│   ├── core/             # kernel (read-only; upgrade = overwrite the directory + rerun make rename): perm
│   │                     #   permission registry, permdata ApplyScope, middleware (principal/authz/audit), captcha,
│   │                     #   the sys system domain (accounts/roles/departments/audit/OTP), data connections and
│   │                     #   transactions
│   ├── biz|data|service/ # business area (keel-layout layering, notice is the example)
│   ├── conf/             # application config proto (app.proto, the sys: node + business fields)
│   ├── server/           # HTTP registration, SPA/brand mounting, providers
│   └── i18n/             # error messages en / zh-CN
└── web/                  # React SPA (make build-web → web/dist): Vite + TanStack Router/Query/Table + shadcn/ui +
                          #   Tailwind; zones (kernel / generated / third-party components / assembly seams /
                          #   business area): docs/admin/shell.md, "Frontend zones"
```

## Quick start

The frontend needs Node.js ≥24; `web/package.json` pins pnpm to 12.14.0. pnpm settings are in
`web/pnpm-workspace.yaml`; `.npmrc` is only for the registry and authentication.

```bash
make init          # install buf (the other plugins are pinned)
(cd web && pnpm install)   # make proto uses the frontend's openapi-typescript
make all           # proto + config + generate

# Create tables (a new database runs the complete schema in migrations/<database>/0_schema.sql by hand; a local
# example; the root seed is built into the end of the script and is idempotent). Two databases: the business
# database keel-biz (database.default) and the admin database keel-admin (database.admin); config.yaml requires both.
# root's initial password is admin888: on every new deployment, sign in as root and change it in the personal center
# right away. Do not edit the script: it is kernel, and the next upgrade overwrites it.
mysql -h127.0.0.1 -uroot -p -e 'CREATE DATABASE IF NOT EXISTS `keel-admin`; CREATE DATABASE IF NOT EXISTS `keel-biz`;'
mysql -h127.0.0.1 -uroot -p keel-admin < migrations/admin/0_schema.sql
mysql -h127.0.0.1 -uroot -p keel-biz   < migrations/business/0_schema.sql

make build-web     # build the frontend (web/dist)
make build         # build bin/admin
# Needs MySQL and Redis (127.0.0.1:6379). Put your own connection strings in configs/config.dev.yaml
# (copy configs/config.dev.example.yaml); it is not committed.
./bin/admin        # :8080
```

Tests run without any infrastructure:

```bash
go test ./...
```

## Fork into your own repo

**Rename before generating anything.** `make all` produces `*.pb.go`, `openapi.yaml` and `web/src/generated/`
carrying `github.com/go-keel/keel-admin`, and the rename script **does not touch generated artifacts** (sed on
`.pb.go` corrupts rawDesc and only blows up at runtime); in the wrong order you have to regenerate again.

```bash
make init                             # install buf
(cd web && pnpm install)              # make proto uses openapi-typescript; the rename script formats the frontend with Biome
make rename MODULE=example.com/demo   # replaces the module path and the proto package under api/ in one go; idempotent
make init-hooks                       # install the commit and merge hooks in .githooks/ (block changes to the propagation surface without an UPGRADING.md entry, check dead links)
make all                              # only now does generation produce your own module path
```

The proto package defaults to the last segment of the module path: `example.com/demo` gives `package api.demo.v1`,
and the directory under `api/` becomes demo; `-`, `.` and the like in that segment become `_` (`example.com/shop-api`
gives `api.shop_api.v1`), and when the last segment is a version suffix like `/v2` the one before it is used. It goes
into the operation names of endpoints (permission bindings and audit logs record by them), OpenAPI type names and
the generated frontend types, and is the service's public name; to choose another, add `API_DOMAIN=xxx`. To rename
the entry directory and the binary (`cmd/admin`, `bin/admin`) too, add `CMD=xxx`.

`make rename` must be rerun after every kernel overwrite and scans all docs in the repository each time. Your own
docs that record "how to follow upstream" (a kernel sync manual and the like) mention
`github.com/go-keel/keel-admin`, `api/keel_admin/v1/` and `cmd/admin` meaning upstream, and these must not be
replaced: write `<!-- rename:skip -->` on the **first line** of such a file and the rename script skips it entirely.
Without it, the replacement raises no error, but the manual's commands then point at your own repo, found only the
next time someone follows them.

**Leave the convention docs and the shared segments of the entry files unchanged.** `docs/guides/`,
`docs/practices/`, `docs/admin/`, the ADRs under `docs/adr/` that carry a shared marker,
`docs/upgrading-protocol.md`, `scripts/check-doc-links.sh` and `scripts/check-cross-repo-sync.sh`, and the two segments delimited by `shared segment begin` /
`shared segment end` in `AGENTS.md` and `GLOSSARY.md` (keel-layout's and keel-admin's) are verbatim copies: follow
them, do not rewrite them; `make check-sync` guards them (trade-off in
`docs/adr/layout-0002-verbatim-shared-docs-and-segmented-entry-files.md`). Write your own rules outside the
segments of the two entry files; longer ones go in a directory of your own under `docs/` (not in `docs/guides/` or
`docs/admin/`: those directory entries require the same file set on both sides), registered in the routing table
outside the segments of `AGENTS.md`.

**Switch from `lint-strict`, which this repo uses on itself, to the derived-repo tier `lint`**, in two places: the
`check` target in `Makefile` and the `make lint-strict` command in `.githooks/pre-commit`. If your repo will itself be
forked, keep `lint-strict` instead and change `keel-admin` in its patterns to your repository's directory name.
`lint-strict` checks everything and reports the paths of your own business area; `lint` checks only what upstream is
responsible for (see `docs/adr/layout-0005-doc-link-check-distribution-boundary.md`). Also delete the
`scripts/check-english.sh` line in the `check` target: requiring English is this template's own rule, and your
service writes in its team's language. Do this before deleting the examples, or the final `make check` below fails.

**A repo nobody will fork** writes no ledger entries of its own: delete the ledger check (the first `if` block) in
`.githooks/pre-commit`, or nearly every commit is stopped. Its `UPGRADING.md` then holds only "following upstream"
entries.

**Enable the cross-repo comparison list.** In the active group of `scripts/cross-repo-sync.list` ("Shared files whose
source is keel-layout"), replace every source repository field `keel-layout` with `keel-admin` (your direct upstream,
the only one you compare with), then change the leading `#keel-admin` of every commented line to `keel-admin`. Left
undone, `check-sync` passes without comparing anything.
Put a clone of keel-admin in a sibling directory (or point `UPSTREAM_DIR` at its
parent directory) and run `make check-sync` once; it should pass.

**Replace the template identity with your own.** `make rename` changes only Go, proto, frontend code and docs, not
YAML, HTML or JSON: `app.name`, `app.domain` and the database names in the two `database` sources in
`configs/config.yaml`, the database names in `configs/config.dev.example.yaml`, `sys.brand.title` in `configs/app.yaml`,
the session cookie name `cookie_name` (`keel_session`) in `configs/config.yaml`, the `<title>` in `web/index.html`, and
`name` and `description` in `web/package.json` still carry keel-admin's own name; also replace `web/public/logo.svg` (Keel's icon),
and set `sys.business_time_zone` in `configs/app.yaml` to the IANA time zone your business counts days in. Database
names go into database connections and the application name into logs and traces; left unchanged, your service still
calls itself keel-admin at runtime; miss the dev example, and a local override copied from it changes the database names
back. The title and introduction of `GLOSSARY.md` outside the segments still describe the template; rewrite them for
your service. The two database names in the table commands of "Quick start" in this file carry the template's names
too; change them. Rewrite the whole section "This repository: keel-admin" outside the segments of `AGENTS.md` and
`GLOSSARY.md` as your own (module name, where the example code is...); keel-admin inside the segments and in the shared
docs means the upstream template, keep it.

### Delete the examples

The template carries two examples; delete them completely after forking: notices (notices and categories,
through endpoints, permission codes, explicit assignment in the data scope, audit and frontend pages), and the Japanese
UI (demonstrating "add a UI language"). The boundary of the examples is marked with comments, and only where the
compiler cannot help:

- **"Example file: delete the whole file after forking"** at the top of a file: delete the file (step 1).
- **"Example: delete after forking"** at the end of a line: delete that line.
- **"Example: delete after forking"** on a line of its own: delete it and the comment lines and the one declaration,
  field, entry or config key directly below it.
- **"Example: delete after forking (…)"** with a range in parentheses ("up to the end of this CREATE TABLE", "the
  whole subtest"…): delete down to there.

In every step, when an error sits inside a marked block, delete the whole block; after every deletion, delete the
imports the compiler now reports as unused. Three steps, in this order:

1. **Delete whole example files.** Delete every file whose header says "Example file: delete the whole file after
   forking", then regenerate (`make proto` first clears the generated artifacts under `api/`, so a deleted proto
   leaves no orphans):

   ```bash
   grep -rl "Example file: delete the whole file after forking" api internal migrations web/src --exclude='*.pb.go' | xargs rm
   make proto
   ```

2. **Fix the interwoven places by compile errors.** In this step the compiler is the marker; it reports one batch
   per run, so delete and run again until it passes:
   - Backend: `go vet ./...` (which compiles test files too) points at the broken references in
     `internal/data/model.go`, `internal/service/http.go`, `cmd/admin/di.go` and `cmd/admin/registration_test.go`;
     delete them one by one (`model.go` stays as the home of your POs). For `cmd/admin` it reports one error per run,
     so expect several rounds.
   - Frontend: `(cd web && pnpm build)`; the build's router plugin first rewrites `web/src/routeTree.gen.ts` for the
     remaining pages, then reports the broken imports in `web/src/languages.ts`; once that passes, run `(cd web && pnpm format)` and then `(cd web && pnpm lint)`,
     whose formatting check runs before the type check; the type check then points at the menu items in
     `web/src/nav.ts` and the workbench shortcuts in `web/src/routes/_app/index.tsx` that reference deleted pages.

   When the compiler reports `bizDB` and `scopeUseCase` as "declared and not used", **do not delete them**: the
   former is the business database connection (both databases are required, see
   `docs/adr/admin-0002-two-databases-and-default-ownership.md`), the latter the resolver of business data scopes;
   the first business module needs both. Hold them with a `_ = bizDB` / `_ = scopeUseCase` line after their
   construction, and delete that line when you wire in the first business module.

3. **Delete the leftovers the compiler cannot see.** Error codes, sentinel errors and their mappings, i18n entries,
   table statements, menus and frontend text still compile after the examples are gone. Delete each place marked
   "Example: delete after forking" (where a range is given in parentheses, delete up to there):

   ```bash
   grep -rn "Example: delete after forking" api internal migrations cmd scripts web/src --exclude='*.pb.go'
   ```

   Once there are no hits, run `go vet ./...` again and delete the imports it reports as unused, run
   `gofmt -w internal cmd`, then regenerate with `make proto` and `make perms` (the frontend's permission codes come
   from the backend registration), and run `(cd web && pnpm format)` so Biome reorders the remaining imports (`pnpm
   lint` reports the unused ones for you to delete). The shared convention docs do not reference the examples and need
   no change; the only places pointing at them are the "Example code" section outside the segments of `AGENTS.md` and
   the entry mentioning notices outside the segments of `GLOSSARY.md`: delete them or replace them with your own. The
   everyday `lint` does not check business area paths; to make sure no dead links remain outside the segments, run
   `STRICT=1 scripts/check-doc-links.sh` once (it also reports your own business area paths; look only at those
   related to the examples). In this README, rewrite the introduction and "Project structure" (both name the notices
   example) and "Quick start" for your service; "Delete the examples" can go, while "Fork into your own repo" stays:
   `make rename` points at it, and you run it again on every kernel upgrade.

   What is left empty stays as the place for your first module: `internal/biz/errors.go`, `internal/data/model.go`
   and `internal/service/http.go` with only their package clause and comments, `migrations/business/0_schema.sql`
   with only `SET NAMES`, the empty `nav: {}` in `web/src/locales/app/` and the `registerTimeZones()` call with no
   entries. Delete the emptied directory `web/src/routes/_app/notices/`. Finally run `make check`.

Both greps exclude `*.pb.go`: the marker comments are carried into generated artifacts, and the hits there vanish
on regeneration; do not edit generated artifacts by hand.

**Start the ledger, then commit.** Delete every entry in `UPGRADING.md`, keeping its introduction: they are upstream's
history, already in your fork. Add the first "following upstream" entry, your sync point (`docs/upgrading-protocol.md`):

```markdown
## <today> · 1

> Follows keel-admin <short hash of the keel-admin commit you forked>

Forked with `make rename MODULE=<your module>` (plus any `API_DOMAIN=` / `CMD=` you passed); every kernel overwrite reruns it the same way.
```

Then commit everything with `git add -A && git commit`: the ledger is in the commit, so the hook lets it through.

From here on work per `AGENTS.md`, `docs/guides/` and `docs/admin/`; to follow upstream changes read `UPGRADING.md`
(how entries are written and how pending ones are counted: `docs/upgrading-protocol.md`).

## Middleware chain

```
recovery → tracing → metrics → logging → metadata → locale
        → auth (built by the server from its own authentication config)
        → validate → principal (claims → principal) → authz (bindings in code + forced OTP)
        → audit (change audit) → sentinel (error translation)
```

Adding a protected endpoint: define the RPC in proto → declare permission codes in biz / endpoint bindings in
service → implement biz/data/service with keel-layout's layering → register and wire in `di.go` (step-by-step
manual: `docs/admin/permission.md`).

## Development conventions

- Standing contract and task routing: `AGENTS.md` (shared by AI agents and developers). Segments: keel-layout's
  shared segment (layering contracts and general rules) → keel-admin's shared segment (how to use the kernel, admin
  rules and routing table) → this repository's own text outside the segments
- Guides: `docs/guides/`; practices: `docs/practices/`; both verbatim from keel-layout, to be followed, not rewritten
- Admin conventions: `docs/admin/` (permission specification, the admin's extra steps, frontend conventions); their
  source is keel-admin, and they are likewise distributed verbatim to derived repos
- Table changes: `docs/admin/domain-model.md`, "Schema: one directory per database"
