# A failed fetch affects only the block that uses that data

<!-- Shared file: carried verbatim; never localized outside its source repository.
     Checked by scripts/check-cross-repo-sync.sh; see docs/upgrading-protocol.md. -->

## Context

Fetching page data with `useSuspenseQuery` and prefetching it with `ensureQueryData` in the `loader` means nothing is
drawn until the data arrives, and a failed fetch throws the error to the router, whose `errorComponent` replaces the
whole content with an error panel. On a list page, when the new page fails after changing conditions or paging, the
filter bar, the table and the pagination all disappear together. A plain `useQuery` with `data?.items ?? []` draws a
blank on failure that looks like "nothing configured" or "no data", harder to notice than an error. An auth shell that
treats any failure to fetch the profile as not logged in sends a server error to the login page.

## Decision

1. **A failed request makes only the block using that data show "Failed to load" and a retry**; the page header, main
   actions and filter bar, which are frame and touch no data, show as usual whether requests succeed or fail. The rule
   is the same whether a page depends on one endpoint or several.
2. **Pages fetch data only with `useQuery`, and kernel components draw the failed state**: `DataRegion` (the data is
   handed over only inside its `children`), `query` on `DataTable`, `queries` on `FormDialog`, `load` on filter
   fields. The judgement is centralized in `web/src/lib/load-state.ts`: any failed query counts as failed; a background
   refetch failing while data is at hand still counts as ready; a disabled query does not count.
3. **What cannot be fetched is not shown as empty.** A failure shows as a failure, never covered with an empty array.
4. **Prefetching only fires, never waits, never throws** (`void prefetchQuery`): when the table or data region mounts
   it picks up the same request, and shows the failure if there is one.
5. **A lint rule forbids the patterns that throw to the router**: under `routes/`, the `useSuspenseQuery` family and
   `ensureQueryData` are errors in Biome, except in the auth shell.
6. **There are only two whole-page failures**: frontend code failing to render, and the auth shell failing to fetch the
   profile (other than an expired session). The former is a defect; with the latter, without a profile there is no menu
   to draw and no permission to check. Both have a retry that really refetches (`router.invalidate()`); an expired
   session still jumps to the login page.

## Rejected approaches

- **A whole-page error panel with a retry that refetches.** A small block failing would still wipe out the whole
  page, the filter bar with it, leaving the user unable even to change conditions.
- **Not drawing the page until data arrives (prefetch everything + throw to the router).** It guarantees nothing shows
  as empty, but the unit of failure is the whole page, exactly what this decision rules out.
- **Kernel data region components, but with the router-level fallback kept as insurance.** A page that forgets the
  data region falls back to whole-page failure; "no whole-page failures" should be guaranteed by how code is written,
  not by insurance. Hence rule 5: the patterns that throw to the router cannot be written at all.
- **Writing it only into the rules and having each page check `isError` itself.** It relies on discipline, which is
  exactly what produces blank pages.
- **Reverting to the last successful conditions when a list fails after changing conditions.** Data and conditions
  would always agree, but the conditions the user just entered would be silently withdrawn; instead the filter bar
  keeps the new conditions and only the table body shows the failure.

## Consequences

- Pages are written in two layers: the parts that read data go inside a data region, the frame outside. Buttons that
  depend on data can only go inside the data region or be greyed out until the data arrives.
- Entering a page does not wait for data: the frame appears first, and data regions show a skeleton first.
  Prefetching runs in parallel with loading the code chunks, so data from fast endpoints shows with no visible delay.
