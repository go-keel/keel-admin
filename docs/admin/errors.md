<!-- Shared file: carried verbatim; never localized outside its source repository.
     Checked by scripts/check-cross-repo-sync.sh; see docs/upgrading-protocol.md. -->

# Errors: reason ranges, registration and the frontend

Read this before adding a domain error or a reason, handling an error in the frontend, or
changing code that fails with a `keel-admin/<id>` error, together with `docs/guides/errors.md`.

The guide covers the error propagation chain, how each layer wraps and logs, and sentinel mappings; this page covers
only what an admin built on keel-admin adds. `<pkg>` is the proto package directory under `api/`.

## Adding a domain error: four places

1. Add the reason in `api/<pkg>/v1/error_reason.proto`; **business reasons are numbered from 1000**: the kernel
   takes everything below 1000, and keeping the ranges apart lets both sides append without collisions.
   → `make proto`.
2. Add the sentinel in `internal/biz/errors.go`.
3. Add the mapping in `RegisterErrors` in `internal/service/errors.go`. The business area **appends its mappings to
   the kernel's sentinel group** instead of building its own: in di.go,
   `nsvc.RegisterErrors(service.NewErrorSentinel())`, kernel mappings underneath, business mappings on top.
4. Add the messages to every `internal/i18n/app.<lang>.toml` this repository ships. The kernel does not check that business
   entries are fully translated, and missing ones fall back to English; a derived repo that wants a guarantee adds
   its own check. Message IDs are passed in variables, so goi18n's extract cannot find them; write them by hand.

**The frontend needs no registration for a new reason**: the default toast is right (see "The frontend has two
rules" below).

```go
// internal/service/errors.go (a fictional ErrArticleNotFound)
func RegisterErrors(g *sentinel.Group) *sentinel.Group {
    g.OnLocaled(biz.ErrArticleNotFound, localed("ErrArticleNotFound", "Article not found.", v1.ErrorArticleNotFound))
    return g
}
```

`localed` is a **deliberate copy** of the helper of the same name in the kernel's `internal/core/sys/service`: the
business area as a whole does not import `core/sys`, its unexported helpers included.

## The reason is the contract, the HTTP status is derived

The response body is always Kratos's `{code, reason, message}`, with `message` already localized by
`Accept-Language` (`keel:docs/http.md#error-responses`). A 401 covers three situations with entirely different
meanings: the identity does not hold (go to the login page), the login flow is not finished (wrong captcha, OTP
needed: show it in place), and a step-up challenge (open a dialog). HTTP cannot express the difference, so
**clients dispatch on the reason, not the status**.

The status is still chosen by meaning: pick it per `docs/guides/errors.md` "HTTP status codes"; in this repository
401 is limited to identity not holding + login flow not finished + step-up. Changing a reason's status changes client
behaviour, so treat it as a breaking change.

## The frontend has two rules

In `web/src/lib/api.ts`: the three session-invalid reasons (`UNAUTHORIZED` / `UNAUTHENTICATED` /
`SESSION_EXPIRED`, all from the framework and the kernel middleware; a business reason never joins this list) clear
the login state and go to the login page; **every other non-2xx shows a toast**. There is deliberately no third
kind such as "left to the caller, no global message": once someone forgets to handle it, the error is swallowed
silently. Nothing dispatches on the status, and no reason gets a second copy of its message in the frontend: the
server's localized `message` is the one copy. A page's extra handling (opening the code dialog, clearing a one-time
credential, navigating) is **added on top of** the toast, never instead of it.

## A failed fetch affects only its block

The two rules above decide whether a failure is announced or redirects; this section decides what the page looks
like after a failure: **a failed request makes only the block that uses that data show "failed to load" with a
retry; the rest of the page keeps showing and keeps working.** The trade-offs are in
`docs/adr/admin-0009-fetch-failure-affects-only-its-block.md`.

- **Pages fetch data only with `useQuery`, and kernel parts draw the failure**: a display block uses `DataRegion`
  (`web/src/components/data-region.tsx`, which hands out the data only inside its `children`); a table gets the query
  result through `DataTable`'s `query` (list pages get this from `useList`); a form dialog passes the detail and
  option queries to `FormDialog`'s `queries` (a confirmation dialog likewise to `ActionDialog`'s `queries`); a filter
  dropdown's options are given as `load` on the field.
- **The page frame does not touch data**: the page header, primary actions and filter bar are drawn outside the data
  block and show whether requests succeed or not.
- **Data that could not be fetched is never shown as empty**: `data?.items ?? []` passed straight into a list, form
  or dropdown turns a failure into "no data", which is not allowed.
- **When data is already on screen and a background refetch fails, keep showing the old data**, with only the
  request layer's toast.
- **Prefetching fires and forgets, never waits or throws**: in a `loader`, write
  `void queryClient.prefetchQuery(...)`.
- **Patterns that throw errors to the route are banned**: no `useSuspenseQuery` family and no `ensureQueryData` under
  `routes/`; Biome reports them (overrides in `web/biome.json`). The authenticated shell `routes/_app.tsx` is the
  exception.
- **The kernel's suspending reads only read data the shell prefetched and never evicts (`gcTime` infinite)**
  (`useProfile`): once the cache is evicted, the next read suspends the whole page. Other values the shell brings in
  are written into a module and read synchronously (the business time zone through `setBusinessTimeZone` /
  `useBusinessTimeZone`).
- **Only two whole-page failures remain**: frontend code throwing while rendering, and the authenticated shell
  failing to fetch the profile (for reasons other than an invalid session), which shows a full-screen "something went
  wrong" with a retry, because without the profile there is no menu and no permission check. An invalid session still
  goes to the login page.

## Contract-layer validation messages

A request that fails the proto annotations returns reason `VALIDATOR` with one generic sentence in the request's
language, chosen by Keel (`keel:docs/middleware.md#validate`); the field-level detail goes to the log. Frontend forms
validate against the same constraints first, so normal use never reaches this point, and the frontend shows the
sentence like any other error.

## Kernel startup errors carry rule IDs

Errors the kernel returns at startup or registration (the permission registry, `sys.*` config, captcha wiring) are
built with `errs.Rules.New` / `errs.Rules.Wrap` from `internal/core/errs` and read `keel-admin/<id>: ...`. Each ID
has a section in `docs/admin/startup-errors.md`, which `internal/core/errs/errs_test.go` checks, so the explanation
and the fix are one lookup away. **Look the ID up there before changing code**: the section says which rule was broken
and how to fix it, and editing the kernel to silence the error breaks that rule for every derived repo. Errors reading
`keel/<id>` come from the framework and are documented in Keel.

## Anti-patterns

- ❌ Numbering a business reason below 1000.
- ❌ The business area importing the kernel's `core/sys/service` to reuse its translation helper.
- ❌ The frontend dispatching on the HTTP status, or keeping a second copy of a reason's message in the frontend.
- ❌ A page using `useSuspenseQuery`, or `ensureQueryData` in a `loader`: one failed request replaces the whole page
  with an error page.
- ❌ Drawing `useQuery(...).data?.items ?? []` without looking at the failure: data that could not be fetched shows
  as "no data".
- ❌ Changing kernel code to make a `keel-admin/<id>` startup error go away without reading its section first.
