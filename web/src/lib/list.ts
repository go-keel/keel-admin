import { type QueryClient, queryOptions } from '@tanstack/react-query';
import { z } from 'zod';
import { DEFAULT_PAGE_SIZE, toOffsetParams } from './table';
import {
  formatRange,
  parseRange,
  type RangeKind,
  toInstantRange,
} from './time-range';
import { getTimeZone } from './timezone';

/**
 * The **list definition** of a paged list (see GLOSSARY.md "Data conventions of a paged list"): everything a paged
 * list agrees on about its data: which filters exist, how each filter looks in the URL and in the request, and which
 * endpoint the data comes from. What the filter controls look like, the columns, row actions and dialogs are not here;
 * the page still writes them (`defineList` covers data only, see docs/admin/pages.md "Page layout").
 *
 * Imports no UI/React component, so vitest can run this pure-logic half on its own
 * (see docs/admin/testing.md "Frontend tests"). The React wiring is in `hooks/use-list.ts`.
 */

/**
 * Built-in conversions plus one escape hatch (how time values are passed: ADR admin-0003):
 * - `text`: the URL stores a string, the request sends it as is (an empty or absent value is not sent).
 * - `number`: the URL stores a number (unquoted), the request sends a number.
 * - `dateRange`: a date range, stored in the URL as `from~to`. The following kind is converted with the display time
 *   zone current at fetch time and sent as an instant range `<key>FromAt` / `<key>ToAt` (half-open);
 *   `zone: 'business'` is the fixed kind and sends business days `<key>FromDate` / `<key>ToDate` (both inclusive).
 * - `timeRange`: a time range to the second, following kind only, passed like a following date range.
 * - `custom`: the escape hatch: `toParams` computes the request parameters, the URL still stores it as text.
 *   `toParams` must be a pure function: the same value gives the same parameters, so the query key stays stable.
 *
 * Request parameter names of time fields are derived from the key (key `created` → `createdFromAt`); name the
 * endpoint fields by the `<field>_from_at` rule and they match, with no escape hatch needed for renaming.
 */
export type ListField =
  | { type: 'text' }
  | { type: 'number' }
  | { type: 'dateRange'; zone?: 'business' }
  | { type: 'timeRange' }
  | {
      type: 'custom';
      toParams: (value: string | undefined) => Record<string, unknown>;
    };

export type ListFields = Record<string, ListField>;

/**
 * Same shape as `FilterValues` in `components/filter-bar.tsx`. It does not import that component file: declaring it
 * separately keeps list.ts free of UI dependencies.
 */
export type ListFilterValues = Record<string, string>;

/**
 * The validated URL search: number fields are real numbers, the rest (text / time / custom) are strings. It uses a
 * loose union shape instead of exact per-field types; each function branches on `field.type` to convert (see
 * `buildSearchSchema`). `useList` relaxes its structural constraint on the route to this shape too.
 */
export type ListSearch = Record<string, string | number | undefined>;

/**
 * Raw URL value (the output of parseSearch, any JSON shape) → internal string.
 * null, arrays and objects all count as no value: a broken link such as `?keyword=null` must not turn the whole route
 * into an error surface; silently treating it as unset is the intended "unknown values are dropped" behaviour.
 * number / boolean are accepted as strings: hand-built links (`?categoryId=3`) and old links can arrive in those forms.
 */
const readText = (raw: unknown): string | undefined => {
  if (typeof raw === 'string') return raw;
  if (typeof raw === 'number' || typeof raw === 'boolean') return String(raw);
  return undefined;
};

/**
 * number fields: raw URL value → a real number, not a numeric string. Anything that does not convert to a finite
 * number counts as no value (bad parameters from old links, empty strings). **This must end as a number**: TanStack
 * serializes the URL from this schema's output; the string "1" is treated by the default stringifySearch as JSON text
 * and gets quotes added again (`status="1"`), only the number 1 is written as `status=1` (root cause and proof: ADR
 * admin-0005 and lib/list.test.ts). text fields (IDs and the like) stay strings; quotes in the URL are the cost of the
 * default rule, accepted on purpose and out of scope here.
 */
const readNumberValue = (raw: unknown): number | undefined => {
  if (typeof raw === 'number') return Number.isFinite(raw) ? raw : undefined;
  const text = readText(raw);
  if (text === undefined || text === '') return undefined;
  const n = Number(text);
  return Number.isFinite(n) ? n : undefined;
};

const rangeKind = (field: ListField): RangeKind | undefined =>
  field.type === 'dateRange'
    ? 'date'
    : field.type === 'timeRange'
      ? 'time'
      : undefined;

/** Raw URL value of a time field → normalized filter value; a malformed value counts as unset. */
const readRange = (kind: RangeKind) => (raw: unknown) => {
  const r = parseRange(readText(raw), kind);
  return r ? formatRange(r) : undefined;
};

/**
 * `validateSearch` of a list route: number fields go into `z.number().optional()` (real numbers, see
 * readNumberValue); time fields are normalized by `readRange`; text/custom are preprocessed as "raw URL value →
 * string or no value". All three end in `optional()`. Unknown keys are dropped by `z.object` by default (zod's strip
 * semantics), so parameters like `page`/`edit` in old bookmarks are still silently discarded.
 */
export function buildSearchSchema<F extends ListFields>(fields: F) {
  const shape = Object.fromEntries(
    Object.entries(fields).map(([key, field]) => {
      const kind = rangeKind(field);
      if (field.type === 'number') {
        return [key, z.preprocess(readNumberValue, z.number().optional())];
      }
      if (kind) {
        return [key, z.preprocess(readRange(kind), z.string().optional())];
      }
      return [key, z.preprocess(readText, z.string().optional())];
    }),
  ) as unknown as {
    [K in keyof F]: F[K] extends { type: 'number' }
      ? z.ZodOptional<z.ZodNumber>
      : z.ZodOptional<z.ZodString>;
  };
  return z.object(shape);
}

/**
 * search (the output of validateSearch) → the string map FilterBar uses: absent keys get '', real numbers of number
 * fields are turned back into strings. FilterBar values are always strings (they end up in the URL search, see
 * components/filter-bar.tsx).
 */
export function toFilterValues<F extends ListFields>(
  fields: F,
  search: ListSearch,
): Record<keyof F & string, string> {
  return Object.fromEntries(
    Object.keys(fields).map((k) => {
      const v = search[k];
      return [k, v === undefined ? '' : String(v)];
    }),
  ) as Record<keyof F & string, string>;
}

/** Raw URL object (unvalidated) → filter bar string map in one step, for tests and occasional manual conversion. */
export function urlToFilterValues(
  fields: ListFields,
  raw: Record<string, unknown>,
): ListFilterValues {
  return toFilterValues(fields, buildSearchSchema(fields).parse(raw));
}

/**
 * The draft FilterBar submits → a URL patch. An empty string means "no value" and removes the key from the URL
 * (the value is set to `undefined`, which the router's stringifySearch skips); number fields become real numbers,
 * so the URL reads `status=1` rather than the quoted `status="1"`.
 */
export function toUrlPatch(
  fields: ListFields,
  values: ListFilterValues,
): Record<string, unknown> {
  return Object.fromEntries(
    Object.keys(fields).map((k) => {
      const v = values[k] ?? '';
      if (v === '') return [k, undefined];
      return [k, fields[k]?.type === 'number' ? Number(v) : v];
    }),
  );
}

/** Reset: every key becomes "no value". Which keys a reset clears follows from the definition, not per-page lists. */
export function resetUrlPatch(fields: ListFields): Record<string, unknown> {
  return Object.fromEntries(Object.keys(fields).map((k) => [k, undefined]));
}

/**
 * search → the filter parameters to send (without paging). Following time filters are converted here (that is, at
 * fetch time) with the current display time zone: the query key depends only on the URL, so changing the display
 * time zone does not refetch; the next fetch (paging, refresh, refetch on focus) uses the new zone. A deliberate
 * simplification, see ADR admin-0003. Values of text / time / custom fields are always strings in search; this
 * function casts after branching on the field kind, so `ListSearch` needs no exact per-field types.
 */
export function toRequestParams(
  fields: ListFields,
  search: ListSearch,
): Record<string, unknown> {
  const out: Record<string, unknown> = {};
  for (const [key, field] of Object.entries(fields)) {
    const v = search[key];
    const kind = rangeKind(field);
    if (field.type === 'number') {
      if (v !== undefined) out[key] = typeof v === 'number' ? v : Number(v);
    } else if (kind) {
      const r = parseRange(v as string | undefined, kind);
      if (!r) continue;
      if (field.type === 'dateRange' && field.zone === 'business') {
        if (r.from) out[`${key}FromDate`] = r.from;
        if (r.to) out[`${key}ToDate`] = r.to;
      } else {
        const at = toInstantRange(r, kind, getTimeZone());
        if (at.from) out[`${key}FromAt`] = at.from;
        if (at.to) out[`${key}ToAt`] = at.to;
      }
    } else if (field.type === 'custom') {
      Object.assign(out, field.toParams(v as string | undefined));
    } else if (v) {
      out[key] = v;
    }
  }
  return out;
}

export type ListResponse<T> = {
  items?: T[] | null;
  total?: string | number | null;
};

export type ListDefConfig<F extends ListFields, T> = {
  /** Query key prefix, consistent with the existing invalidate calls (e.g. `['sys', 'users']`). */
  queryKey: readonly unknown[];
  fields: F;
  /** Sends the request with offset and filter parameters already built; returns the endpoint's list response as is. */
  fetch: (params: Record<string, unknown>) => Promise<ListResponse<T>>;
  /** The result set ID per row, which the page supplies (an optional DataTable input); a default so pages need not. */
  getRowId?: (row: T) => string;
  /**
   * Optional: for how many milliseconds the same data (same filters, page, page size) is reused from the cache
   * without a new request. Defaults to 0: every use shows the cached data first and refetches in the background (see
   * `lib/query.ts`). Use it only for lists whose data changes slowly and whose request is heavy; clicking "Search"
   * with unchanged conditions is not limited by it and still refetches.
   */
  staleTime?: number;
};

/**
 * List definition. In a route file, spreading `...xxxList.route` gives `validateSearch` + `loaderDeps` + `loader`
 * (prefetches page 1 with the same query key `useList` computes in the page).
 */
export function defineList<F extends ListFields, T>(
  config: ListDefConfig<F, T>,
) {
  const { fields, queryKey, fetch, getRowId, staleTime } = config;
  const validateSearch = buildSearchSchema(fields);

  const listQueryOptions = (
    search: ListSearch,
    page: number,
    pageSize: number,
  ) =>
    queryOptions({
      queryKey: [...queryKey, search, page, pageSize],
      queryFn: () =>
        fetch({
          ...toOffsetParams(page, pageSize),
          ...toRequestParams(fields, search),
        }),
      ...(staleTime === undefined ? {} : { staleTime }),
    });

  return {
    fields,
    getRowId,
    queryOptions: listQueryOptions,
    route: {
      validateSearch,
      loaderDeps: ({ search }: { search: ListSearch }) => search,
      // The page number is not in the URL, so a page always opens on page 1 and only that page is prefetched. Fire and
      // do not wait: the page frame renders at once and the table picks up the same request when it mounts; a failure
      // is not thrown to the route but shown by the table region (docs/admin/errors.md "A failed fetch affects only
      // its block").
      loader: ({
        context,
        deps,
      }: {
        context: { queryClient: QueryClient };
        deps: ListSearch;
      }) => {
        void context.queryClient.prefetchQuery(
          listQueryOptions(deps, 1, DEFAULT_PAGE_SIZE),
        );
      },
    },
  };
}

export type ListDef<F extends ListFields, T> = ReturnType<
  typeof defineList<F, T>
>;
