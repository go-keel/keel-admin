import { type ListFields, type ListSearch, toFilterValues } from './list';
import { DEFAULT_PAGE_SIZE } from './table';

/**
 * **List state** (see GLOSSARY.md "List state"): the filter conditions a list page currently applies, its page and
 * page size, and how many times the page was reopened. Request parameters, the page and the result set ID are all
 * computed from it alone, with no second copy.
 *
 * The rules are items 4 and 7 of docs/adr/admin-0005-list-definitions.md, and this is their only home:
 * - A new result set goes back to page 1; the page size is kept across result sets, and changing it also goes back
 *   to page 1.
 * - Only "Search" and "Reset" change the applied conditions; with unchanged conditions it stays on the current page
 *   (the caller refetches).
 * - A URL not written by this page (back/forward, editing the address bar), or a link to this page opening the same
 *   URL again, is handled like a first open: restore the conditions from the URL and go back to page 1.
 *
 * The page is reset to 1 when the conditions change, not masked as 1 when read: masking would bring A's page 3 back
 * in "A goes to page 3 → switch to B → switch back to A".
 *
 * Pure conversion without React, so vitest can test each rule (docs/admin/testing.md "Frontend tests"). The wiring
 * is in `hooks/use-list.ts`.
 */
export type ListState = {
  /**
   * The applied filter conditions, in the same shape as the URL (the output of the list definition's
   * `validateSearch`).
   */
  applied: ListSearch;
  /** How many times the page was reopened (navigation to the same URL). Part of the result set ID, not the request. */
  revision: number;
  page: number;
  pageSize: number;
};

export type ListEvent =
  /** The URL changed. If it equals the applied conditions, it is the one this page just wrote back; nothing changes. */
  | { type: 'urlChanged'; search: ListSearch }
  /** A navigation to this page that did not change the URL (clicking this page's link in the sidebar). */
  | { type: 'reopened' }
  /** "Search" / "Reset"; `search` is already validated by the list definition. */
  | { type: 'submitted'; search: ListSearch }
  | { type: 'pageChanged'; page: number }
  | { type: 'pageSizeChanged'; pageSize: number };

export const initialListState = (search: ListSearch): ListState => ({
  applied: search,
  revision: 0,
  page: 1,
  pageSize: DEFAULT_PAGE_SIZE,
});

const sameSearch = (a: ListSearch, b: ListSearch) =>
  JSON.stringify(a) === JSON.stringify(b);

/**
 * The list state after an event. When nothing changed, the same object is returned: callers use that to tell
 * "conditions unchanged" and can cache values derived from `applied` by reference.
 */
export function nextListState(state: ListState, event: ListEvent): ListState {
  switch (event.type) {
    case 'urlChanged':
    case 'submitted':
      if (sameSearch(event.search, state.applied)) return state;
      return { ...state, applied: event.search, page: 1 };
    case 'reopened':
      return { ...state, revision: state.revision + 1, page: 1 };
    case 'pageChanged':
      return event.page === state.page ? state : { ...state, page: event.page };
    case 'pageSizeChanged':
      return event.pageSize === state.pageSize
        ? state
        : { ...state, pageSize: event.pageSize, page: 1 };
  }
}

/**
 * Result set ID (see GLOSSARY.md "Result set"): the data the filter values select; a reopen also counts as a new set.
 * The page and page size are not in the key; the time zone of time filters is part of the value, so changing the
 * display time zone does not change it. The table uses it to decide when to clear the checked rows.
 */
export const resultSetKey = (fields: ListFields, state: ListState): string =>
  `${state.revision}:${JSON.stringify(toFilterValues(fields, state.applied))}`;
