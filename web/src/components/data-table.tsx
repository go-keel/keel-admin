import {
  type ColumnDef,
  type ColumnPinningState,
  type ColumnSizingState,
  columnPinningFeature,
  columnResizingFeature,
  columnSizingFeature,
  createExpandedRowModel,
  type ExpandedState,
  type Row,
  type RowData,
  type RowSelectionState,
  rowExpandingFeature,
  rowSelectionFeature,
  type TableOptions,
  tableFeatures,
  type Updater,
  useTable,
} from '@tanstack/react-table';
import type { TFunction } from 'i18next';
import { ChevronsLeftIcon, ChevronsRightIcon } from 'lucide-react';
import {
  type ReactNode,
  useEffect,
  useLayoutEffect,
  useMemo,
  useRef,
  useState,
} from 'react';
import { useTranslation } from 'react-i18next';
import { LoadFailed } from '@/components/data-region';
import { Card } from '@/components/ui/card';
import { Checkbox } from '@/components/ui/checkbox';
import {
  Pagination,
  PaginationContent,
  PaginationEllipsis,
  PaginationItem,
  PaginationLink,
  PaginationNext,
  PaginationPrevious,
} from '@/components/ui/pagination';
import {
  Select,
  SelectContent,
  SelectGroup,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from '@/components/ui/select';
import { Skeleton } from '@/components/ui/skeleton';
import {
  Table,
  TableBody,
  TableCell,
  TableHead,
  TableHeader,
  TableRow,
} from '@/components/ui/table';
import { loadState, type QueryLike, retryFailed } from '@/lib/load-state';
import { readPreference, writePreference } from '@/lib/preference';
import {
  type ColumnWidthDef,
  type ColumnWidthMemory,
  columnWidthStorageKey,
  FALLBACK_COLUMN_WIDTH,
  intersectSelection,
  PAGE_SIZES,
  pageCount,
  pageWindow,
  parseColumnWidths,
  rememberColumnWidths,
  restoreColumnWidths,
  spreadWidths,
} from '@/lib/table';
import { cn } from '@/lib/utils';

/**
 * The features the table enables are exactly this list: v9 features must be registered explicitly, and an unregistered
 * feature has neither state nor methods. It is also the statement of what the kernel table does and does not do: no
 * sorting, no column visibility, no pagination feature here (pagination is server-side, no client-side slicing).
 * A derived repo that needs more passes options through or builds its own features, without changing this file.
 *
 * Row selection is registered but **off by default**: without `bulkActions` there is no selection column and no
 * selection state, exactly as before. It is in the kernel because the selected count, clearing the selection and
 * clearing it on page change all touch the same rowSelection state; spread over every derived repo, they would end up
 * with three different behaviours.
 */
export const dataTableFeatures = tableFeatures({
  columnPinningFeature,
  columnSizingFeature,
  columnResizingFeature,
  rowExpandingFeature,
  rowSelectionFeature,
  expandedRowModel: createExpandedRowModel(),
});

export type DataTableFeatures = typeof dataTableFeatures;
export type DataTableColumn<T extends RowData> = ColumnDef<
  DataTableFeatures,
  T,
  unknown
>;

/** A stable reference for an empty array: a new one on every render would invalidate every data-dependent model. */
const EMPTY: never[] = [];

/**
 * A pinned column's cell. While stuck, the other columns scroll behind it, so its background must be opaque (the card
 * background); but on row hover and while a row's menu is open, TableRow overlays the whole row with a translucent
 * `bg-muted/50`, which the opaque pinned columns cover, so the two end cells do not change color. This gives the
 * opaque version of the same color: a translucent color over the card background is the two mixed proportionally in
 * sRGB. The 50% matches the ratio of TableRow in ui/table.tsx; if it changes after the CLI regenerates it, change it
 * here too.
 */
const PINNED_CELL =
  'sticky z-10 bg-card in-[tr:hover]:bg-[color-mix(in_srgb,var(--muted)_50%,var(--card))] in-[tr:has([aria-expanded=true])]:bg-[color-mix(in_srgb,var(--muted)_50%,var(--card))]';

/**
 * The kernel table of admin lists, and also the **standard shell** of a list page: inside one border, from top to
 * bottom, the filter header, the bulk action bar, the table body and the footer bar (selected count + pagination),
 * separated by dividers.
 *
 * **No refresh button**: react-query's refetch on window focus plus invalidation after create, update and delete
 * already cover the ways data goes stale; a button taking space on every page buys nothing.
 *
 * The shell belongs to the shared component rather than being assembled by each page because the sections are really
 * coupled: the selected count in the footer, disabling the buttons on the bulk bar and clearing the selection on page
 * change all touch the same rowSelection state. Having pages assemble it would expose the table instance upward,
 * far more roundabout than two more slots. The filter **logic** stays with the page: `toolbar` is only a slot, and
 * the shared component does not know what filtering is.
 *
 * **Deliberately not done**: column visibility, density, fullscreen, column reordering by drag, inline editing.
 * Density is decided globally by the appearance preset's `--spacing`, with no second entry point; for the rest see
 * docs/admin/tables.md, "What the shared table parts do and do not do". A derived repo that needs sorting or
 * expanding passes `options` through to useReactTable rather than forking the component.
 */
export type DataTableProps<T extends RowData> = {
  columns: DataTableColumn<T>[];
  /**
   * The table's data: one query's result whose data is the row array (if the endpoint returns `{ items }` or something
   * to turn into a tree, convert it with `select` in `useQuery`). Skeleton, "Failed to load" or data is decided by
   * `lib/load-state.ts`, the same as for data regions and form dialogs: a skeleton when nothing has arrived the first
   * time; data whenever data is at hand (an empty list counts), with no flash back to the skeleton on background
   * refetch; "Failed to load" with a retry when it failed, while the card header (filter bar), bulk bar and footer bar
   * stay (docs/admin/errors.md, "A failed fetch affects only its block"). List pages get it wired by `useList`.
   */
  query: QueryLike & { data?: T[] };
  /**
   * Other data this block waits for: drawn only when all of it has arrived, failed if any part failed (as in
   * `DataRegion`).
   */
  requires?: readonly QueryLike[];
  /**
   * Which columns are pinned is **declared explicitly by the page**; the shared component does not infer it (see the
   * table rules). The selection column is the only exception: with `bulkActions` it pins itself leftmost, no need to
   * list it here.
   */
  pinning: ColumnPinningState;
  /**
   * The card header section. List pages put a `<FilterBar>` here; without it the section is not rendered, so pages
   * without a filter bar (tree tables, simple admin lists) start directly with the table, leaving no blank strip.
   */
  toolbar?: ReactNode;
  /**
   * The multi-select bulk action bar. Only with it is there a selection column; it is **always shown**, and its
   * buttons disable themselves when nothing is selected: inserting and removing the bar by selection would make the
   * table jump up and down, and checking rows is a continuous action.
   *
   * The arguments are the currently selected row ids (produced by `getRowId`) and a callback that clears the
   * selection.
   */
  bulkActions?: (ids: string[], clearSelection: () => void) => ReactNode;
  /**
   * The result set key (see GLOSSARY.md, "result set"). With it, the selection is cleared whenever the result set
   * changes; without it, only on page change and page size change, as before (tree tables and the like are
   * unaffected). Selected ids are also always intersected with the row ids in the current `data`, so deleted ids left
   * over after a bulk delete disappear from the count and the bulk bar's arguments without waiting for the result set
   * key to change.
   */
  resultSetKey?: string;
  /** Server-side pagination: without total no paginator is rendered (tree tables are not paged). */
  total?: number;
  page?: number;
  pageSize?: number;
  onPageChange?: (page: number) => void;
  onPageSizeChange?: (pageSize: number) => void;
  /**
   * Page size options. Defaults to `PAGE_SIZES` (20/50/100/200).
   *
   * **When passing a larger value, change the `lte` of limit in that endpoint's proto too**: the server's maximum must
   * equal the maximum here, or the UI offers a size the request is rejected for with 400 (see lib/table.ts).
   */
  pageSizes?: readonly number[];
  /** Tree rows: with it, rows expand into sub-rows, all expanded by default. */
  getSubRows?: (row: T) => T[] | undefined;
  getRowId?: (row: T) => string;
  /**
   * With it, dragged column widths are stored in the browser and still there next time (a preference, see GLOSSARY.md,
   * "The two kinds of choices a person makes"); without it they last for the session and reset on refresh. **Unique across the
   * whole system**, written "domain.page", such as `sys.users`: two tables with the same value share one set of widths.
   */
  tableKey?: string;
  /** Other options passed through to useReactTable (row selection, sorting, expanding...). */
  options?: Partial<TableOptions<DataTableFeatures, T>>;
  empty?: ReactNode;
};

export function DataTable<T extends RowData>({
  columns,
  query,
  requires = [],
  pinning,
  toolbar,
  bulkActions,
  resultSetKey,
  total,
  page = 1,
  pageSize = 20,
  onPageChange,
  onPageSizeChange,
  pageSizes = PAGE_SIZES,
  getSubRows,
  getRowId,
  tableKey,
  options,
  empty,
}: DataTableProps<T>) {
  const { t, i18n } = useTranslation();
  const loads = [query, ...requires];
  const state = loadState(loads);
  const data = state === 'ready' ? (query.data ?? EMPTY) : EMPTY;
  // Tree tables expand fully by default: the hierarchy should be seen at a glance; a table opened level by level hides
  // the structure.
  const [expanded, setExpanded] = useState<ExpandedState>(true);
  const [rowSelection, setRowSelection] = useState<RowSelectionState>({});

  warnMissingSizes(columns);

  const selectable = bulkActions !== undefined;
  const allColumns = useMemo(
    () => (selectable ? [selectColumn<T>(t), ...columns] : columns),
    [selectable, columns, t],
  );
  // The selection column pins itself leftmost, so the page does not write it into pinning: its position is not the
  // page's choice but the fixed shape "the checkbox is always at the start of the row".

  // Dragged column widths are stored per language: a drag applies only to the language in use, and after switching
  // language the widths dragged in that language, or the defaults, apply
  // (docs/admin/tables.md, "Pinned columns and column widths").
  // The session and the browser store hold the same widths, so behaviour is the same before and after a refresh;
  // without tableKey there is simply no storage read or write.
  const locale = i18n.language;
  const storageKey =
    tableKey === undefined ? undefined : columnWidthStorageKey(tableKey);
  const [widthMemory, setWidthMemory] = useState<ColumnWidthMemory>(() =>
    storageKey === undefined
      ? {}
      : parseColumnWidths(readPreference(storageKey)),
  );
  // Write only once the user has changed something: a table only looked at leaves no empty entry in the browser.
  const widthTouched = useRef(false);
  const widthDefs = useMemo(() => columnWidthDefs(allColumns), [allColumns]);
  const sizing = useMemo(
    () => restoreColumnWidths(widthMemory, locale, widthDefs),
    [widthMemory, locale, widthDefs],
  );
  const setSizing = (updater: Updater<ColumnSizingState>) => {
    widthTouched.current = true;
    setWidthMemory((prev) => {
      const current = restoreColumnWidths(prev, locale, widthDefs);
      const next = typeof updater === 'function' ? updater(current) : updater;
      return rememberColumnWidths(prev, locale, next, widthDefs);
    });
  };

  const allPinning = useMemo(
    () =>
      selectable
        ? { ...pinning, start: [SELECT_COLUMN_ID, ...(pinning.start ?? [])] }
        : pinning,
    [selectable, pinning],
  );

  const table = useTable<DataTableFeatures, T>({
    features: dataTableFeatures,
    data,
    columns: allColumns,
    getSubRows,
    getRowId,
    state: {
      columnPinning: allPinning,
      columnSizing: sizing,
      expanded,
      rowSelection,
    },
    enableRowSelection: selectable,
    onRowSelectionChange: setRowSelection,
    onColumnSizingChange: setSizing,
    onExpandedChange: setExpanded,
    columnResizeMode: 'onChange',
    enableColumnResizing: true,
    // Resetting the expanded state to "all collapsed" whenever data changes is the default behaviour, which would make
    // "all expanded by default" hold only on first load: one create, update, delete or refresh and the whole tree
    // collapses. Turning it off keeps the initial "all expanded", and rows the user collapsed by hand do not spring
    // back on a refresh.
    autoResetExpanded: false,
    defaultColumn: { size: FALLBACK_COLUMN_WIDTH, minSize: 60 },
    ...options,
  });

  // Canvas width = the sum of column widths. Without it, the pinning written down **silently** fails: no error, no
  // warning, just no sticking. This is the only place in the whole approach with a failure mode, so it lives in the
  // shared component.
  const canvasWidth = table.getTotalSize();
  // No column visibility feature is registered, so there is no getVisible*: all columns are present, getAll* will do.
  const columnCount = table.getAllLeafColumns().length;

  // While dragging, the width changes every frame, so storage is written once on release; each arrow-key press is
  // already a complete adjustment and is written as usual.
  const resizing = table.getAllLeafColumns().some((c) => c.getIsResizing());
  useEffect(() => {
    if (storageKey === undefined || resizing || !widthTouched.current) return;
    writePreference(storageKey, JSON.stringify(widthMemory));
  }, [storageKey, resizing, widthMemory]);

  // Container width: when the sum of column widths is less, the remainder goes only to unpinned columns
  // (spreadWidths). The first frame measures it synchronously in a layout effect, so it does not draw one frame the
  // browser's way and then jump.
  const scrollRef = useRef<HTMLDivElement>(null);
  const [viewport, setViewport] = useState(0);
  useLayoutEffect(() => {
    const el = scrollRef.current;
    if (!el) return;
    setViewport(el.clientWidth);
    const observer = new ResizeObserver(() => setViewport(el.clientWidth));
    observer.observe(el);
    return () => observer.disconnect();
  }, []);
  const widths = spreadWidths(
    table.getAllLeafColumns().map((c) => ({
      id: c.id,
      size: c.getSize(),
      pinned: c.getIsPinned() !== false,
    })),
    viewport,
  );

  // Intersect the selection with the row ids actually rendered on the current page: after a successful bulk delete,
  // ids of deleted rows still in rowSelection disappear from the count and the bulk bar's arguments through this step,
  // without waiting for the result set clearing below to take effect (same page, same filters, only fewer rows).
  const currentRowIds = table.getRowModel().rows.map((r) => r.id);
  const selectedIds = intersectSelection(rowSelection, currentRowIds);
  const clearSelection = () => setRowSelection({});

  // Clear the selection when the result set, page or page size changes: **the selection does not cross pages or result
  // sets**. Safer than clearing only in goToPage/changePageSize: a filter change makes useList's internal paging state
  // set page straight back to 1 without going through those two callbacks, and that case must clear too. Without
  // resultSetKey the signature changes only with page/pageSize, the same behaviour as before callers passed it. Uses
  // the "adjusting state during render" pattern from the React docs: when the signature changes, clear in this render
  // directly instead of waiting for a useEffect on the next one.
  const selectionSignature = `${resultSetKey ?? ''}::${page}::${pageSize}`;
  const [prevSelectionSignature, setPrevSelectionSignature] =
    useState(selectionSignature);
  if (selectionSignature !== prevSelectionSignature) {
    setPrevSelectionSignature(selectionSignature);
    setRowSelection({});
  }

  // Changing page or page size clears the selection: **the selection does not cross pages**. With server-side paging, a
  // selection on another page is invisible: if 20 of 23 rows in a bulk delete are on other pages, the confirmation
  // cannot list them, and the cost of a mistaken delete falls entirely on the user. Reasons and trade-offs:
  // docs/admin/tables.md, "Bulk actions".
  const goToPage = (next: number) => {
    clearSelection();
    onPageChange?.(next);
  };
  const changePageSize = (next: number) => {
    clearSelection();
    onPageSizeChange?.(next);
  };

  return (
    // The shell is shadcn's Card: radius and border come from the same source as dashboard cards, and the horizontal
    // padding of all four sections is its --card-spacing, so the left edges of the filter header, the first table
    // column and the footer bar line up. The Card's own vertical padding and section gap are zeroed: sections are
    // separated by dividers.
    // min-w-0: when this layer is a flex item, the default `min-width: auto` stretches it to the content width and the
    // overflow-x-auto inside does nothing: the table still pushes the whole page into a horizontal scrollbar.
    <Card className="min-w-0 gap-0 py-0">
      {toolbar ? (
        <div className="border-b p-(--card-spacing)">{toolbar}</div>
      ) : null}

      {bulkActions ? (
        <div className="flex flex-wrap items-center gap-2 border-b px-(--card-spacing) py-2">
          {bulkActions(selectedIds, clearSelection)}
        </div>
      ) : null}

      <div ref={scrollRef} className="min-w-0 overflow-x-auto">
        {/* When the column widths sum to less than the container, the table fills it; otherwise a white strip
            remains on the right, and the actions column pinned at the end stops left of it, looking "not pinned".
            spreadWidths gives the extra width only to unpinned columns; `minWidth: 100%` covers only the moment
            before the container width is measured. When filled there is no horizontal scroll anyway, so pinning not
            taking effect does no harm. */}
        <Table
          // The first column's start padding aligns with the card padding (the left edge of the other sections). Only
          // the
          // first column: between cells the width is already the two sides' padding added, and the last column holds
          // action buttons sized by measured width, so wider padding would eat the margin the page left.
          className="**:data-[slot=table-cell]:first:ps-(--card-spacing) **:data-[slot=table-head]:first:ps-(--card-spacing)"
          style={{
            width: canvasWidth,
            minWidth: '100%',
            tableLayout: 'fixed',
          }}
        >
          <TableHeader>
            {table.getHeaderGroups().map((group) => (
              <TableRow key={group.id}>
                {group.headers.map((header, i) => (
                  <TableHead
                    key={header.id}
                    style={{
                      width: widths[header.column.id] ?? header.getSize(),
                      ...pinnedStyle(header.column.getIsPinned(), {
                        start: header.column.getStart('start'),
                        end: header.column.getAfter('end'),
                      }),
                    }}
                    // Headers use the secondary text color: the hierarchy against the data comes from color, the
                    // weight stays the component's default medium, so headers in narrow columns stay easy to read (as
                    // in the reference shadcn admin).
                    className={cn(
                      'relative text-muted-foreground',
                      header.column.getIsPinned() && PINNED_CELL,
                    )}
                  >
                    {/* A text header longer than its column (a narrow default, or a column dragged narrow) is cut
                        with an ellipsis instead of running into the next column, the full text in the hover title. A
                        header that is a control (the select-all checkbox) is left alone: clipping would cut its focus
                        ring. */}
                    {header.isPlaceholder ? null : typeof header.column
                        .columnDef.header === 'string' ? (
                      <span
                        className="block truncate"
                        title={header.column.columnDef.header}
                      >
                        <table.FlexRender header={header} />
                      </span>
                    ) : (
                      <table.FlexRender header={header} />
                    )}
                    {/* No handle on the last column: there is no next column to its right to squeeze, dragging it
                        only changes its own width, and it is usually the actions column, whose width is fixed by its
                        buttons. */}
                    {i < group.headers.length - 1 ? (
                      <ResizeHandle
                        label={t('table.resizeColumn')}
                        width={header.getSize()}
                        min={header.column.columnDef.minSize ?? 60}
                        onPointerDown={header.getResizeHandler()}
                        onStep={(delta) =>
                          setSizing((prev) => ({
                            ...prev,
                            [header.column.id]: Math.max(
                              header.column.columnDef.minSize ?? 60,
                              header.getSize() + delta,
                            ),
                          }))
                        }
                        onReset={() =>
                          setSizing(
                            ({ [header.column.id]: _, ...rest }) => rest,
                          )
                        }
                      />
                    ) : null}
                  </TableHead>
                ))}
              </TableRow>
            ))}
          </TableHeader>
          <TableBody>
            {state === 'failed' ? (
              <TableRow>
                {/* When the table is wider than the container, a cell spanning all columns centres its content in the
                    middle of the whole table, scrolled out of view. Pinned to the left edge with the measured container
                    width, the content lands in the visible part. */}
                <TableCell colSpan={columnCount} className="p-0">
                  <div
                    className="sticky left-0"
                    style={viewport ? { width: viewport } : undefined}
                  >
                    <LoadFailed onRetry={() => retryFailed(loads)} />
                  </div>
                </TableCell>
              </TableRow>
            ) : state === 'loading' ? (
              <SkeletonRows columns={columnCount} pageSize={pageSize} />
            ) : table.getRowModel().rows.length === 0 ? (
              <TableRow>
                <TableCell colSpan={columnCount} className="p-0">
                  <div
                    className="sticky left-0 flex h-24 items-center justify-center text-muted-foreground"
                    style={viewport ? { width: viewport } : undefined}
                  >
                    {empty ?? t('table.empty')}
                  </div>
                </TableCell>
              </TableRow>
            ) : (
              table
                .getRowModel()
                .rows.map((row) => (
                  <DataRow
                    key={row.id}
                    row={row}
                    table={table}
                    widths={widths}
                  />
                ))
            )}
          </TableBody>
        </Table>
      </div>

      {total === undefined && !selectable ? null : (
        <TableFooterBar
          selectable={selectable}
          selectedCount={selectedIds.length}
          rowCount={table.getRowModel().rows.length}
          total={total}
          page={page}
          pageSize={pageSize}
          pageSizes={pageSizes}
          onPageChange={goToPage}
          onPageSizeChange={changePageSize}
        />
      )}
    </Card>
  );
}

/** The fixed id of the selection column. Pages need not know it, unless to avoid the name in columns. */
const SELECT_COLUMN_ID = '__select';

/**
 * Selection column: a column of checkboxes with a fixed width and no column width handle (it has no content length).
 *
 * Select-all in the header **covers only the current page**: selection does not cross pages, and "select all" can
 * only select what is in view anyway. Uses getIsAllRowsSelected rather than getIsAllPageRowsSelected: with no
 * pagination feature registered, the table's "all rows" are the one page the server gave.
 */
function selectColumn<T extends RowData>(
  t: TFunction<'sys'>,
): DataTableColumn<T> {
  return {
    id: SELECT_COLUMN_ID,
    size: 44,
    minSize: 44,
    header: ({ table }) => (
      <Checkbox
        checked={table.getIsAllRowsSelected()}
        // Base UI's indeterminate state is a separate prop, not a third value of checked; it becomes
        // aria-checked="mixed", so a screen reader can announce "partially selected".
        // "Partially selected" counts the rows in view, not getIsSomeRowsSelected: that only checks whether
        // rowSelection has keys, and ids of deleted rows stay in it after a bulk delete, so with the list deleted empty
        // the header would still be indeterminate (the footer count uses intersectSelection for the same reason).
        indeterminate={
          table.getRowModel().rows.some((r) => r.getIsSelected()) &&
          !table.getIsAllRowsSelected()
        }
        // The indeterminate **look** must be supplied here: the generated Checkbox renders the same check mark in every
        // state, so "2 of 3 selected" looks exactly like "all selected". Hide the check and draw a dash: aria-checked
        // already states the meaning, this adds the visible half. The style is at the call site rather than in
        // ui/checkbox.tsx: that file is CLI-generated, and the next --overwrite would lose the change.
        className="data-indeterminate:border-primary data-indeterminate:bg-primary data-indeterminate:text-primary-foreground data-indeterminate:before:absolute data-indeterminate:before:h-0.5 data-indeterminate:before:w-2 data-indeterminate:before:rounded-full data-indeterminate:before:bg-current data-indeterminate:before:content-[''] data-indeterminate:[&_svg]:hidden"
        onCheckedChange={(v) => table.toggleAllRowsSelected(!!v)}
        aria-label={t('table.selectAll')}
      />
    ),
    cell: ({ row }) => (
      <Checkbox
        checked={row.getIsSelected()}
        onCheckedChange={(v) => row.toggleSelected(!!v)}
        aria-label={t('table.selectRow')}
      />
    ),
  };
}

function DataRow<T extends RowData>({
  row,
  table,
  widths,
}: {
  row: Row<DataTableFeatures, T>;
  table: ReturnType<typeof useTable<DataTableFeatures, T>>;
  widths: Record<string, number>;
}) {
  return (
    <TableRow>
      {row.getAllCells().map((cell) => (
        <TableCell
          key={cell.id}
          style={{
            width: widths[cell.column.id],
            ...pinnedStyle(cell.column.getIsPinned(), {
              start: cell.column.getStart('start'),
              end: cell.column.getAfter('end'),
            }),
          }}
          className={cn('truncate', cell.column.getIsPinned() && PINNED_CELL)}
        >
          <table.FlexRender cell={cell} />
        </TableCell>
      ))}
    </TableRow>
  );
}

// v9 pinning positions are **logical** directions (start/end, following the writing direction), so offsets are
// written as inset-inline-* too: no second flip under RTL.
const pinnedStyle = (
  pinned: false | 'start' | 'end',
  offset: { start: number; end: number },
): React.CSSProperties =>
  pinned === 'start'
    ? { insetInlineStart: offset.start }
    : pinned === 'end'
      ? { insetInlineEnd: offset.end }
      : {};

/**
 * Column width handle. **Both pointer and keyboard must work**: these rules have no automatic column widths and leave
 * dragging to the user, so a pointer-only handle takes this ability away from keyboard users. Arrow keys adjust by
 * 8px, Shift speeds it up to 32px. Double-click restores the column's default width in the current language; restoring
 * has only this one pointer path, a deliberate trade-off (docs/admin/tables.md, "Why these rules").
 *
 * A native `<button>` is an exception to "prefer shadcn controls": shadcn has no column width handle, and its Button's
 * built-in size and background are exactly what an 8px-wide transparent hit area does not want.
 */
function ResizeHandle({
  label,
  width,
  min,
  onPointerDown,
  onStep,
  onReset,
}: {
  label: string;
  width: number;
  min: number;
  onPointerDown: (e: React.PointerEvent | React.MouseEvent) => void;
  onStep: (delta: number) => void;
  onReset: () => void;
}) {
  return (
    // biome-ignore lint/a11y/useSemanticElements: handle takes focus and keys, <hr> cannot (docs/admin/tables.md)
    <button
      type="button"
      role="separator"
      aria-orientation="vertical"
      aria-label={label}
      // Report the current column width: only then does a screen reader have something to announce while arrow keys
      // adjust it; otherwise it is adjusting blind.
      aria-valuenow={width}
      aria-valuemin={min}
      // The hit area is twice the visual line's width: a 1px line is nearly impossible to hit with a mouse. The hit
      // area lies entirely within this column and does not cross the column boundary: when the right neighbour is a
      // pinned column it is a sticky stacking context and would cover the half reaching into it (no dragging, no
      // double-click); when this column is itself pinned, it cannot escape its own stacking context either.
      className="absolute right-0 top-0 h-full w-2 cursor-col-resize touch-none select-none border-transparent focus-visible:outline-2 focus-visible:outline-ring"
      onPointerDown={onPointerDown}
      onMouseDown={onPointerDown}
      onDoubleClick={onReset}
      onKeyDown={(e) => {
        const step = e.shiftKey ? 32 : 8;
        if (e.key === 'ArrowLeft') {
          e.preventDefault();
          onStep(-step);
        } else if (e.key === 'ArrowRight') {
          e.preventDefault();
          onStep(step);
        }
      }}
    >
      <span className="pointer-events-none absolute top-1 right-0 bottom-1 w-px bg-border" />
    </button>
  );
}

function SkeletonRows({
  columns,
  pageSize,
}: {
  columns: number;
  pageSize: number;
}) {
  const rows = Math.min(pageSize, 8);
  return (
    <>
      {Array.from({ length: rows }, (_, i) => (
        // biome-ignore lint/suspicious/noArrayIndexKey: skeleton rows have no identity, the index is all there is
        <TableRow key={i}>
          {Array.from({ length: columns }, (_, j) => (
            // biome-ignore lint/suspicious/noArrayIndexKey: as above
            <TableCell key={j}>
              <Skeleton className="h-4 w-full" />
            </TableCell>
          ))}
        </TableRow>
      ))}
    </>
  );
}

/**
 * Footer bar: the selection state on the left, pagination on the right. The split is deliberate: a selected count and
 * its actions at opposite ends would make people look in both places, so the count stays in its standard place here,
 * and the number on the button ("Delete (3)") identifies itself at the bulk bar's end.
 *
 * "N in total" exists only with server-side paging and is not in the reference implementation; it stays, because the
 * first thing someone paging wants to know is the total. Conversely "Page 1/5" was removed: the highlighted item in the
 * paginator is the current page, and the last number is the page count, so writing it again says the same thing twice.
 */
function TableFooterBar({
  selectable,
  selectedCount,
  rowCount,
  total,
  page,
  pageSize,
  pageSizes,
  onPageChange,
  onPageSizeChange,
}: {
  selectable: boolean;
  selectedCount: number;
  rowCount: number;
  total?: number;
  page: number;
  pageSize: number;
  pageSizes: readonly number[];
  onPageChange: (page: number) => void;
  onPageSizeChange: (pageSize: number) => void;
}) {
  const { t } = useTranslation();
  return (
    <div className="flex flex-wrap items-center justify-between gap-3 border-t px-(--card-spacing) py-2 text-sm">
      {/* An empty span must still take its place: without it, justify-between pushes the right-hand content of a table
          with only pagination to the left. */}
      <span className="text-muted-foreground">
        {selectable
          ? t('table.selected', { count: selectedCount, rows: rowCount })
          : null}
      </span>
      {total === undefined ? null : (
        <div className="flex flex-wrap items-center justify-end gap-3">
          <span className="text-muted-foreground">
            {t('table.total', { count: total })}
          </span>
          <Select
            value={String(pageSize)}
            onValueChange={(v) => onPageSizeChange(Number(v))}
            items={Object.fromEntries(
              pageSizes.map((n) => [
                String(n),
                t('table.perPage', { count: n }),
              ]),
            )}
          >
            <SelectTrigger
              size="sm"
              className="w-28"
              aria-label={t('table.pageSize')}
            >
              <SelectValue />
            </SelectTrigger>
            <SelectContent>
              <SelectGroup>
                {pageSizes.map((n) => (
                  <SelectItem key={n} value={String(n)}>
                    {t('table.perPage', { count: n })}
                  </SelectItem>
                ))}
              </SelectGroup>
            </SelectContent>
          </Select>
          <Pager
            page={page}
            pages={pageCount(total, pageSize)}
            onPageChange={onPageChange}
          />
        </div>
      )}
    </div>
  );
}

/**
 * Paginator. Paging changes only the URL search, and navigation is started by the page's onPageChange, so the
 * `href="#"` here only gives shadcn's Pagination the `<a>` it wants; every click calls preventDefault.
 *
 * The first-page and last-page buttons hide on narrow screens: a row of seven or eight controls does not fit on a small
 * screen, and what they save is "a few more clicks", not "unreachable".
 */
function Pager({
  page,
  pages,
  onPageChange,
}: {
  page: number;
  pages: number;
  onPageChange: (page: number) => void;
}) {
  const { t } = useTranslation();
  const current = Math.min(Math.max(1, page), pages);
  const win = pageWindow(current, pages);
  // pageWindow guarantees non-empty, but noUncheckedIndexedAccess does not accept the guarantee.
  const firstShown = win[0] ?? 1;
  const lastShown = win[win.length - 1] ?? pages;
  const canPrev = current > 1;
  const canNext = current < pages;
  const stop = (e: React.MouseEvent) => e.preventDefault();
  const disabled = 'pointer-events-none opacity-50';

  return (
    <Pagination className="mx-0 w-auto justify-end">
      <PaginationContent className="gap-1">
        <PaginationItem className="hidden lg:block">
          <PaginationLink
            href="#"
            aria-label={t('table.firstPage')}
            aria-disabled={!canPrev}
            className={cn(!canPrev && disabled)}
            onClick={(e) => {
              stop(e);
              if (canPrev) onPageChange(1);
            }}
          >
            <ChevronsLeftIcon />
          </PaginationLink>
        </PaginationItem>
        <PaginationItem>
          <PaginationPrevious
            href="#"
            text={t('table.prev')}
            aria-label={t('table.prevPage')}
            aria-disabled={!canPrev}
            className={cn(!canPrev && disabled)}
            onClick={(e) => {
              stop(e);
              if (canPrev) onPageChange(current - 1);
            }}
          />
        </PaginationItem>
        {firstShown > 1 ? (
          <PaginationItem>
            <PaginationEllipsis />
          </PaginationItem>
        ) : null}
        {win.map((n) => (
          <PaginationItem key={n}>
            <PaginationLink
              href="#"
              isActive={n === current}
              onClick={(e) => {
                stop(e);
                onPageChange(n);
              }}
            >
              {n}
            </PaginationLink>
          </PaginationItem>
        ))}
        {lastShown < pages ? (
          <PaginationItem>
            <PaginationEllipsis />
          </PaginationItem>
        ) : null}
        <PaginationItem>
          <PaginationNext
            href="#"
            text={t('table.next')}
            aria-label={t('table.nextPage')}
            aria-disabled={!canNext}
            className={cn(!canNext && disabled)}
            onClick={(e) => {
              stop(e);
              if (canNext) onPageChange(current + 1);
            }}
          />
        </PaginationItem>
        <PaginationItem className="hidden lg:block">
          <PaginationLink
            href="#"
            aria-label={t('table.lastPage')}
            aria-disabled={!canNext}
            className={cn(!canNext && disabled)}
            onClick={(e) => {
              stop(e);
              if (canNext) onPageChange(pages);
            }}
          >
            <ChevronsRightIcon />
          </PaginationLink>
        </PaginationItem>
      </PaginationContent>
    </Pagination>
  );
}

/**
 * Names columns missing size in development mode. This is exactly the warning the underlying library lacks: without a
 * width, pinning silently fails and columns collapse to 0 under the fixed layout. The fallback width keeps the page
 * from
 * collapsing, **but the fallback is not permission**: a table relying on it has random column widths.
 */
function warnMissingSizes<T extends RowData>(
  columns: DataTableColumn<T>[],
): void {
  if (!import.meta.env.DEV) return;
  const missing = columns.filter((c) => c.size === undefined).map(columnIdOf);
  if (missing.length > 0) {
    console.warn(
      `DataTable: these columns have no size and render with the ${FALLBACK_COLUMN_WIDTH}px fallback: ${missing.join(', ')}`,
    );
  }
}

const columnIdOf = <T extends RowData>(c: DataTableColumn<T>): string =>
  c.id ?? String((c as { accessorKey?: unknown }).accessorKey);

/** The column info column width memory needs; the fallbacks for default width and minimum match `defaultColumn`. */
function columnWidthDefs<T extends RowData>(
  columns: DataTableColumn<T>[],
): ColumnWidthDef[] {
  return columns.map((c) => ({
    id: columnIdOf(c),
    size: c.size ?? FALLBACK_COLUMN_WIDTH,
    minSize: c.minSize ?? 60,
  }));
}
