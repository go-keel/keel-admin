import { keepPreviousData, useQuery } from '@tanstack/react-query';
import { useRouter } from '@tanstack/react-router';
import { useEffect, useMemo, useRef, useState } from 'react';
import type { FilterField } from '@/components/filter-bar';
import {
  type ListDef,
  type ListFields,
  type ListFilterValues,
  type ListSearch,
  resetUrlPatch,
  toFilterValues,
  toUrlPatch,
} from '@/lib/list';
import {
  initialListState,
  type ListEvent,
  nextListState,
  resultSetKey,
} from '@/lib/list-state';
import { toTotal } from '@/lib/table';

const EMPTY_ROWS: never[] = [];

/**
 * Type of a list page's filter field list: keys can only be keys of the list definition. Not needed when the field
 * list is an inline array (`FilterBar` infers it); when it is stored in a variable first (e.g. `useMemo`) and passed
 * later, annotate the variable with it, otherwise the keys widen to any string and do not match in `FilterBar`.
 */
export type FilterFieldsOf<L extends { fields: ListFields }> = FilterField<
  keyof L['fields'] & string
>[];

/**
 * The standard wiring of a list page: given a list definition (the output of `defineList` in `lib/list.ts`) and its
 * route, builds all the props `FilterBar` and `DataTable` need. What the definition does not cover (what the filter
 * controls look like, columns, row actions, dialogs) is still written by the page, see docs/admin/forms.md
 * "URLs and paging on list pages".
 *
 * **The query follows the page's own state; the URL is only kept in sync**: the applied filter conditions are local
 * state here, and the query, paging and result set derive from it. Only "Search" and "Reset" change it, and they write
 * it to the URL (adding a history entry). The URL decides the conditions in only two cases, both handled like a first
 * open (restore the conditions from the URL, go to page 1, fetch): the first open of the page, and a URL change not
 * made by a search (back/forward, clicking this page's link in the sidebar, editing the address bar). Going back
 * therefore returns to the previous search's conditions.
 *
 * `route` only needs the two methods `useSearch`/`useNavigate`: a loose structural constraint instead of importing the
 * concrete type generated for the route. When a call site passes the real `Route` (from TanStack file routing), its
 * own concrete type still applies; here it is used only through the shape it is known to have. search values are
 * strings or numbers depending on the field kind (see `ListSearch` in `lib/list.ts`), so the constraint is relaxed to
 * `string | number | undefined` rather than assuming all strings.
 */
export function useList<F extends ListFields, T>(
  list: ListDef<F, T>,
  route: {
    useSearch: () => ListSearch;
    useNavigate: () => (opts: {
      search: <S extends object>(prev: S) => S;
    }) => unknown;
  },
) {
  const urlSearch = route.useSearch();
  const navigate = route.useNavigate();
  const urlKey = JSON.stringify(urlSearch);

  // Conditions, page, page size and the reopen count are one state whose transitions all live in lib/list-state.ts;
  // this only hands it what happens on the route and the buttons.
  const [state, setState] = useState(() => initialListState(urlSearch));
  const dispatch = (event: ListEvent) =>
    setState((s) => nextListState(s, event));

  // A URL change goes to the list state: it ignores the URL this page just wrote back, otherwise it restores the
  // conditions as an external change. Uses React's documented "adjusting state during render" pattern, so it takes
  // effect in the same pass as the route's navigation.
  const [seenUrlKey, setSeenUrlKey] = useState(urlKey);
  if (urlKey !== seenUrlKey) {
    setSeenUrlKey(urlKey);
    dispatch({ type: 'urlChanged', search: urlSearch });
  }

  // An external navigation that leaves the URL unchanged (clicking the sidebar link to this page) is also handled as
  // a first open: the router writes no history for the same URL and the URL does not change, so the only signal is
  // hrefChanged being false in its public navigation event. Cost: `router.invalidate()` takes the same path, so
  // calling it also sends the list back to page 1.
  const router = useRouter();
  useEffect(
    () =>
      router.subscribe('onBeforeNavigate', (e) => {
        if (!e.hrefChanged)
          setState((s) => nextListState(s, { type: 'reopened' }));
      }),
    [router],
  );

  // While the applied conditions are unchanged, the list state returns the same applied object, so caching by
  // reference is enough: re-renders such as a refetch on window focus produce no new values, and together with
  // FilterBar's own value-based backfill the draft is not wiped.
  // Keys carry the list definition's keys: a wrong field key in the filter bar (not in the definition) does not
  // compile.
  const values = useMemo(
    () => toFilterValues(list.fields, state.applied),
    [list.fields, state.applied],
  );

  // Not useSuspenseQuery: on failure it throws to the route and the whole page becomes an error page. Here a failure
  // only makes the table region show "failed to load" (docs/admin/errors.md "A failed fetch affects only its
  // block"). When conditions or pages change, the old table stays until new data arrives (keepPreviousData) instead
  // of flashing back to the skeleton.
  const query = useQuery({
    ...list.queryOptions(state.applied, state.page, state.pageSize),
    placeholderData: keepPreviousData,
  });
  // The table wants rows: the same query with the data swapped for items. The rest of the query state is passed on
  // as is; the table decides loading, failed and ready by lib/load-state.ts.
  const rows = useMemo(
    () =>
      query.data === undefined ? undefined : (query.data.items ?? EMPTY_ROWS),
    [query.data],
  );
  const { refetch } = query;
  // When a fetch fails the table shows the failed state, and the pager still uses the last total so users can page
  // back.
  const lastTotal = useRef(0);
  if (query.data) lastTotal.current = toTotal(query.data.total);
  // On reopen the conditions and page may be unchanged, so the query key is too; refetch explicitly.
  useEffect(() => {
    if (state.revision > 0) void refetch({ cancelRefetch: false });
  }, [state.revision, refetch]);

  // "Search" / "Reset": if the conditions changed, update the list state and write it to the URL; if not, stay on the
  // current page and refetch. A click should always have an effect.
  const submit = (patch: Record<string, unknown>) => {
    const search = list.route.validateSearch.parse({
      ...state.applied,
      ...patch,
    }) as ListSearch;
    const next = nextListState(state, { type: 'submitted', search });
    if (next === state) {
      void refetch();
      return;
    }
    setState(next);
    void navigate({
      search: (prev) => ({ ...prev, ...resetUrlPatch(list.fields), ...search }),
    });
  };

  return {
    /**
     * The applied filter conditions (validated shape, same as the URL). For the rare page that sends a request outside
     * `useList`/`DataTable` with the current filters (such as exporting the whole result set), use this instead of
     * building them again. Most pages do not need it; `filterBar`/`table` cover everything the UI needs.
     */
    search: state.applied,
    /**
     * The rows of the current page (an empty array when not fetched), for pages that need them: subscribing to live
     * status of this page's rows, or bulk actions that read fields of checked rows. Not details: a drawer or form
     * that needs one record's full content fetches it fresh by ID.
     */
    rows: rows ?? EMPTY_ROWS,
    /**
     * Whether a fetch is in flight right now (including background refetches), for "refresh elsewhere when the list
     * refetches".
     */
    fetching: query.isFetching,
    filterBar: {
      values,
      revision: state.revision,
      // The parameter still accepts any key, so pages passing filterBar to their own function (typed with "any key")
      // still match. Keys are checked through values above.
      onSubmit: (v: ListFilterValues) => submit(toUrlPatch(list.fields, v)),
      onReset: () => submit(resetUrlPatch(list.fields)),
    },
    table: {
      query: {
        data: rows,
        isError: query.isError,
        isPending: query.isPending,
        fetchStatus: query.fetchStatus,
        refetch,
      },
      total: lastTotal.current,
      page: state.page,
      pageSize: state.pageSize,
      onPageChange: (page: number) => dispatch({ type: 'pageChanged', page }),
      onPageSizeChange: (pageSize: number) =>
        dispatch({ type: 'pageSizeChanged', pageSize }),
      resultSetKey: resultSetKey(list.fields, state),
      ...(list.getRowId ? { getRowId: list.getRowId } : {}),
    },
  };
}
