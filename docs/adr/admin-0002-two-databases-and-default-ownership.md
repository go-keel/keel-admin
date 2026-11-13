# `default` belongs to the business database, `sys_*` tables use `database.admin`, and both are required

<!-- Shared file: carried verbatim; never localized outside its source repository.
     Checked by scripts/check-cross-repo-sync.sh; see docs/upgrading-protocol.md. -->

The admin's tables live in two databases: `database.default` is the business database (the business area's own
tables), and `database.admin` is the admin database (all `sys_*` tables). Both keys are required; missing either one
fails at startup. The assembly is in `cmd/<entry>/di.go`; the term is under "Two databases" in `GLOSSARY.md`.

Who owns `default` and requiring both are two decisions, with separate reasons.

## Why `default` belongs to the business

Not for semantic tidiness, but for consistency along the template chain. In keel-layout, the `default` that
`kdb.NewDB` takes is that repository's own business database; Keel's `kdb.NewDB` is only an alias of
`NewNamedDB(…, "default")` and takes no position on what `default` holds. If keel-admin gave `default` to the kernel,
then assembly code copied over from layout would get something with a different meaning from `NewDB`, and that is
written down nowhere.

Along the chain, `default` uniformly means "the database of this repository's own code", and the kernel, as a guest,
takes a named instance. When a derived repo copies assembly code from layout or admin, `kdb.NewDB` means the same
thing.

The key is named `admin`, not `sys`, at a clear cost: in yaml, `scripts/rename.sh` replaces only the entry directory
name (the segment under `cmd/` and `bin/`), so the bare `admin:` key escapes the rename, and in a derived repo with
another name it is just "that key in the config" (the same trade-off is decided the other way for
`web/src/routes/_app/sys/`; see "Code zones" in `GLOSSARY.md`). In return the key name is the term: docs can say
"admin database" without translating it into a config key.

## Why there is no fallback

A mirror fallback (`admin` falls back to `default` when not set) would give single-database deployments zero config,
but it makes **the zero-config default exactly the shape keel-admin does not recommend**: `sys_*` tables land in the
business database. The recommendation is written in docs and the default in `configs/config.yaml`; when the two
disagree, the default always wins.

The cost of requiring both is that a single-database deployment writes the same DSN twice. What it buys:
`cmd/<entry>/di.go` has no conditional branch, `internal/data/data.go` need not explain a fallback, and
`docs/admin/config.md` has one shape fewer to explain.
