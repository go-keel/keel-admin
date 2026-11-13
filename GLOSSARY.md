# keel-admin: the admin approach

keel-admin is an **approach**: a ready-to-use implementation for admin back-office scenarios that follows
keel-layout's conventions, and it is also an admin template that gets forked itself. Keel's GLOSSARY.md calls such an
implementation a specific approach: it has made trade-offs for one scenario, so it stays out of the framework. The
keel-layout segment below comes verbatim from upstream; the keel-admin segment has its source here.

<!-- shared segment begin keel-layout: carried verbatim; never localized outside its source repository. Checked by scripts/check-cross-repo-sync.sh; see docs/upgrading-protocol.md. -->

This file is the glossary. The paragraphs delimited by `shared segment begin` / `shared segment end` come verbatim
from upstream and are the terms shared along the template chain; everything outside them belongs to this repo.

## Roles

Use these words when writing docs and commits.

**User** (of the framework):
A project that depends directly on the Keel framework, this repository included.
_Avoid_: consumer, downstream project

**Derived repo**:
A repo forked from one on the template chain, renamed, and evolving on its own.
_Avoid_: fork project, business project, business team

**Upstream**:
The repo you forked from, counting only the direct level, never further up. keel-layout has no upstream: it is
where distribution starts. The Keel framework is referenced by version through go.mod and is nobody's upstream.
_Avoid_: calling the Keel framework "upstream"; skipping levels (such as a derived repo bypassing its direct
upstream to treat an earlier level as its upstream)

**Consumer**:
Defined in Keel's GLOSSARY.md (the consuming side of a message queue, nothing else; a JetStream consumer is written
**durable**). Here it is a handler registered in `internal/server/`.
_Avoid_: using "consumer" for a user or a derived repo

## Layering concepts

Three model shapes flow through three layers, and `service` converts at the boundary.

**DTO** (Data Transfer Object):
The request/response types generated from proto. Only the `service` layer touches them.
_Avoid_: VO, request body, pb type

**DO** (Domain Object):
The pure domain model owned by `biz`, with no proto and no storage tags. It is what crosses layers.
_Avoid_: Entity, domain entity, model

**PO** (Persistent Object):
The GORM model private to `data`. It never leaves the `data` package: the type is named after its table in
lowerCamelCase without a `Model` suffix, and the lowercase start lets the compiler enforce this.
_Avoid_: Model, DB entity, table struct

**repo interface**:
The dependency-inversion seam that `biz` declares and `data` implements. Constructors return the interface, not the
concrete type.
_Avoid_: DAO, repository implementation

**Port**:
An interface `biz` declares for an external system **whose contract someone else defines**: a downstream gRPC
service, a third-party HTTP API, things that evolve independently of this service. Like repo it is an inversion
seam; the difference is that repo faces this service's storage, Port faces someone else's system.

Mechanisms the framework provides (`knats`, `kasynq`, `kdb`, `kredis`) **do not go through a Port**; `biz` uses
their types directly: Keel defines the contract, it follows the version, breaking changes are covered by UPGRADING,
and another wrapper would only duplicate the abstraction.
_Avoid_: wrapping framework mechanisms in a Port; calling a repo a Port (repo faces this service's storage)

**Assembly seam**:
`cmd/<app>/di.go`. The hand-written construction order, with cleanups released LIFO. Derived repos append here
without changing the structure. There are at least three more of the same shape, where upstream provides the
baseline and the derived repo appends to the same file:
`api/<domain>/<version>/error_reason.proto`, `internal/biz/errors.go` and `internal/service/errors.go`, and the
message files under `internal/i18n/`; any others this repo has are listed outside the segment.
**Syncing an assembly seam always means a manual merge, by design**: it is neither overwritten wholesale like the
zero-diff area below, nor left untouched by upstream like a derived repo's own business code.
_Avoid_: dependency injection, DI container (this project has no DI tool); protecting the assembly seam like a
framework file, or giving it up like a business file

**Zero-diff area**:
The parts a derived repo replaces wholesale without conflicts: in every repo at least those registered in
`scripts/cross-repo-sync.list` (framework scripts and the shared files and shared segments described below); a
repo that has more lists them outside the segment. It is what lets a derived repo follow upstream for the long run:
syncing never stalls on accumulated conflicts. _Avoid_: confusing it with "read-only": read-only means "do not change",
zero-diff means "replacing it causes no conflict"

**Generated artifact**:
A file the generate commands can recompute from source files in the repo: at least `*.pb.go` and `openapi.yaml`;
any others this repo has are listed outside the segment. Committed, but written only by the generate commands.
Trade-offs: `docs/adr/layout-0003-generated-artifacts-committed.md`.
_Avoid_: merging a generated artifact by hand like an assembly seam (merge its source instead)

**Ledger**:
`UPGRADING.md`: one entry per change derived repositories must act on, plus this repository's "following upstream"
entries. The upstream anchor of the newest following entry is the **sync point**, where the next sync starts. Rules:
`docs/upgrading-protocol.md`.
_Avoid_: changelog (bug fixes and new capabilities stay out); recording the sync point anywhere else

**Sentinel error**:
A stdlib `errors.New` value in `internal/biz/errors.go`, mapped by `service` to a Kratos error and proto reason.
The `biz` layer does not know Kratos.
_Avoid_: business exception, error code

## Conventions and sharing

**Shared file**:
A file carried verbatim along the template chain and never rewritten outside its source repo: framework scripts,
shared ADRs, the upgrading protocol, and the two kinds of convention docs below; listed in
`scripts/cross-repo-sync.list` (`docs/upgrading-protocol.md`, "Shared files"). Trade-offs:
`docs/adr/layout-0002-verbatim-shared-docs-and-segmented-entry-files.md`.
_Avoid_: "a locally adapted copy per repo" (convention docs are never localized)

**Shared segment**:
A passage in an entry file (`AGENTS.md`, `GLOSSARY.md`) delimited by a pair of whole-line HTML comments and taken
verbatim from upstream; both markers name the source repo. Each additional upstream level along the template chain adds
one segment, in order; outside the segments the file belongs to this repo.
_Avoid_: copying upstream's whole entry file and rewriting it; writing this repo's rules inside a segment

**Guide**:
The conventions under `docs/guides/` loaded per task: steps and rules for one kind of work (adding an endpoint,
assembly, publishing events...). Shared files, independent of any business
(`docs/adr/layout-0002-verbatim-shared-docs-and-segmented-entry-files.md`). _Avoid_: writing this repo's business,
example code paths or concrete config values into a guide

**Practice**:
The task-independent coding conventions under `docs/practices/`, one topic per page (writing Go code, database
design, database transactions...). Also shared files; they differ from guides in when they load: a guide loads per
task, a practice before writing that kind of code.
_Avoid_: confusing them with guides

## Keel terms in this template

Authentication, contract endpoint, raw route, event contract, task contract and process facility are defined in
Keel's GLOSSARY.md. Only what this template adds is written here.

**Authentication**:
Keel's server-side check only; business checks such as a ban check are a separate step after it
(`docs/guides/auth.md`).

**Contract endpoint** / **raw route**:
The raw-http guide numbers their kinds 1a-1d and 2a-2e.
_Avoid_: type 1, type 2

**Pre-routing filter**:
A filter mounted with `sc.Filter` that runs on every request of a port before routing, outside the middleware chain
(kind 2e in the raw-http guide).
_Avoid_: wrapping the server's whole `Handler`

<!-- shared segment end keel-layout -->

<!-- shared segment begin keel-admin: carried verbatim; never localized outside its source repository. Checked by scripts/check-cross-repo-sync.sh; see docs/upgrading-protocol.md. -->

## Terms of the admin template

These terms are shared by every admin built on keel-admin, on the same footing as the segment above. The core asset
of an admin is the four-dimensional permission model; its full semantics are in `docs/admin/permission-model/`.
`<pkg>` is the proto package directory under `api/`.

## The two meanings of "business" (the easiest to confuse)

**Business area** / **business domain** / **business module**:
The management modules a derived repo develops **inside** the admin, living in `internal/biz|data|service/`, on the
frontend in `web/src/routes/_app/` (except `sys/`, which is kernel) and the own area of `web/src/locales/app/`; the
derived repo decides how to organize directories inside. It may use kernel facilities.
_Avoid_: calling it a "business system"

**Business system**:
A service **outside** the admin that the admin manages. The source of truth is there; the admin is only one of its
clients. The dependency goes one way: admin → business system; a business system must not depend on the admin's
tables, endpoints or config.
_Avoid_: calling it a "business domain" or "business module"

## The two databases

The admin's tables live in two databases, each matching a named config instance; both are required.

**Admin database** (`database.admin`):
All `sys_*` tables, the kernel's system-domain data: accounts, roles, departments, audit, one-time codes.
_Avoid_: framework database ("framework" means Keel only), kernel database, sys database

**Business database** (`database.default`):
The business area's own tables. `default` belongs to the derived repo's own code, as in keel-layout.
_Avoid_: the business system's database (a database outside the admin that the admin never touches)

## Code zones

**Kernel**:
The four-dimensional permission engine and the system domain (accounts, roles, departments, audit, one-time codes):
`internal/core/` and `internal/server/`, the admin database's schema `migrations/admin/`, the frontend kernel
(`docs/admin/shell.md`, "Frontend zones") and the English and Chinese kernel copy (`web/src/locales/sys/`,
`internal/i18n/sys.{en,zh-CN}.toml`: kernel, although the keel-layout segment counts message files under
`internal/i18n/` as assembly seams). Read-only for derived repos and a **zero-diff area**: replaced as a whole on
upgrade. Assembly seams, by contrast, are read-only in shape but merged by
hand. The frontend directory is named `sys`, not after the repository, because `make rename` does not rename
directories under `web/`.
_Avoid_: core layer, framework layer ("framework" means Keel only)

**Generated artifacts** (the admin's additional ones):
`web/src/generated/` and `web/src/routeTree.gen.ts`, on the same footing as `*.pb.go` and `openapi.yaml`.

**Third-party components**:
`web/src/components/ui/`, component source the shadcn CLI writes into this repo. Not kernel: on upgrade upstream's
files are overwritten one by one and the derived repo's additions are kept. Not a generated artifact: its source is
not in this repo, so it cannot be regenerated.
_Avoid_: calling it "kernel" or "generated artifact"

**Assembly seams** (the admin's additional ones):
`cmd/<app>/di.go`, `internal/conf/app.proto`, the business protos under `api/<pkg>/v1`; on the frontend
`web/src/main.tsx`, `web/src/shell.ts`, `web/src/nav.ts`, `web/src/languages.ts` (language registry),
`web/src/timezones.ts` (time zone registry), `web/src/header.tsx`, `web/src/login-layout.tsx` and `web/src/styles/`.
Files appended to as well: `api/<pkg>/v1/error_reason.proto` (kernel and business reasons share one enum),
`api/<pkg>/v1/types.proto` (messages shared by kernel and business), `internal/biz|service/errors.go`,
`internal/i18n/app.*.toml`.

**Admin conventions**:
The convention docs under `docs/admin/`: the permission specification, the admin's extra steps (a page named like a
guide supplements that guide) and the frontend conventions. Shared files like the guides and practices, with
keel-admin as their source.
_Avoid_: writing a particular derived repo's business or example code paths into them

## After authentication

**Permission check** (authz):
The kernel middleware step after principal resolution that decides whether the principal may call the operation;
audit follows it. In the admin, "authorization" means this, never authentication. Raw routes skip principal
resolution, the permission check and audit.
_Avoid_: using "authorization" for authentication

## Language

**Fallback language**:
The language shown when an entry is missing in the current language, always English. Trade-off:
`docs/adr/admin-0010-languages-and-copy-ownership.md`.
_Avoid_: default language (that is which language the UI shows when nobody has chosen one, a different question from
"which language shows when an entry is missing")

## The three shapes of a list

"List" is used for three different things whose pagination conventions are opposite. The criterion is **the shape
and use of the data**, not the widget a page uses: judging by widget is circular, and anyone could exempt themselves
by switching widgets. Trade-off: `docs/adr/admin-0004-list-pagination-shape.md`.

**Paged list**:
A management list for people to browse. Paged on the server, filtered on the server too.
_Avoid_: using "list" loosely for the two below

**Option set**:
A slim list serving a selector or filter, from an endpoint whose name ends in `Options`; whole or in keyword batches
depending on how the data grows (`docs/admin/api-endpoint.md`).
_Avoid_: dropdown data, dictionary, slim list

**Tree**:
Hierarchical parent-child data (departments, the permission tree). Not paged: paging would split children from their
parents.
_Avoid_: tree list (it is not a shape of paged list but another kind of thing)

## Data conventions of a paged list

**List definition**:
Everything a paged list agrees on about its data: which filters it has, what each filter looks like in the URL and
in the request, and which endpoint the data comes from. What filter controls, columns and row actions look like is
not part of it; trees and option sets have no list definition.
_Avoid_: query definition (clashes with the "Query" button of the filter bar), list config

**Result set**:
The batch of data the current filters of a paged list select. Changing page, page size or display time zone does not
change the result set; when the page is treated as opened anew (back/forward, clicking the page's own sidebar link,
editing the address bar), it counts as a new batch, even with the same filters.
_Avoid_: this batch of data, result identifier

**List state**:
The filters, page number and page size in effect on a list page right now, plus how many times the page was opened
anew. Request parameters, the page number and the result set are all derived from it.
_Avoid_: pagination state (it governs more than the page number)

**Display time zone**:
The time zone in which times are displayed, chosen in the header (defaults: `docs/admin/shell.md`). It changes only
the reading, never the data.
_Avoid_: selected time zone, current time zone

**Filter time zone**:
The time zone in which a time filter converts the chosen date or instant to an absolute instant, labelled on the
filter. Fixed filters use the business time zone; following filters use the display time zone at the moment the data
is fetched.
_Avoid_: calling it the "display time zone"

**Business time zone**:
The time zone in which a business "day" is counted; one value for the whole system, configured on the server and sent
after login, independent of who is logged in. It also serves as the default display time zone when the browser's
location is not recognized.
_Avoid_: system time zone, server time zone (that is the time zone of a machine or database connection, not a
business rule)

**Instant**:
A point on the timeline, such as a creation time or login time. Independent of time zones; another time zone only
changes the reading.
_Avoid_: using "time" for instants, business days and calendar dates alike

**Business day**:
A day as understood in the business time zone. Only the date travels between frontend and backend, and the server
converts it to instants in the business time zone; fixed filters pick business days.
_Avoid_: natural day (it does not say whose time zone)

**Calendar date**:
A date that is business data itself, such as a birthday or a contract's effective date; it belongs to no time zone
and is passed and stored as is, never converted to instants. Not the same as a date picked in a time filter, which
stands for a range of instants and is converted in the filter time zone.
_Avoid_: using "date" for both

## Appearance

**Appearance preset** (theme preset):
A named bundle of CSS variables: two color sets (light and dark), `--radius`, `--spacing`, shadows, fonts
(`docs/admin/shell.md`, "Appearance presets").
_Avoid_: theme, skin, style

**Color mode** (mode):
light / dark / system, an axis orthogonal to presets.
_Avoid_: theme

**Style**:
shadcn's component skeleton (Nova / Vega and so on), a build-time constant that reinstalls all component source when
changed. **The word has only this meaning.**
_Avoid_: using "style" for an appearance preset or color mode

**Tri-state**:
The `auto | hidden | fixed` of shell controls: rendered and switchable / not rendered but the value mechanism works
as usual / locked and not rendered. Color mode, preset, time zone and language share this one scheme.
_Avoid_: switch (a tri-state is not a boolean)

## The two kinds of choices a person makes

The same person makes two kinds of choices, stored in different places with different meanings; mixing them up
gives the wrong answer to "is it still there on another computer". What belongs in and why writes are per item:
`docs/admin/auth.md`, "User settings".

**Preference**:
A choice that follows the device, kept in the browser: color mode, appearance preset, time zone, language, sidebar
collapse, table column widths. The login page needs them before anyone is identified.
_Avoid_: calling it a "user setting"

**User setting**:
A choice that follows the account, stored on the server, such as favorites.
_Avoid_: calling it a "preference"

**Favorite**:
A page the person pins to the sidebar's "Favorites" group, identified by the page's URL, in favorite order. Sub-views
are not favorited separately; the page they belong to is.
_Avoid_: bookmark, pin, shortcut

## The edge columns of a table

A management list has a column with a fixed role at each side, with the data columns in between. Rules and
reasons: `docs/admin/tables.md`.

**Identity column**:
The column of a row that answers "who is this", leftmost. Usually the ID, but not necessarily.
_Avoid_: ID column (suggests it must be the ID)

**Actions column**:
The column holding row-level actions, rightmost. It may be absent entirely because of permissions.
_Avoid_: operation column, button column

**Selection column**:
The column holding row checkboxes, before the identity column; present only when the table has bulk actions.
_Avoid_: tick column, multi-select column, checkbox column

## Pages and sub-views

**Page**:
A whole screen with its own header title, opened from the sidebar menu or a URL. A page with navigable sub-views has
one URL per sub-view.
_Avoid_: view

**Sub-view**:
A content area inside a page, shown with the page rather than floating above it (what floats above is one of the
containers below).
_Avoid_: block, panel

**Tab**:
A kind of sub-view: switched by a row of tabs, one shown at a time. Whether it is directly navigable depends on the
need.
_Avoid_: page tab; using "tab" alone for a browser tab

**Browser tab**:
A tab at the top of the browser window. Write it in full, as in "open in a new browser tab".
_Avoid_: writing just "tab" or "new tab" for it

## Containers

The three containers below hold forms, confirmations and details; all are inside a page and have no URL of their
own. What goes into which: `docs/admin/forms.md`, "Container roles".

**Dialog**:
A window popping up in the middle of the screen, covering the whole page.
_Avoid_: popup, modal box, modal

**Popover**:
A small box popping up next to the button that triggered it, without covering the page.
_Avoid_: confirmation bubble, popconfirm

**Drawer** (Sheet):
A full-height panel sliding in from the right of the screen.
_Avoid_: shadcn's Drawer (another component, with a drag-to-close gesture; a drawer here uses Sheet), sidebar
(which means the left navigation only)

## Permission model

**Principal**:
The operator identity resolved per request: user, department, roles, permission codes, root flag. Permissions are
computed per request, never carried in the token.
_Avoid_: current user, user context, claims

**Permission code**:
A feature permission identifier shaped like `sys:account:view`, declared and registered in code, a compile-time
constant.
_Avoid_: permission point, permission string

**Permission dependency**:
Granting a permission code implicitly grants all its prerequisite codes too; prerequisites may cross business
domains and feature modules and must not form cycles.
_Avoid_: using it for Go package dependencies; treating business domain or module groups as dependency boundaries

**Data scope**:
A role × resource type row-level visibility range (self / own department / subtree / all / explicit assignment).
_Avoid_: data permission, row-level permission

**Explicit assignment**:
A kind of data scope: a role is granted a set of specific instances of a resource (such as grants per category). The
enum value in code is `RESOURCE_GRANT`; prose says "explicit assignment".
_Avoid_: resource grant

**Management boundary** (ARBAC):
Limits which roles a role may assign to others and which departments it may manage. Orthogonal to feature
permissions and data scope.
_Avoid_: tiered administrator permission

**Step-up**:
A sensitive operation asks for a one-time code again on the spot, regardless of the session.
_Avoid_: second verification, double confirmation (the latter means a UI confirmation box)

**One-time code** (OTP):
The numeric code an authenticator app generates on a rolling time basis, entered at login and at step-up.
_Avoid_: GA, Google code (Google Authenticator is only one of the usable apps)

**Reduced-permission preview**:
root browses the system as a chosen set of roles. It only removes permissions, never adds them.
_Avoid_: switch identity, impersonation (it does not change the user, only removes roles)

<!-- shared segment end keel-admin -->

## This repository: keel-admin

- The example of the **business area** is the notice module (notices and categories; categories are an
  explicit-assignment resource); the **business database** currently holds only its two example tables. After forking
  it is usually deleted entirely, see `AGENTS.md`, "Example code".
- **Consumer**: this repository uses no message queue, so the word does not appear here.
- **Zero-diff area**: besides those listed in the shared segments, the whole kernel (the two backend trees and the
  frontend kernel), registered in the last block of `scripts/cross-repo-sync.list` for derived repos to compare with
  `norm`.
