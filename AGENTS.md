# Agent guide

<!-- shared segment begin keel-layout: carried verbatim; never localized outside its source repository. Checked by scripts/check-cross-repo-sync.sh; see docs/upgrading-protocol.md. -->

# Repository guide

This repository is a service on the Keel framework (Kratos v3, standard `slog`, hand-written assembly), derived from
the keel-layout template. Keel's own docs are linked as `keel:docs/<page>.md`: the files of that name in the Keel
module your `go.mod` resolves to (`go list -m -f '{{.Dir}}' github.com/go-keel/keel`).

**This file has segments.** Text between `shared segment begin` / `shared segment end` comes verbatim from upstream:
keel-layout's segment first, then one per further template on the chain. Follow it; change it in its source repository,
not here. Text outside the segments is this repository's own. `docs/guides/` (playbooks per task) and `docs/practices/`
(rules independent of the task) are shared files too, checked by `make check-sync`; this repository's own conventions go
in new files routed from outside the segments.

## Done

A change is done when `make check` passes; run it before every commit. After changing data-layer code or a migration,
also run `make integration`. Generated files (`*.pb.go`, `*_grpc.pb.go`, `*_http.pb.go`,
`*_errors.pb.go`, `openapi.yaml`, `third_party/`) come only from `make all`; never edit them, never `sed` a `.pb.go`.

## Read before you act

Read the page before doing the task. A routing table outside the segments adds this repository's own pages.

| Before you | Read |
|---|---|
| write any Go code, or doubt a pattern | `docs/practices/go.md` |
| add a package, a resource or a dependency between layers; choose a name | `docs/practices/structure.md` |
| add or change an HTTP / gRPC endpoint, list endpoints included | `docs/guides/api-endpoint.md` |
| add a webhook, SSE, file download, log stream or pre-routing filter | `docs/guides/raw-http.md` |
| publish or consume events | `docs/guides/mq.md` |
| add an async task or a cron job | `docs/guides/task.md` |
| call another service (gRPC / HTTP) | `docs/guides/grpc-client.md` |
| change `di.go`, add a `cmd/`, add infrastructure | `docs/guides/assembly.md` |
| add a domain error, error code or sentinel mapping | `docs/guides/errors.md` |
| add a domain model, GORM model or repo | `docs/guides/domain-model.md` |
| create or change a table, write a migration | `docs/practices/database.md` |
| open a transaction, lock, conditional update, concurrent writes to one row | `docs/practices/transaction.md` |
| configure authentication, anonymous endpoints, claims | `docs/guides/auth.md` |
| add a config field | `docs/guides/config.md` |
| log, trace, add metrics | `docs/guides/observability.md` |
| deploy behind nginx; anything about the client IP | `docs/guides/deploy-nginx.md` |
| write tests | `docs/guides/testing.md` |
| write an error message, an i18n entry or any text end users read | `docs/practices/user-text.md` |
| follow upstream (a new template or Keel version); change something derived repositories must act on; have a commit stopped for want of an `UPGRADING.md` entry | `docs/upgrading-protocol.md` |
| write an ADR | `docs/adr/layout-0004-adr-naming-and-carry-protocol.md` |
| look up what Keel provides | `keel:docs/README.md` |
| use a term | `GLOSSARY.md` |

## Rules for every change

- **Use Keel first.** HTTP/gRPC servers, event consumers, tasks, cron, long-running workers, distributed locks, rate
  limits, snowflake IDs, JWT, password hashing, decimal conversion, redaction and hot-reloaded config come from Keel;
  do not rewrite them or add a library for them.
- **One concept, one home.** Each business rule, query criterion or state transition is defined in one function or repo
  method. Before coding, list the new concepts and their homes; when logic appears a second time, extract it first.
- **Layers point one way**: service → biz → data, wired only in `di.go` (`docs/practices/structure.md`).
- **Keel and upstream template code are read-only** (module cache, `go.work` targets, sibling checkouts). When one of
  them gets in the way (a bug, a missing mechanism, a rule that does not fit), open an issue in that repository: Keel's
  for the framework, your direct upstream's for the template. Give the reproduction, the suspected cause and what you
  need; upstream weighs it against its own scope, and a fix arrives when this repository follows upstream. Until then,
  work around it in this repository's own code.
- **Before changing a mechanism, a public API or a config key**, `grep -rn "<keyword>" --include="*.md" .` and update
  every doc it hits (shared ones in their source repository).

## Ask before acting

Report the plan and its impact, then wait for confirmation, when the work involves (additions outside the segments
apply too):

1. a fix that needs a change to the external API or observable behaviour;
2. a conflict with an existing decision (`AGENTS.md`, `GLOSSARY.md`, `docs/adr/`, specs listed outside the segments,
   commit history);
3. a high-risk or destructive problem (data loss, a security hole);
4. a new third-party dependency;
5. **a new file outside the business area.** New files in `internal/biz/`, `internal/data/`, `internal/service/`,
   `api/`, `migrations/` (and business areas listed outside the segments) need no question; everywhere else (new
   directories, a new `cmd/`, the repository root, `docs/`, `scripts/`, `internal/conf/`, `internal/server/`) ask.

## Git

Conventional Commits; a regenerated file goes in the same commit as its source. `configs/` holds only local development
placeholders, never real credentials. Push or tag only when asked; the permission covers that one push.

## Decisions

`GLOSSARY.md` decides wording; `docs/adr/` holds decisions. State a contradiction with them explicitly instead of
overriding it silently.

<!-- shared segment end keel-layout -->

<!-- shared segment begin keel-admin: carried verbatim; never localized outside its source repository. Checked by scripts/check-cross-repo-sync.sh; see docs/upgrading-protocol.md. -->

## Admin template: keel-admin

This repository is an admin back office built on the keel-admin template: on top of the segment above it has a
read-only kernel, a permission model and a React SPA. This segment holds for every admin built on keel-admin and is
followed, not rewritten.

**The admin's convention docs are in `docs/admin/`**, shared like `docs/guides/` and `docs/practices/`. A page named
like a guide is that guide's admin supplement: read both. `<app>` is the entry directory under `cmd/`, `<pkg>` the
proto package directory under `api/`.

### Shape

- **One binary**: `cmd/<app>` serves the proto HTTP endpoints and the embedded React SPA (`web/`) on one HTTP server;
  no gRPC, event consumer or task entry until one is added (`docs/admin/assembly.md`).
- **The kernel is read-only for derived repositories**: backend `internal/core/` and `internal/server/`, the admin
  database's complete schema `migrations/admin/`, and the frontend kernel (`docs/admin/shell.md`, "Frontend zones").
- **Business modules live in the business area** (`internal/biz|data|service/`, `web/src/routes/_app/` outside `sys/`,
  `web/src/locales/app/`), never in the kernel; their registration is appended in `internal/service/http.go`.

### Read before you act (admin)

| Before you | Read |
|---|---|
| add a business module: permission codes, data scope, registration, options endpoints, frontend gating | `docs/admin/permission.md` |
| need the full semantics of permissions, data scope, management boundaries, step-up, audit, root | `docs/admin/permission-model/README.md` |
| add or change an endpoint (permission binding, registration, lists, passing time) | `docs/guides/api-endpoint.md` + `docs/admin/api-endpoint.md` |
| change `cmd/<app>/di.go`, add a cmd, touch what the kernel owns | `docs/guides/assembly.md` + `docs/admin/assembly.md` |
| add a domain error or reason range; handle errors in the frontend | `docs/guides/errors.md` + `docs/admin/errors.md` |
| add a business table, repo or table script; write across the two databases | `docs/guides/domain-model.md` + `docs/admin/domain-model.md` |
| touch login, the session cookie, anonymous endpoints, captcha, user settings, the client IP | `docs/guides/auth.md` + `docs/admin/auth.md` |
| add a config field, a `sys.*` key, database config | `docs/guides/config.md` + `docs/admin/config.md` |
| add a webhook, SSE, download or other raw route | `docs/guides/raw-http.md` + `docs/admin/raw-http.md` |
| write tests (data scope, registration, frontend) | `docs/guides/testing.md` + `docs/admin/testing.md` |
| write a form, confirmation, dialog or drawer, a success message, a list page's URL state | `docs/admin/forms.md` |
| write a list: columns and widths, actions column, bulk actions, filter bar | `docs/admin/tables.md` |
| create a page: title, layout, sub-views, sidebar menu, controls, displaying time, user-facing text | `docs/admin/pages.md` |
| change the brand, header, time zones, UI languages, appearance; place frontend code | `docs/admin/shell.md` |
| meet a `keel-admin/<id>` startup error | `docs/admin/startup-errors.md` |

### Admin rules for every change

- **Permission bindings are written in code** (registered with `internal/core/perm`, never in the database); an endpoint
  with neither a binding nor an anonymous annotation fails startup (`perm.operation-unowned`).
  Raw routes skip the permission middleware.

### Ask before acting (admin additions)

Point 5 of the segment above covers the business areas listed in "Shape". Two more:

6. **Changing the kernel** (`internal/core/`, `internal/server/`, `migrations/admin/`, the frontend kernel, including new
   kernel files): upgrading overwrites the kernel whole, so a local change is lost on the next upgrade; report the
   need upstream instead. Adding components to `web/src/components/ui/` with the shadcn CLI
   needs no question (a new third-party dependency still does).
7. **A custom UI implementation** where shadcn/ui components and their combinations fall short: explain why, and wait
   for approval (`docs/admin/pages.md`, "Controls and sizes").

`docs/admin/permission-model/` is the full permission specification and counts as an existing decision.

<!-- shared segment end keel-admin -->

## This repository: keel-admin

**Maintenance.** This template is maintained only by its maintainer and their agents, inside the maintainer's
workspace with this repository checked out in the workspace's directory layout; the rules for changing the template
itself live in the workspace's `AGENTS.md`. A derived repository rewrites this section as its own.

Module: `github.com/go-keel/keel-admin`

### Example code

Two examples show the rules in real code; convention docs never cite them, and a derived repository usually deletes
them (`README.md`, "Delete the examples"; files are marked `Example file: delete the whole file after forking` or lines
`Example: delete after forking`).

- **Notices (notices and categories)**: a complete business module through endpoints, permission codes, data scope
  (categories are an explicitly assigned resource; notices are gated by `category_id`), audit and frontend pages.
  Backend `api/keel_admin/v1/notice.proto` → `internal/service/notice.go`, `internal/service/notice_perm.go` →
  `internal/biz/notice.go` → `internal/data/notice.go`, `internal/data/model.go`; tests `internal/biz/notice_test.go`,
  `internal/data/notice_integration_test.go` (MySQL, under `make integration`); table script
  `migrations/business/0_schema.sql`; frontend `web/src/routes/_app/notices/` and the two notice
  shortcuts on the workbench (`web/src/routes/_app/index.tsx`).
- **Japanese (`ja-JP`)**: the five steps of adding a UI language (`docs/admin/shell.md`, "Adding a UI language"):
  `web/src/languages.ts`, `web/src/locales/app/ja-JP.ts`, `web/src/locales/sys/ja-JP.ts`, `internal/i18n/app.ja.toml`
  and `sys.ja.toml`, and the Japanese widths in `web/src/styles/index.css`. The `Asia/Shanghai` entry in
  `web/src/timezones.ts` is an example too.
