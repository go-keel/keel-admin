# Lists shown in pages are paginated on the server, always in the offset/limit shape

<!-- Shared file: carried verbatim; never localized outside its source repository.
     Checked by scripts/check-cross-repo-sync.sh; see docs/upgrading-protocol.md. -->

## Context

If the template does not fix whether and how a list is paginated, derived repos each do what they happen to see in
the template: an endpoint reads the whole table and lets the frontend slice pages, or every endpoint invents its own
pagination parameters. A derived repo's default is whatever the template's examples and guides show:

- if the kernel's list example "sends everything at once and filters in the frontend", derived repos copy the full
  load, and the role list is the heaviest of all: every row carries perms plus the three boundary dimensions, plus a
  permission code lookup per row;
- if no guide has a list endpoint example, "how to paginate" has no source;
- if the template contains two pagination shapes at once (`offset/limit` and `page/page_size`), a derived repo reads
  "pagination parameters differ per endpoint; each page writes its own adapter".

So the template fixes the pagination shape, and the example, the guides and the shared frontend helper agree on it.

## Decision

**Every list that needs to be shown in pages is paginated on the server, always as `offset/limit` + `total`;
filtering moves to the server together with pagination.**

There are only two kinds of exemption:

1. **Tree / hierarchical data**: pagination would put children and their parent on different pages;
2. **Option sets returned whole for a selector**: a fixed enumeration, or business data known to be small (roles).

An exemption states which of the two kinds it is in the rpc comment (`docs/admin/api-endpoint.md`).

**Option sets that keep growing with the business (merchants, users) are not exempt; they are fetched in batches by
keyword**: the same `offset/limit` + `total`, plus a keyword; an empty keyword also returns the first batch. They also
accept `ids` and then return only those items, so the frontend can show the names of the selected items. The batch
limit is set per use case and is not tied to the table's page-size limit; the endpoint only has to declare a limit and
the frontend must not ask for more than it per batch.

## Why option sets are split by business case, not all returned whole or all batched

Returning everything at once: options that keep growing, like merchants, sooner or later can neither be transferred
nor let the user find the one to pick. Batching everything: a dropdown of a few statuses or roles would also need a
search box and batch loading, an extra layer of interaction for nothing. So it is split by business case: whether the
count is fixed or known to be small is judged by whoever writes the endpoint, who writes the reason into the
rpc comment. A reason that can be written down and reviewed fits the case better than a one-size-fits-all count
threshold.

Showing the names of selected items reuses the same endpoint (`ids`) instead of opening a separate "get by ID"
endpoint: permissions and data scope are checked in one place, so there is no hole where an item cannot be found by
search but can be fetched by ID.

Not judged by the widget ("the page renders it with a non-paginated widget, so it is exempt"): **that criterion is
circular**; any endpoint could exempt itself by saying "my page does not use the table component". Data that serves both
is the counterexample: the same rows are split by purpose into a selector's option set and a paginated
management list, regardless of which widget a page uses.

## Why offset/limit and not page/page_size

`page/page_size` is closer to the `page/pageSize` table components commonly use, so the frontend converts nothing. But
the conversion does not disappear; it moves to the service layer (`offset := (page-1)*pageSize`), and that wrapper
buys nothing. `offset/limit` maps directly onto the storage layer's query parameters, and the frontend keeps a single
conversion (the shared helper `toOffsetParams` and its unit test).

Cost: `offset` starts at 0 and page numbers start at 1; forget the minus one and you get "page two shows page one's
data". Requests still go out and the page still renders; only the numbers are wrong. That risk is covered by **having
only one conversion** (`toOffsetParams` + its unit test), not by avoiding the minus one. This is also one reason the
shape must be uniform: once the shape splits, the shared helper can no longer cover it.

## Consequences

- Every endpoint shown in pages has to implement `offset/limit` + `total` and server-side filtering; reading the whole
  table at once to save effort is not allowed.
- An exemption has to be stated explicitly in the rpc comment (a convention in `docs/admin/api-endpoint.md`), one more step, and that step exists precisely to make the author state which kind it is.
