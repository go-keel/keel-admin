# Languages and copy: English fallback, owners keep their copy complete, the kernel overwrites two files

<!-- Shared file: carried verbatim; never localized outside its source repository.
     Checked by scripts/check-cross-repo-sync.sh; see docs/upgrading-protocol.md. -->

## Context

A derived repo must be able to add a language without changing kernel code. If the language set is hard-coded (a
frontend language type of only `'en-US' | 'zh-CN'`, a backend that loads two fixed file names), a third language
means editing the kernel.

Missing messages also need one handling: when a language lacks a message, go-i18n falls back to the English source and
returns a `MessageNotFoundErr` along with it. If each caller handles this error its own way (a panic, an empty string
that shows permission names as code names, English hard-coded in Go and worded differently from the toml), every
language that is not fully translated surfaces these failures each in its own way.

## Decision

1. **English is the fallback language**: whenever any language lacks any message, it silently falls back to English;
   never a message ID, never a panic, never an empty string.
2. **Whoever owns the copy is responsible for its completeness; upstream guarantees only that the kernel copy agrees
   between Chinese and English.** Whether business copy (the frontend `app` namespace, backend `app.*.toml`) is fully
   translated and whether it is checked is up to the derived repo; for kernel copy (the frontend `sys` namespace,
   backend `sys.*.toml`) the only guarantee is that the key sets of `en-US`/`zh-CN` (frontend) and
   `sys.en.toml`/`sys.zh-CN.toml` (backend) agree; translations into other languages are not subject to this check.
3. **The kernel's overwrite scope for copy is two files, not the whole directory**: frontend
   `web/src/locales/sys/en-US.ts`, `zh-CN.ts`; backend `internal/i18n/sys.en.toml`, `sys.zh-CN.toml`. Other language
   files a derived repo adds in the same directories are neither overwritten nor checked by kernel upgrades.
4. **Backend copy is split like the frontend's into kernel files and business files**: `sys.*.toml` holds the messages
   referenced by `internal/core/`, `app.*.toml` those referenced by `internal/biz|service`, mirroring the frontend's two
   i18n namespaces `locales/sys/` and `locales/app/`.
5. **Changing language does not refetch**: fields in an already loaded list that the backend translated and sent with
   the data (such as the audit log's operation display names) are reinterpreted in the new language at the next real
   fetch (paging, refresh, changing filters). This is the same as rule 3 of
   `docs/adr/admin-0003-three-meanings-of-time-and-how-to-pass-them.md`, "changing the display time zone does not
   refetch": both postpone "reinterpreting already loaded data" to the next fetch; the argument
   is in that rule.

## Why English is the fallback

When a message is missing, go-i18n already falls through the language negotiation chain to the bundle's default
language (passed to `i18n.NewBundle` at creation): "fallback" is the library's own mechanism, not a newly invented
concept. With English as the default language, a user hitting a missing message sees a readable English sentence
rather than an internal message ID like `ErrDeptNotFound` (which would leak implementation details to end users).
English as the fallback also rests on a premise: the template ships Chinese and English, and English is the language
of the code, docs and error codes.

What needs solving is the judgement between "fall back to English" and "report an error": when go-i18n returns
`MessageNotFoundErr`, it is not a loading failure; the message really is missing, and the library has already fallen
back to English by design. `internal/i18n.MustLocalizeMessage` and the frontend's `fallbackLng: 'en-US'` judge this
signal "normal" and route it down one path, instead of scattered, inconsistent failures (panic / empty string /
hard-coded English).

## Why whoever owns the copy keeps it complete, and upstream guarantees only kernel Chinese and English

Business copy is the derived repo's own content, never touched by upstream upgrades ("Assembly seam" in
`GLOSSARY.md`); asking upstream to police "is it fully translated" makes no sense: upstream does not know which
languages a derived repo intends to support and should not decide that for it. A derived repo that wants to guarantee
nothing is missed adds the type constraint `Translations<typeof appEnUS>` to its own copy files
(`locales/app/zh-CN.ts` is written this way); one that does not want the check is unaffected, and the kernel never
refuses an upgrade or reports an error because business copy lacks keys.

The kernel guarantees only two files because of "read-only ≠ zero diff" ("Kernel" in `GLOSSARY.md`): read-only says
"do not change it"; zero diff says "replaced wholesale on sync, without conflicts". A kernel upgrade overwrites whole
files, and overwriting requires the overwritten content to be fixed verbatim: a file that may lack keys and that the
derived repo may fill in at any time cannot be overwritten as a whole; overwriting it deletes the keys the derived repo
added. The Chinese and English files evolve with upstream and are maintained verbatim, so they qualify for the kernel;
other language files are added and maintained by the derived repo and should never be overwritten by upstream.

## Why the kernel overwrites two files, not the whole directory

Overwriting the whole `locales/sys/` directory (frontend) or a single copy file (backend) assumes the directory or file
holds only upstream's own content. A derived repo, through the assembly seam (`web/src/languages.ts`), adds its own
language file such as `fr-FR.ts` to the same directory, and overwriting the whole directory deletes it or conflicts
with it.

The alternative is a separate `sys-extra/` (or a backend directory of the same name) for non-kernel language files,
with the kernel overwriting the "safe" location as a whole directory / whole file. It loses: derived repos would have
to learn one more rule, "this directory is safe, that one is not", and both kinds of file would still sit at the same
level (under `locales/`, under `internal/i18n/`), easy to misremember and misplace. File level needs no new concept:
the "read-only ≠ zero diff" boundary is drawn per file, and scripts that operate on whole directories, like
`make rename`, need no special case for it. The accepted cost is that the boundary names two specific files instead of
a single directory.

## Why the backend also splits sys/app

A single backend copy file would mix the business area's messages with middleware errors and system domain messages.
Business messages keep growing with the business (derived repos add their own
domains); without a split, the rule "the kernel guarantees only Chinese/English agreement" has no file boundary to
attach to: the check would either also require business messages to be fully translated, or fail to cover kernel
messages; both are wrong.

Split into `sys.*.toml` / `app.*.toml`, with the same names and structure as the frontend's `locales/sys/` /
`locales/app/`: both ends share one mental model, "sys is the kernel, app is the business", instead of two
partitioning rules to remember. Ownership is decided by who references the message (referenced by `internal/core/` →
sys; by `internal/biz|service` → app), not by "does it look like a system message": the former is a fact you can
`grep`, the latter an impression, easy to get wrong.

## Rejected approaches

- **Showing the message ID or a blank when a message is missing**: see "Why English is the fallback"; it leaks
  implementation details to users.
- **The kernel overwriting whole directories / whole files**: it would delete or conflict with language files a
  derived repo added; see "Why the kernel overwrites two files, not the whole directory".
- **A separate directory/file for non-kernel languages**: one more "where is safe" concept, with both kinds of file
  still at the same level; it costs more than it gains; see the same section.
- **The kernel forcing business copy to be fully translated too**: upstream does not know which languages a derived
  repo will support, so doing this makes no sense (decision 2).
