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
import {
  ChevronDownIcon,
  ChevronsLeftIcon,
  ChevronsRightIcon,
  ExternalLinkIcon,
  MoreHorizontalIcon,
} from 'lucide-react';
import {
  Fragment,
  type ReactNode,
  useEffect,
  useLayoutEffect,
  useMemo,
  useRef,
  useState,
} from 'react';
import { useTranslation } from 'react-i18next';
import { ActionDialog, ConfirmPopover } from '@/components/containers';
import { LoadFailed } from '@/components/data-region';
import { Button } from '@/components/ui/button';
import { ButtonGroup } from '@/components/ui/button-group';
import { Card } from '@/components/ui/card';
import { Checkbox } from '@/components/ui/checkbox';
import {
  DropdownMenu,
  DropdownMenuContent,
  DropdownMenuGroup,
  DropdownMenuItem,
  DropdownMenuLabel,
  DropdownMenuSeparator,
  DropdownMenuSub,
  DropdownMenuSubContent,
  DropdownMenuSubTrigger,
  DropdownMenuTrigger,
} from '@/components/ui/dropdown-menu';
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
import {
  Tooltip,
  TooltipContent,
  TooltipTrigger,
} from '@/components/ui/tooltip';
import { loadState, type QueryLike, retryFailed } from '@/lib/load-state';
import { readPreference, writePreference } from '@/lib/preference';
import {
  type ColumnWidthDef,
  type ColumnWidthMemory,
  columnWidthStorageKey,
  FALLBACK_COLUMN_WIDTH,
  inlineCount,
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
                    {header.isPlaceholder ? null : (
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
 * Row actions: what the page writes is what the row shows. Each cell of the actions column sets `kind`, chosen by
 * these three cases:
 *
 * - `button`: a single action the page always shows, or one bound to one permission code;
 * - `menu`: a set of actions belonging to one business domain. The button text is the set's group name ("Funds ⌄"),
 *   not a specific action; without `label` it is a "⋯" icon button. When the user has no permission for any item in
 *   the set, the cell is not rendered, so the page need not check for empty;
 * - `split`: like `menu`, but the action marked `primary` moves to the left half as a direct button, and the ⋯ on the
 *   right half holds the rest. **Without an item marked `primary` the whole cell is drawn as a `menu`**: when the
 *   primary action has no permission, the second one does not move up, because the column width is fixed and names
 *   that move up vary in length, so the column would need space for the longest. A dangerous action is not moved up
 *   even when marked (putting delete or disable in the handiest place should be avoided anyway); if it is the only
 *   one left after moving up, it is an ordinary button.
 *
 * The choice comes down to one test: **can these actions get a group name the user understands at a glance**. If not
 * (only "More" or "Other" fits), they are not a group: give each its own `button` cell, and leave what does not fit
 * to `maxInline`. What none of the three can express (a switch, a status badge, a progress bar) is written as JSX in
 * `cell` directly, and the kernel promises nothing about that column; but the container rules are site-wide, and a
 * hand-written column is still bound by them.
 *
 * A menu item is either an action or a named set of actions (`RowActionGroup`, which may nest). **Flatten when it
 * can be flattened, nest when it cannot**: groups at the menu's first level are laid flat as "title + those items",
 * deeper ones become submenus; when `maxInline` folds this cell into "⋯", everything drops one level and the formerly
 * flat level uses submenus: two levels of titles on one plane do not show which belongs to which. The trade-off is in
 * `docs/admin/tables.md`, "Actions column".
 *
 * The action itself (the same whether shown on a button or tucked into a menu) is one of three:
 * - `onSelect` is an in-place action (open a drawer, open a dialog);
 * - `confirm` is an action that needs confirming first: when its button sits directly on the row, a popover anchored
 *   to it confirms; inside a menu a dialog confirms instead, because once the menu closes the popover has no button to
 *   anchor to (the container rules). This split happens once, here; the page gives only the text and callback;
 * - `href` **goes to the list related to this row** with the filter values set. It must be a real link, not
 *   `window.open`: middle-click, Cmd+click and right-click "copy link" are what a user rightly expects from a link,
 *   and simulating it with a click callback takes them all away. An item with href gets a ↗ suffix and opens in a new
 *   browser tab: the current page's filter state should not be replaced.
 */
export type RowAction = {
  key: string;
  label: string;
  /**
   * The icon to the left of the menu item. Whether it is drawn on a **button** is up to the cell: `button` and `menu`
   * draw it when given, `split` needs `showIcon`; icons do not appear on their own, since an icon on a button means
   * the column's width must be measured again.
   */
  icon?: ReactNode;
  onSelect?: () => void;
  /** The address of the related list (with filter values). */
  href?: string;
  confirm?: {
    title: string;
    description?: string;
    needOtp?: boolean;
    /**
     * The sentence to say after success; do not give it for actions with a visible change on screen (see
     * ActionDialog). Fixed text only: the actions of a row share one array with differing return types, which cannot
     * match a toast function's argument; an action whose message depends on what it returns uses `onSelect` to open
     * an ActionDialog.
     */
    success?: string;
    onConfirm: (otpCode: string) => Promise<void>;
  };
  /**
   * The color of the button on the row; a plain outline when omitted. `positive` is the one that moves the queue a
   * step forward (confirm, approve), at most one per row; `destructive` is a dangerous action, and the confirm button
   * of its confirmation turns red with it.
   */
  tone?: 'positive' | 'destructive';
  /**
   * This item is the primary action of its `split`, moved to the left half button. Only the first marked one in a cell
   * counts; after the page filters by permission it may not be in the array at all, and the cell then falls back to a
   * `menu` by itself. The kernel cannot see filtered-out items, and without this mark could not tell whether the first
   * item is the primary action or one that moved up.
   */
  primary?: boolean;
  /**
   * Unavailable for this row right now: the button on the row and the menu item are disabled and cannot be clicked,
   * and `onSelect` / `href` / `confirm` do not fire. The page passes availability as decided by the server; the kernel
   * does not infer it.
   */
  disabled?: boolean;
  /**
   * Why it is unavailable, shown only with `disabled`: as a hover tooltip on the row's button, as a line under the name
   * in a menu item.
   */
  disabledReason?: string;
};

/** A named set of actions, which may nest. */
export type RowActionGroup = {
  key: string;
  label: string;
  items: RowActionNode[];
};

/** One menu item: an action, or a named set of actions. */
export type RowActionNode = RowAction | RowActionGroup;

/** One cell of the actions column. */
export type RowActionItem =
  | ({ kind: 'button' } & RowAction)
  | {
      kind: 'menu';
      key: string;
      /** Group name. Without it, rendered as a "⋯" icon button. */
      label?: string;
      /** The icon left of the group name; the unnamed "⋯" ignores it, being an icon itself. */
      icon?: ReactNode;
      items: RowActionNode[];
    }
  | {
      kind: 'split';
      key: string;
      /**
       * Group name. The left half button shows the name of the action moved up; the group name serves as the section
       * title when folded into "⋯".
       */
      label: string;
      /**
       * Whether the left half button draws that action's icon. Off by default: icons on buttons are heavy and noisy
       * down a whole column, and drawing them means measuring the column width again. None of the kernel's system
       * pages turns it on; icons appear only in menus.
       */
      showIcon?: boolean;
      items: RowActionNode[];
    };

function isGroup(n: RowActionNode): n is RowActionGroup {
  return 'items' in n;
}

/**
 * Recursively removes empty groups. After the page filters item by item by permission, a group may have nothing left,
 * and it may itself sit inside another group; looking only at the first level would leave an entry that opens to just
 * a title, or an empty submenu.
 */
function prune(nodes: RowActionNode[]): RowActionNode[] {
  return nodes.flatMap<RowActionNode>((n) => {
    if (!isGroup(n)) return [n];
    const items = prune(n.items);
    return items.length > 0 ? [{ ...n, items }] : [];
  });
}

export function RowActions({
  actions,
  maxInline,
}: {
  actions: RowActionItem[];
  /**
   * The most cells this column holds, **with the unnamed "⋯" at the end counting as one**: if everything fits, all
   * are laid flat; if not, the last cell goes to "⋯" and the rest fold into it. The column width is therefore exactly
   * this many cells wide, so measuring the column width and writing this number are the same thing. **Without it
   * everything is laid flat**: how many fit is this table's business, not the kernel's. Write it only when the number
   * of cells varies with permissions and a narrow table cannot hold them; when unsure, leave it out.
   */
  maxInline?: number;
}) {
  // The item tucked into a menu that needs confirming: falls back to a dialog for confirmation.
  const [pending, setPending] = useState<RowAction>();
  // After filtering item by item by permission nothing may be left: first recursively remove empty groups, then check
  // whether the cell has anything left. An empty cell is not rendered, so no button opens to nothing; the flattening,
  // folding and split's primary action below all use this cleaned-up copy.
  const shown = actions.flatMap<RowActionItem>((a) => {
    if (a.kind === 'button') return [a];
    const items = prune(a.items);
    return items.length > 0 ? [{ ...a, items }] : [];
  });
  if (shown.length === 0) return null;
  const inline = shown.slice(0, inlineCount(shown.length, maxInline));
  const folded = shown.slice(inline.length);

  return (
    <div className="flex items-center gap-1">
      {inline.map((a) => (
        <InlineItem key={a.key} item={a} onPending={setPending} />
      ))}
      {folded.length > 0 ? (
        <ActionMenu>
          <FoldedBody items={folded} onPending={setPending} />
        </ActionMenu>
      ) : null}
      {pending?.confirm ? (
        <ActionDialog
          open
          onOpenChange={(next) => {
            if (!next) setPending(undefined);
          }}
          title={pending.confirm.title}
          description={pending.confirm.description}
          needOtp={pending.confirm.needOtp}
          destructive={pending.tone === 'destructive'}
          confirmLabel={pending.label}
          success={pending.confirm.success}
          onConfirm={(otp) => pending.confirm?.onConfirm(otp)}
        />
      ) : null}
    </div>
  );
}

/** How one cell looks on the row. */
function InlineItem({
  item: a,
  onPending,
}: {
  item: RowActionItem;
  onPending: (a: RowAction) => void;
}) {
  if (a.kind === 'button') return <ActionButton action={a} showIcon />;
  // split moves up the action marked primary: only at the top level (groups cannot be clicked), and not a dangerous
  // one. When none can be moved up it is the same as menu: a named set of actions, drawn as one menu.
  const at =
    a.kind === 'split'
      ? a.items.findIndex(
          (n) => !isGroup(n) && n.primary && n.tone !== 'destructive',
        )
      : -1;
  const primary = at < 0 ? undefined : (a.items[at] as RowAction);
  const rest = primary ? a.items.filter((_, i) => i !== at) : a.items;
  if (!primary) {
    return (
      <ActionMenu label={a.label} icon={a.kind === 'menu' ? a.icon : undefined}>
        <MenuNodes nodes={rest} onPending={onPending} />
      </ActionMenu>
    );
  }
  const button = (
    <ActionButton
      action={primary}
      showIcon={a.kind === 'split' && a.showIcon}
    />
  );
  if (rest.length === 0) return button;
  return (
    <ButtonGroup>
      {button}
      <ActionMenu>
        <MenuNodes nodes={rest} onPending={onPending} />
      </ActionMenu>
    </ButtonGroup>
  );
}

/**
 * The cells `maxInline` folds into "⋯": one section per cell, headed by its group name, its contents one level down.
 */
function FoldedBody({
  items,
  onPending,
}: {
  items: RowActionItem[];
  onPending: (a: RowAction) => void;
}) {
  return items.map((a, i) => {
    // Consecutive single actions sit together; each group is its own section.
    const prev = items[i - 1];
    const sep =
      prev !== undefined && (a.kind !== 'button' || prev.kind !== 'button');
    return (
      <Fragment key={a.key}>
        {sep ? <DropdownMenuSeparator /> : null}
        {a.kind === 'button' ? (
          <MenuAction action={a} onPending={onPending} />
        ) : (
          // The title must be inside the Group: Base UI's GroupLabel connects aria-labelledby through the group's
          // context, and outside it throws MenuGroupContext is missing.
          <DropdownMenuGroup>
            {a.label ? <DropdownMenuLabel>{a.label}</DropdownMenuLabel> : null}
            <MenuNodes nodes={a.items} nested onPending={onPending} />
          </DropdownMenuGroup>
        )}
      </Fragment>
    );
  });
}

/**
 * Menu contents. Groups are laid flat as "title + those items"; once `nested` (already under a title, or this cell
 * folded into "⋯" and taking the section title) they become submenus: two levels of titles on one plane do not show
 * which governs which items.
 */
function MenuNodes({
  nodes,
  nested,
  onPending,
}: {
  nodes: RowActionNode[];
  nested?: boolean;
  onPending: (a: RowAction) => void;
}) {
  return nodes.map((n, i) => {
    const prev = nodes[i - 1];
    const sep = prev !== undefined && (isGroup(n) || isGroup(prev));
    return (
      <Fragment key={n.key}>
        {sep ? <DropdownMenuSeparator /> : null}
        {!isGroup(n) ? (
          <MenuAction action={n} onPending={onPending} />
        ) : !nested ? (
          // The title is inside the Group, for the same reason as in FoldedBody.
          <DropdownMenuGroup>
            <DropdownMenuLabel>{n.label}</DropdownMenuLabel>
            <MenuNodes nodes={n.items} nested onPending={onPending} />
          </DropdownMenuGroup>
        ) : (
          <DropdownMenuSub>
            <DropdownMenuSubTrigger>{n.label}</DropdownMenuSubTrigger>
            <DropdownMenuSubContent>
              <MenuNodes nodes={n.items} nested onPending={onPending} />
            </DropdownMenuSubContent>
          </DropdownMenuSub>
        )}
      </Fragment>
    );
  });
}

/** One action in a menu. One that needs confirming is handed back up to confirm in a dialog. */
function MenuAction({
  action: a,
  onPending,
}: {
  action: RowAction;
  onPending: (a: RowAction) => void;
}) {
  if (a.disabled) {
    return (
      <DropdownMenuItem
        disabled
        className={cn(a.tone === 'destructive' && 'text-destructive')}
      >
        {a.icon}
        {a.disabledReason ? (
          <span className="flex flex-col">
            {a.label}
            <span className="text-xs text-muted-foreground">
              {a.disabledReason}
            </span>
          </span>
        ) : (
          a.label
        )}
      </DropdownMenuItem>
    );
  }
  if (a.href) {
    return (
      <DropdownMenuItem
        render={<a href={a.href} target="_blank" rel="noopener" />}
      >
        {a.icon}
        {a.label}
        <ExternalLinkIcon />
      </DropdownMenuItem>
    );
  }
  return (
    <DropdownMenuItem
      onClick={a.confirm ? () => onPending(a) : a.onSelect}
      className={cn(a.tone === 'destructive' && 'text-destructive')}
    >
      {a.icon}
      {a.label}
    </DropdownMenuItem>
  );
}

/**
 * One action shown on the row. Dangerous ones use the built-in destructive variant (light red background, red text),
 * positive ones swap the outline for green text and a green border, the rest are outlined. Positive gets no background:
 * a column of a dozen rows with two or three buttons each would be noisy in full color; it only needs to stand apart
 * from its neighbours. Green is `--success`, not the brand preset, for the same reason as in `StatusBadge`.
 */
function ActionButton({
  action: a,
  showIcon,
}: {
  action: RowAction;
  /** Off by default: unspecified means not drawn, so a new call site added someday does not forget to turn it off. */
  showIcon?: boolean;
}) {
  const variant = a.tone === 'destructive' ? 'destructive' : 'outline';
  const className =
    a.tone === 'positive'
      ? 'border-success/40 text-success hover:text-success dark:border-success/40'
      : undefined;
  // data-icon makes Button narrow the padding on the icon side (a shadcn convention); icon is any node, so the
  // attribute goes on a wrapping span, and contents creates no extra box.
  const icon =
    showIcon && a.icon ? (
      <span className="contents" data-icon="inline-start">
        {a.icon}
      </span>
    ) : null;
  if (a.disabled) {
    const button = (
      <Button variant={variant} className={className} size="sm" disabled>
        {icon}
        {a.label}
      </Button>
    );
    if (!a.disabledReason) return button;
    // A disabled button receives no mouse events, so the tooltip goes on the wrapping layer (shadcn's way to add a
    // tooltip to a disabled button). The radius belongs to this layer and the button inherits it: split's ButtonGroup
    // removes this layer's radius, and without inheriting, the seam between a disabled primary action and the "⋯" on
    // its right would show a gap.
    return (
      <Tooltip>
        <TooltipTrigger
          render={
            <span className="inline-flex w-fit rounded-lg *:rounded-[inherit]!" />
          }
        >
          {button}
        </TooltipTrigger>
        <TooltipContent>{a.disabledReason}</TooltipContent>
      </Tooltip>
    );
  }
  if (a.href) {
    return (
      <Button
        variant={variant}
        className={className}
        size="sm"
        nativeButton={false}
        render={<a href={a.href} target="_blank" rel="noopener" />}
      >
        {icon}
        {a.label}
        <ExternalLinkIcon data-icon="inline-end" />
      </Button>
    );
  }
  if (a.confirm) {
    return (
      <ConfirmPopover
        trigger={
          <Button variant={variant} className={className} size="sm">
            {icon}
            {a.label}
          </Button>
        }
        title={a.confirm.title}
        description={a.confirm.description}
        needOtp={a.confirm.needOtp}
        destructive={a.tone === 'destructive'}
        confirmLabel={a.label}
        success={a.confirm.success}
        onConfirm={a.confirm.onConfirm}
      />
    );
  }
  return (
    <Button
      variant={variant}
      className={className}
      size="sm"
      onClick={a.onSelect}
    >
      {icon}
      {a.label}
    </Button>
  );
}

/** The menu shell: with a group name it is "Funds ⌄", without one a "⋯" icon button. */
function ActionMenu({
  label,
  icon,
  children,
}: {
  label?: string;
  icon?: ReactNode;
  children: ReactNode;
}) {
  const { t } = useTranslation();
  return (
    <DropdownMenu>
      <DropdownMenuTrigger
        render={
          <Button
            variant="outline"
            size={label ? 'sm' : 'icon-sm'}
            aria-label={label ? undefined : t('table.moreActions')}
          />
        }
      >
        {label && icon ? (
          <span className="contents" data-icon="inline-start">
            {icon}
          </span>
        ) : null}
        {label}
        {label ? (
          <ChevronDownIcon data-icon="inline-end" />
        ) : (
          <MoreHorizontalIcon />
        )}
      </DropdownMenuTrigger>
      <DropdownMenuContent align="end">{children}</DropdownMenuContent>
    </DropdownMenu>
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
