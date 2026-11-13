import type { SupportedLocale } from './locale';

/**
 * Pure conversions for list pages. The UI half is in components/data-table.tsx; they are separate because frontend
 * unit tests run pure logic only, and mixing them would pull the whole UI dependency chain into the test process.
 */

/**
 * Page size options and default: a **kernel default**; changing it here changes every list page without its own.
 * For a single page that differs, pass `pageSizes` to DataTable instead of changing this.
 *
 * **The maximum pairs with the server's limit cap**: every list request in `api/keel_admin/v1/*.proto` declares
 * `limit` with `lte: 200`, and the two must be equal. A mismatch fails silently: the UI offers the option, the
 * request is rejected by protovalidate with 400, and the user sees only a toast and a pager that does not move.
 * A derived repo that passes a larger `pageSizes` to a page must change that endpoint's `lte` too.
 */
export const PAGE_SIZES = [20, 50, 100, 200] as const;
export const DEFAULT_PAGE_SIZE = 20;

/**
 * Columns without a size render at this width. The fallback is not a licence, see docs/admin/tables.md
 * "Pinned columns and column widths".
 */
export const FALLBACK_COLUMN_WIDTH = 160;

/**
 * A column width as "default + per-language values": language keys come from `SupportedLocale`, derived from the
 * registry, so a typo does not compile; a language without a value falls back to `default` (reasons:
 * docs/admin/tables.md, "Why these rules"). `locale` takes the runtime string (`i18next`'s `i18n.language`) instead of
 * a narrowed type; an unknown language falls back to default along the same path as a missing value, with no separate
 * check.
 */
export const widthFor = (
  widths: { default: number } & Partial<Record<SupportedLocale, number>>,
  locale: string,
): number => widths[locale as SupportedLocale] ?? widths.default;

/**
 * Dragged column widths: language → column id → pixels. **Separate per language**: a dragged width applies only to
 * the language it was dragged in; other languages keep the `widthFor` default (docs/admin/tables.md). Only columns
 * that were dragged and differ from the default are stored; undragged columns follow the default width the page
 * declares.
 */
export type ColumnWidthMemory = Record<string, Record<string, number>>;

/** Column information needed to compute widths: the default width (`size` for the current language) and the minimum. */
export type ColumnWidthDef = { id: string; size: number; minSize: number };

/** localStorage key of a table with remembered widths; the page provides `tableKey`, unique across the system. */
export const columnWidthStorageKey = (tableKey: string): string =>
  `keel_column_widths:${tableKey}`;

/**
 * Reads the raw stored text. Storage is the user's browser: the content may be written by an older version or edited
 * by hand. Any unrecognized part is dropped as if never stored, without throwing: forgetting a width only falls back
 * to the default and must not break table rendering.
 */
export const parseColumnWidths = (raw: string | null): ColumnWidthMemory => {
  if (!raw) return {};
  let data: unknown;
  try {
    data = JSON.parse(raw);
  } catch {
    return {};
  }
  if (!isRecord(data)) return {};
  const memory: ColumnWidthMemory = {};
  for (const [locale, widths] of Object.entries(data)) {
    if (!isRecord(widths)) continue;
    const valid = Object.entries(widths).filter(
      (e): e is [string, number] =>
        typeof e[1] === 'number' && Number.isFinite(e[1]) && e[1] > 0,
    );
    if (valid.length > 0) memory[locale] = Object.fromEntries(valid);
  }
  return memory;
};

const isRecord = (v: unknown): v is Record<string, unknown> =>
  typeof v === 'object' && v !== null && !Array.isArray(v);

/**
 * The dragged widths the table uses in the current language (the table library's columnSizing state). A stored column
 * may no longer exist (removed in a redesign, or an actions column missing under other permissions), or may be narrower
 * than a minimum the page raised later: the former is dropped, the latter clamped to the minimum.
 */
export const restoreColumnWidths = (
  memory: ColumnWidthMemory,
  locale: string,
  columns: readonly ColumnWidthDef[],
): Record<string, number> => {
  const saved = memory[locale] ?? {};
  return Object.fromEntries(
    columns.flatMap((c) => {
      const w = saved[c.id];
      return w === undefined ? [] : [[c.id, Math.max(c.minSize, w)]];
    }),
  );
};

/**
 * Writes the current language's dragged widths back to memory. Keeps only existing columns whose width differs from
 * the default: clicking a handle without moving it still commits "the current width", and without that filter the
 * column would be pinned to the default of that moment and miss later default changes by the page. Writing back also
 * clears columns that no longer exist.
 */
export const rememberColumnWidths = (
  memory: ColumnWidthMemory,
  locale: string,
  sizing: Record<string, number>,
  columns: readonly ColumnWidthDef[],
): ColumnWidthMemory => {
  const widths = Object.fromEntries(
    columns.flatMap((c) => {
      const w = sizing[c.id];
      return w === undefined || w === c.size ? [] : [[c.id, w]];
    }),
  );
  const { [locale]: _, ...rest } = memory;
  return Object.keys(widths).length > 0 ? { ...rest, [locale]: widths } : rest;
};

/**
 * The width each column actually renders at. When the widths add up to less than the container, the extra goes
 * **only to non-fixed columns, in proportion**; fixed columns (selection, identity and actions columns) always keep
 * their declared or dragged width, unaffected by the window or by other columns being dragged. Left to the browser,
 * `table-layout: fixed` would spread it over every column and the actions column would grow with the window. When
 * the widths are enough, they are returned as is and the container scrolls horizontally.
 */
export const spreadWidths = (
  columns: { id: string; size: number; pinned: boolean }[],
  viewport: number,
): Record<string, number> => {
  const total = columns.reduce((sum, c) => sum + c.size, 0);
  const flexible = columns.reduce(
    (sum, c) => (c.pinned ? sum : sum + c.size),
    0,
  );
  const extra = flexible > 0 ? Math.max(0, viewport - total) : 0;
  return Object.fromEntries(
    columns.map((c) => [
      c.id,
      c.pinned || extra === 0 ? c.size : c.size + (extra * c.size) / flexible,
    ]),
  );
};

/**
 * Page number → offset/limit. **The only conversion**: page numbers start at 1 and offsets at 0; an off-by-one on any
 * page shows up as "page two still shows page one's data", and separate copies are bound to get it wrong.
 */
export const toOffsetParams = (
  page: number,
  pageSize: number,
): { offset: number; limit: number } => ({
  offset: Math.max(0, (Math.max(1, page) - 1) * pageSize),
  limit: pageSize,
});

/**
 * total to number. **The only place**: it is int64 in proto and becomes a string over JSON; converting it with
 * Number() page by page, one missed place means "total is NaN, the pager is blank".
 */
/**
 * Where the next batch of an option set starts: the number of items fetched so far; there is no next batch once
 * `total` is reached or the previous batch was empty. Stopping on an empty batch prevents spinning forever when the
 * server's total does not match the actual rows.
 */
export const nextBatchOffset = (
  pages: readonly { items: readonly unknown[] }[],
  total: number,
): number | undefined => {
  const loaded = pages.reduce((n, p) => n + p.items.length, 0);
  const last = pages.at(-1);
  return last && last.items.length > 0 && loaded < total ? loaded : undefined;
};

export const toTotal = (total: string | number | undefined | null): number => {
  const n = typeof total === 'string' ? Number(total) : (total ?? 0);
  return Number.isFinite(n) && n > 0 ? n : 0;
};

/**
 * Intersects checked IDs with the current row IDs. After a successful bulk delete, IDs of deleted rows still in
 * rowSelection disappear from the count and from the bulk bar's parameters through this step, without waiting for the
 * result set ID to change: same page, same filters, just fewer rows. Skipping it breaks as "wrong count, and the next
 * click sends deleted IDs to an all-or-nothing endpoint".
 */
export const intersectSelection = (
  selection: Record<string, boolean>,
  rowIds: readonly string[],
): string[] => {
  const ids = new Set(rowIds);
  return Object.keys(selection).filter((id) => selection[id] && ids.has(id));
};

/** Total items + page size → page count. An empty list counts as 1 page, so the pager never shows "page 1 of 0". */
export const pageCount = (total: number, pageSize: number): number =>
  Math.max(1, Math.ceil(total / Math.max(1, pageSize)));

/**
 * Which page numbers the pager shows. All of them when there are fewer pages than the window; otherwise the current
 * page is centred as far as possible and clamped at both ends: an edge window like `[8,9,10]` is more intuitive than
 * "always centred but out of range".
 *
 * Ellipses are not decided here: they depend only on whether the window touches 1 and pages, which the component
 * sees at a glance; putting them in the return value would mean inventing a "page or ellipsis" union type.
 */
export const pageWindow = (page: number, pages: number, size = 5): number[] => {
  const n = Math.min(size, Math.max(1, pages));
  const start = Math.min(
    Math.max(1, page - Math.floor(n / 2)),
    Math.max(1, pages - n + 1),
  );
  return Array.from({ length: n }, (_, i) => start + i);
};

/**
 * How many cells the actions column shows inline. `maxInline` is the **maximum** number of cells in the column, and
 * the trailing "⋯" counts as one: if everything fits it is all inline, otherwise the last cell goes to "⋯". The column
 * width is therefore exactly that many cells; with "inline the first N, then add a "⋯" for the rest", the widest case
 * would be N cells plus a "⋯", and the column would waste that much width.
 */
export const inlineCount = (total: number, maxInline?: number): number =>
  maxInline === undefined || total <= maxInline ? total : maxInline - 1;
