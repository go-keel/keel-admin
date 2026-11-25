<!-- Shared file: carried verbatim; never localized outside its source repository.
     Checked by scripts/check-cross-repo-sync.sh; see docs/upgrading-protocol.md. -->

# Testing: permissions, data scope and the frontend

Read this when writing tests in an admin built on keel-admin, after `docs/guides/testing.md`: the guide covers fakes,
table-driven tests, service tests and data tests on real databases at the storage boundary; this page covers only what
the admin adds. `<app>` is the entry directory name.

## Backend seams the admin adds

| Under test | Seam | Dependencies |
|---|---|---|
| Data scope branches in biz | A fake `perm.ScopeResolver` injecting a fixed `ScopeSpec` | No infrastructure |
| Data scope filtering in data | The WHERE built by `permdata.ApplyScope`, run on every dialect kept (`make integration`) | MySQL, PostgreSQL |
| Permission registration | Kernel and business batches registered together, then `perm.Seal` on di.go's HTTP services (`cmd/<app>/registration_test.go`) | None |
| Middleware order (authentication before principal / authz / audit) | `cmd/<app>/chain_order_test.go`, already written upstream | None |

**Data scope is the branch most worth testing**: the same call must give different results under different specs.

- **biz**: a fake `ScopeResolver` returns a fixed spec and a fake repo records the spec it receives; assert that the
  UseCase passed the scope to the repo unchanged (not rewritten or swallowed on the way); that ownership on write comes
  from `actor`, not from the input; and that "id does not exist" and "out of scope" return the same domain error.

  ```go
  type fakeScopeResolver struct{ spec perm.ScopeSpec }

  var _ perm.ScopeResolver = (*fakeScopeResolver)(nil)

  func (f *fakeScopeResolver) Resolve(context.Context, *perm.Principal, perm.Resource) (perm.ScopeSpec, error) {
      return f.spec, nil
  }
  ```

- **data**: the WHERE built by `ApplyScope` can only be tested at this layer, which is why admin data tests are a must.
  Open the test database as in `docs/guides/testing.md` "data tests: the storage boundary", insert a few rows with
  different ownership, and assert the visible set and deny-all under each spec.
- **service**: permission checks belong to the authz middleware and the `perm` registry, not to service tests; all a
  service test can check is that the scope it received is passed down unchanged.

**`cmd/<app>/registration_test.go` seals di.go's own `registerPermissions` against di.go's own `httpServices`**; a new
business module passes `nil` there for each parameter it adds to either function (the `httpServices` call in
`servedOperations`, the `registerPermissions` call in `registerAll`) and changes nothing else: an unregistered code in
the binding table, an unowned endpoint or a cross-module dependency passes each unit test on its own and would fail only
when the binary starts; this test exists to close that gap. The completeness of the kernel entries for permission codes
and audit actions is tested here too.

## Real-database tests

Data tests are `*_integration_test.go` files with `//go:build integration`, run by `make integration` with
`TEST_MYSQL_DSN` and `TEST_POSTGRES_DSN`; a missing variable for a dialect the repository runs on fails the test. The
helper is the kernel's `internal/core/testdb` (`testdb.ForEachDB`): once per dialect the repository runs on (those every
`migrations/<database>/` has; `migrations/admin/` keeps both, so the business directories decide), it creates a fresh
database, applies the complete schemas of the databases it is given and drops it afterwards. Each package wraps it in
a `forEachDB` whose callback receives a `*gorm.DB`, the handle `Data` holds:

- the kernel's (`internal/core/sys/data/`) applies only `migrations/admin/<dialect>/0_schema.sql`, since kernel tests
  touch only `sys_*` tables, and deletes the root seed so each test inserts exactly the rows it asserts on;
- the business area's (`internal/data/testdb_integration_test.go`) applies every `migrations/*/<dialect>/0_schema.sql`;
  it is this repository's own, so a database that is not MySQL or PostgreSQL (Doris, say) stays out of `migrations/`
  or out of its list.

Concurrency tests (locks, unique keys) use the same helper and so run under both MySQL's repeatable read and
PostgreSQL's read committed; kernel examples: the concurrent role save and user edit tests in
`internal/core/sys/data/`. Seed rows give every `NOT NULL` column without a default, as the real tables require.

## Frontend tests

`web/` tests are vitest (`pnpm test`, jsdom environment, no watch). Validating a frontend change = `pnpm lint` (Biome +
`tsc --noEmit`, including `*.test.ts`) + `pnpm test`, both run in `web/`.

Coverage is deliberately narrow, with the same criterion as the backend: **"would this logic be silently wrong if it
broke"**. Logic that fails loudly (a page that does not render) is caught by running it; logic that fails silently is
not.

- **Must test**: logic in your own pages and shared business code that would be silently wrong if it broke: URL
  parameter encoding and decoding, `custom` filter `toParams`, pagination and default fallbacks, permission-dependent
  branching (the request still goes out and the page still renders, only the numbers are wrong). The kernel's own
  logic under `web/src/lib/` is tested upstream.
- **Do not test**: component rendering. The assertions would land on Base UI and TanStack, which is upstream behaviour;
  so **do not install `@testing-library/*`**, and verify DOM behaviour by running it in a browser.
- i18n keys need no tests: the `en-US` resources are the keys' type source, so a misspelled key fails in `tsc`; a missing
  translation fails only in a copy file that declares `Translations<typeof appEnUS>`.

To test adaptation, move it out of the JSX. Code specific to one page goes in the same directory with TanStack file
routing's `-` prefix (`routes/_app/orders/-logic.ts`, not treated as a route); code shared across business
pages goes in `routes/_app/-shared/` (`web/src/lib/` is kernel and is overwritten on upgrade). **Pagination parameters are not page-specific**: every list endpoint uses `offset/limit`, so the
conversion exists exactly once (`toOffsetParams` in the kernel's `web/src/lib/table.ts`, applied by `defineList`).
