<!-- Shared file: carried verbatim; never localized outside its source repository.
     Checked by scripts/check-cross-repo-sync.sh; see docs/upgrading-protocol.md. -->

# Business tables and the data layer: what the admin adds

Read this before adding a business table, a repo or a schema change, or writing across the two databases, together
with `docs/guides/domain-model.md`.

The guide covers writing DOs, POs and repos, schema scripts, caches and locks, and `docs/practices/database.md`
covers table design; this page covers only what an admin built on keel-admin adds.

## Tables and databases

- **`sys_` is the kernel's reserved prefix; business tables never use it**, so a kernel upgrade can never collide
  with a business table. A business table's prefix follows the meaning of its business domain (`order_`, `pay_`…),
  and tables of one domain share a prefix; business models live in the business area, never under
  `internal/core/`.
- **Two databases**: the kernel's `sys_*` tables are in the admin database (`database.admin`), business tables in the
  business database (`database.default`), and both are required. A business repo holds the `Data` of the business
  area's own `internal/data` (built from the business database), never the kernel's: borrowing it silently breaks
  deploying the two databases apart.
- **There is no JOIN across the databases.** Data scope only adds a WHERE on the table's own ownership columns
  (`permdata.ApplyScope`), never a JOIN with kernel tables such as `sys_dept`: that is what lets the two databases
  be deployed apart. Ownership columns, `ApplyScope` and gating by explicit assignment are in
  `docs/admin/permission.md`.

## No transactions across databases

The two databases are two connections, so **an operation that writes both cannot rely on being atomic**: order it as
"do what may fail first, then clean up" and accept partial effects. The typical case is deleting an instance that
can be explicitly assigned (such as a category): first delete the instance in the business database ("no child rows
below it" is a condition of that delete statement, and both the delete and the child-row insert lock the parent row
first in their own transaction, see `docs/practices/transaction.md` "Parent-child consistency takes both sides"),
then clear each role's grant rows for it in the admin database through `perm.GrantRevoker`. If the cleanup fails,
the dangling grant rows left behind do not widen any permission, so this order is acceptable. The kernel's own
cross-database order works the same way (`internal/core/sys/data/scope.go`). How the two databases are split is in
`docs/adr/admin-0002-two-databases-and-default-ownership.md`.

## Schema: one directory per database

The scripts follow the guide's "Schema: hand-run .sql scripts only" (complete schema, baseline, incremental scripts),
one directory per database:

| Directory | Tables | Run against | Who owns it |
|---|---|---|---|
| `migrations/admin/` | kernel tables (`sys_*`) | admin database (`database.admin`) | the kernel: fixed name, overwritten whole when following upstream |
| `migrations/business/` | business tables | business database (`database.default`) | this repository: the default directory, renamed or joined by more as it likes |

- **`migrations/admin/` holds only `0_schema.sql`**, the upstream complete schema; this repository never edits it and
  never adds a baseline or incremental scripts there. The `ALTER` an existing admin database needs is in the matching
  `UPGRADING.md` entry.
- **The business directories are this repository's own**: upstream ships `migrations/business/0_schema.sql` with the
  example tables only, and this repository manages its business databases by the guide's rules from there on. One
  more business database means one more directory under `migrations/`.
- Only the MySQL dialect is shipped; another dialect needs the DDL ported.

## Table design choices each repository makes

For the points `docs/practices/database.md` leaves to each repository, the admin's business tables choose as
follows. Some kernel columns predate these choices; follow this list, not the kernel's column names.

- **Boolean columns use the `is_` prefix** (`is_pinned`, `is_public`): it works for any part of speech and avoids
  reserved words (`sensitive` is a MySQL reserved word). The kernel's existing columns mix `is_root`, `otp_bound`,
  `force_otp` and `sensitive`.
- **Two-state columns use `bool` in Go.** The kernel's existing two-state columns are `int8` in the PO, with column
  type `TINYINT(1)`.
- **An ID column meaning "none" uses `0`**, not NULL (`sys_dept.parent_id = 0` is a top-level department).

## Soft deletes and time

- **Kernel master data is never physically deleted**: `sys_user`, `sys_dept` and `sys_role` carry `deleted_at` and
  are hidden by GORM's default scope. The lifecycle of business-area data is the business's decision and this rule
  does not apply; if you choose soft deletes, how to build the column is in `docs/practices/database.md` "Soft
  delete".
- **Every instant exposed externally is serialized as UTC RFC3339 by `RFC3339` in `internal/core/timefmt`**; the
  backend never converts it to a display time zone ahead of time (the frontend header decides the display time
  zone). DOs hold `time.Time`, and conversion happens only at the service boundary. How to pass instants, business
  days and calendar dates is in `docs/admin/api-endpoint.md` "Passing time: meaning first".

## Live examples of where field constraints live

The guide's "Where field constraints live" puts domain invariants in biz, with one validation function shared by
every entry point. The kernel's live example is the avatar: `validateAvatar` in `internal/core/sys/biz/avatar.go`
is called by all three entry points (creating a user, editing a user, editing one's own profile); the proto side
only puts a coarse upper bound on the own-profile request.

## Anti-patterns

- ❌ Creating a business table under `internal/core/` or with the `sys_` prefix.
- ❌ A business repo taking the kernel's `Data` from `internal/core/data`.
- ❌ Writing a data-scope filter as a cross-database JOIN with `sys_dept`.
- ❌ Expecting one transaction to cover writes to both databases.
- ❌ Adding a baseline or an incremental script under `migrations/admin/`: take the upstream `0_schema.sql` whole and
  run the ALTER given in the matching `UPGRADING.md` entry against the existing admin database.
