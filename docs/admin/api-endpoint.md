<!-- Shared file: carried verbatim; never localized outside its source repository.
     Checked by scripts/check-cross-repo-sync.sh; see docs/upgrading-protocol.md. -->

# API endpoints: what an admin built on keel-admin adds

Read this when adding or changing an endpoint in the admin, together with `docs/guides/api-endpoint.md`.

The guide covers the whole chain proto → service → biz → data and the shape of list endpoints; this page covers only
the rules the admin adds. `<pkg>` is the proto package directory under `api/`, `<app>` is the entry directory name.

## HTTP only

The admin exposes HTTP as its only transport. A `service` in proto is still a gRPC definition, but it is registered
only through `*_http.pb.go`; when another service needs gRPC, add a separate entry per `docs/admin/assembly.md`
"Adding a cmd".

## The extra steps

On top of the guide's steps:

1. **`make proto` also generates the frontend's `web/src/generated/`** (OpenAPI types and the reason union type);
   these generated artifacts are never edited by hand. `make proto` fails when the frontend dependencies are not
   installed, because it uses the frontend's openapi-typescript.
2. **HTTP registration goes in the business area's `internal/service/http.go`** and is appended by
   `cmd/<app>/di.go` after the kernel's list; `internal/server/` stays untouched (see `docs/admin/assembly.md`).
3. **The permission binding is a required step**: every endpoint has one line in `perm.Bindings` in
   `internal/service/<domain>_perm.go`, its permission code is declared in `internal/biz/<domain>.go`, and both are
   registered through `perm.MustRegister`. **An endpoint with neither a binding nor `allow_anonymous` fails
   startup** (`perm.operation-unowned`). Full steps in `docs/admin/permission.md`.
4. **Business error reasons are numbered from 1000**; see `docs/admin/errors.md`.

## List endpoints: the admin additions

The general list rules (`offset/limit` + `total`, filter fields, tri-state, sort enums, list row types, the upper
bound only in the proto `lte`, the default count in biz, options and management lists kept apart) follow the guide.
The admin adds these:

- **For a paged list behind a table, the `lte` on `limit` must equal the largest page size the frontend offers**
  (the kernel's `PAGE_SIZES`, `web/src/lib/table.ts`): otherwise a size selectable in the UI is rejected as 400 by
  validation. An options endpoint fetched in batches (name ending in `Options`) does not feed a table; its per-batch
  bound is set per case and only has to be written as `lte`, and the frontend never asks for more per batch.
- **Only two kinds of list are exempt from paging**, judged by the shape and use of the data, not by the widget:
  trees (paging would split children from their parents) and option sets for pickers (an option you cannot page to
  cannot be picked). Name them so (`List<Resource>Options` for option sets) and say which kind in the rpc comment. "I
  render it with a widget that does not page" is no reason: any endpoint could exempt itself. The kernel's exempt
  lists: `ListDept` and `ListPermissions` (trees); `ListDeptOptions`, `ListRoleOptions`,
  `ListAssignableRoleOptions`, `ListAuditActions` and `ListResources` (option sets).
- **When a list shows names of kernel entities (owning department, creator), the backend fills them into the list
  row.** Having the frontend call the department or user endpoints to look them up puts a higher gate on the page
  than its own (`docs/adr/admin-0008-option-endpoints-and-page-gates.md`). A business-area service takes a
  `NameResolver` (`internal/service/names.go`), and di.go injects the kernel's `refRepo`; one page costs one query
  for departments and one for users, a name not found (deleted) stays empty, and the frontend falls back to `#id`.
- **Option sets for selectors and filters come from each domain's own `*Options` endpoint, never from its management
  endpoints; the permission binding follows the use, not the owning module.** The management list carries the
  management gate, so a page that only picks from it would demand rights it does not need.
- **A request sent on page load must not have a higher gate than the page's own entry gate**: a page the user can
  enter that then hits 403 while loading is the most damaging kind of mismatch. The reasoning is in
  `docs/adr/admin-0008-option-endpoints-and-page-gates.md`; the steps for adding an options endpoint and auditing
  page gates are in `docs/admin/permission.md` "Options endpoints (`*Options`): steps to add one" and "Page gate
  audit".
- **Wiring a list in the frontend**: the list definition uses `defineList`; URL, filters and paging conversion are
  in `docs/admin/forms.md` "List page URL and pagination", the table in `docs/admin/tables.md`.

## Detail endpoints for forms

**A form's initial values always come from a fresh call to the detail endpoint (`Get<Resource>`), never from the
list row or a frontend cache**; every editable resource must have a detail endpoint, and without one there is no
edit entry. List rows therefore carry only the fields the list shows: an extra field gets used by the next person
to prefill a form. The reasoning is in `docs/adr/admin-0006-form-initial-values-fetch-detail.md`.

## Passing time: meaning first

Decide what a time value means before deciding how to pass it (terms in `GLOSSARY.md`):

- **An instant** is UTC RFC3339 both in and out. A range is half-open, with parameters named `<field>_from_at` /
  `<field>_to_at`. The UI shows both ends as inclusive; the conversion is done once in `web/src/lib/time-range.ts`,
  never in a page.
- **A business day** and **a calendar date** are passed only as `YYYY-MM-DD`. A range includes both ends, with
  parameters named `<field>_from_date` / `<field>_to_date`. A business day is converted by the server using
  `sys.business_time_zone`.
- Parse with `ParseInstantRange` / `ParseDateRange` from `internal/core/timefmt`. The backend never converts an
  instant to a display time zone ahead of time: the display time zone is the viewer's choice in the header.
- The process, the database connection and the session time zone are always UTC.
- Switching the display time zone does not refetch data.

The trade-offs are in `docs/adr/admin-0003-three-meanings-of-time-and-how-to-pass-them.md`.

## Anti-patterns

- ❌ A new endpoint without a permission binding: startup fails (`perm.operation-unowned`); or worse, it is bound to
  the wrong code and open to people who should not see it.
- ❌ Adding a business endpoint under `internal/core/`: the kernel is read-only for derived repos; business proto and
  biz/data/service live in the business area.
- ❌ Sending extra fields in list rows "just in case", or prefilling an edit form from a list row.
- ❌ The frontend calling the kernel's department or user endpoints on a list page to show department or user names.
- ❌ A selector loading its options from another domain's management list.
- ❌ Sending a local wall-clock time without a zone, or a business-day range as instants.
