<!-- Shared file: carried verbatim; never localized outside its source repository.
     Checked by scripts/check-cross-repo-sync.sh; see docs/upgrading-protocol.md. -->

# Tables and list pages

Read this before writing or changing an admin list: table columns and widths, the actions column, bulk actions,
pagination, the filter bar. This page is a frontend convention of keel-admin and has no counterpart among the guides in
`docs/guides/`; the backend shape of list endpoints is in `docs/guides/api-endpoint.md` "List endpoints are paged by
default" and `docs/admin/api-endpoint.md`. How list definitions, URLs and pagination are wired is in
`docs/admin/forms.md` "List page URL and pagination"; "identity column", "actions column" and "selection column" are
defined in `GLOSSARY.md`. Live example: the kernel's `web/src/routes/_app/sys/users.tsx`.

## Pinned columns and column widths

**The identity column is pinned left and the actions column pinned right; every column has an explicit numeric width,
and widths can be dragged; a table given a `tableKey` remembers dragged widths. A tree table pins only the actions
column, not the hierarchy column.** The page declares pinning explicitly on the table; the shared part does not infer
it.

- The horizontal offset of a pinned column is computed from the column widths, so **without widths, the pinning you
  wrote silently fails**: no error, no warning, it just does not stick. A missing width is named in development mode
  and rendered with a fallback width, but the fallback is not permission: a column width is a property of its content,
  given column by column by whoever writes the table.
- **Default values are measured in English; the actions column gives a value measured in each UI language its area
  ships; other columns as needed** (`widthFor` in `lib/table.ts`; the language keys are checked against the
  `SupportedLocale` type, so a misspelling fails to compile). Kernel pages give English (the default) and Chinese, the
  languages the kernel ships; a business page gives every UI language the repository ships. A language without its own
  value falls back to the English default, so a language a derived repository adds keeps the English widths on kernel
  pages, which it cannot edit. Widths in styles (the sidebar and similar) are overridden per language with CSS
  `:lang()`, not in JS.
- **The actions column's width is the measured content width** (measure the longest set of buttons plus cell padding,
  then add 8px; with `maxInline`, the widest case is that many cells), with a comment beside it recording the measured
  number: it varies with permissions (the number of buttons) and language, and the next person who changes the copy
  needs to know the baseline.
- **To remember dragged column widths, pass `tableKey` to `DataTable`** (unique across the system, written
  `domain.page`, such as `sys.users`); without it, widths last for the session and reset on refresh. What is stored is
  a preference (in the browser, so it follows the device, not the account), kept per UI language: a drag applies only
  to the language active at the time, and after switching language the widths dragged in that language, or the
  defaults, are used. Double-clicking the handle restores that column's default for the current language.
- **When you place a table inside your own flex layout, give every flex level `min-w-0`**, or a wide table pushes out
  a page-level scrollbar.

Why these rules:

- **Widths are explicit because dragging needs a fixed table layout.** Under an auto layout the browser lays columns
  out by content and dragging narrower does nothing; under a fixed layout a column without a width gets a share of the
  leftover space or collapses, the scroll canvas is the sum of the widths, and the Nth pinned column's offset is the sum
  of the widths before it. Resizable, multi-pinned columns therefore cost auto-fit. Measuring the content in code and
  writing the number is fine, and suits the actions column, whose buttons are fixed per table; it does not suit content
  columns, whose width would change with the data: those truncate with hover, or the user drags them wider.
- **The page declares pinning because inference needs two kinds of rule**: "first column left" goes by position,
  "actions column right" by meaning, and a wrong guess needs a veto marker. The declaration is the table library's own
  column-pinning state and shows at a glance what sticks. A tree table leaves its hierarchy column unpinned: it is read
  together with its indentation, and pinning the widest column leaves the rest too little room.
- **Per-language widths are measured, never scaled by a ratio**: the length ratio between languages differs by page and
  by button combination, so a fixed ratio leaves some columns wrapping and others too wide. Style widths use `:lang()`
  because a CSS variable is already overridable in CSS; routing them through `shell.ts` would turn style values into
  props. The mobile sidebar keeps its width: its constant lives inside `components/ui/sidebar.tsx` with no documented
  override, and that third-party component is not edited; as a full-screen drawer its empty space does not stay in
  view.
- **Remembering widths is opt-in by `tableKey`** because two tables under one route cannot be told apart
  automatically: mount order shifts under conditional rendering, a signature of the column set breaks when a column is
  added, and ids like `id` or `actions` repeat in every table. Dragged widths are kept per language because the
  defaults already differ per language, and one drag must not override the width measured for another. On read,
  stored widths for columns that no longer exist are dropped and widths are clamped to the column's minimum; a changed
  default does not override a value the user dragged. The handle also takes the keyboard (arrow keys, Shift for larger
  steps), since auto-fit is left out in favour of dragging; reset is double-click only.
- **This is the default shape, not a gate**: a derived repo whose tables need something else deviates, and records
  why in its own docs so the next person does not change it back.

## What goes in a column

- **The time column of a log is called "Time"; for other records it is "Created at"** (`audit.time` /
  `table.createdAt`): a log record is the event itself, so `created_at` is when the event happened, while a business
  record's `created_at` is when the record was created. They are not the same thing, so do not borrow each other's
  copy keys.
- **Enum and yes/no values use `StatusBadge`**, not bare text: `ok` holds / `warn` pending / `danger` something is
  wrong / `muted` neutral fact; the four map to `--success` / `--warning` / `--destructive` and grey. Status colors
  **deliberately do not follow the brand preset**: dye "Active" in the brand color, and a preset with an orange primary
  gives you an orange "Active".
- **The actions column has a header too** (`table.actions`): it is pinned at the far right and is a column, not a
  blank area; without a title, scrolling to the end shows an unnamed floating strip.
- **When a row really has nothing to click (login logs can only be viewed and have no details), there is no actions
  column**, and nothing is pinned on the right.
- **Whether other columns hold buttons, links or switches is up to the page**, written in `cell`; the kernel makes no
  promise about them. What happens on click follows the site-wide container rules: a confirmation pops a popover next
  to the button, a value change opens a dialog whose initial values are fetched from the details
  (`docs/admin/forms.md`).

## Actions column

**Every cell in the actions column sets `kind`, and the choice comes down to one question: can these actions be given
a group name the user understands at a glance?**

- If not (only "More" or "Other" fits), they were never a group, and each gets its own `button` cell: a single action,
  one the page always shows, or one bound to a single permission code.
- If so, they are a group: when no item in it is particularly frequent, use `menu`, with the group name as the button
  text ("Funds ⌄"), or "⋯" when no `label` is given; when one item is used most, use `split` and mark that item
  `primary` to lift it into the left half of the button for a direct click, with the other items behind the ⋯ on the
  right. **A dangerous action (`tone: 'destructive'`) is not lifted even when marked `primary`**; the cell is drawn as
  a `menu`, because delete and disable do not belong in the handiest spot. If it really must be exposed on the row,
  give it its own `button` cell.
- **When the user has permission for none of the group, the cell is not rendered; when the `primary` item lacks
  permission, the cell falls back to `menu`**, rather than promoting the second item: promoted names vary in length,
  and the column would have to reserve space for the longest. The kernel does both; the page neither checks for empty
  nor writes branches.
- **The page decides how many cells the column holds at most**: give `maxInline` a number, and **the trailing "⋯"
  counts as a cell**. If everything fits, everything is laid out flat; if not, the last cell goes to "⋯" and the rest
  are tucked inside. Without it, everything is flat; set it only when the number of cells varies with permissions and a
  narrow table cannot fit them.
- **A menu item can be a named group of actions, and groups can nest; the page writes only the grouping, the kernel
  draws it by depth**: a group at a menu's first level is laid flat as "heading + its items", deeper groups become
  submenus, and when `maxInline` folds the cell into "⋯" everything drops one level. Two levels of headings on one
  plane do not show which belongs to which, while flat items save a click: **flatten what can be flattened, nest what
  cannot**. So the same group looks different depending on folding, which the page cannot foresee; the page does not
  pick section or submenu itself, because it does not know whether its cell will be folded. When the exact look
  matters, write the column as JSX in `cell`.
- **Button colors come only from `tone`**: none is outlined, `positive` is green text with a green border (the action
  that moves things forward, such as confirm or approve; **at most one per row**), `destructive` has a light red
  background (dangerous actions). Pages do not set `variant`.
- **Buttons draw no icon by default**: `button` and `menu` draw one only when given `icon`, `split` needs `showIcon`,
  and turning it on means remeasuring the column width; none of the kernel's system pages turns it on.
- **When an action is unavailable for this row right now but people should know it exists**: set `disabled: true` and
  give the reason in `disabledReason`. A button on the row is greyed out and shows the reason on hover; a menu item is
  greyed out with the reason on a line under its name. Availability is decided by the server and passed in; the page
  does not infer it.
- What the three forms cannot express (a switch, a status tag, a progress bar) goes directly in `cell` as your own JSX;
  the kernel makes no promise about that column, but the confirmation container rules are site-wide and still apply to
  it (see `docs/admin/forms.md`).

## What the shared table parts do and do not do

The kernel's shared table parts do only this: the list page's card shell, pinned columns, column width dragging, row
selection, server-side pagination, the loading skeleton and empty state, the row actions column, and tree rows.

- **No refresh button**: refetching on window focus and invalidating after create, update and delete already cover the
  ways data goes stale, and a button taking space on every page buys nothing.
- **No column visibility, density, full screen, column reordering or inline editing** (remembered column widths are
  the only remembered column state): density is set globally by the appearance preset's `--spacing`, with no second
  entry point. A derived repo that needs row expansion passes table options through instead of forking the component.
- **No header-click sorting**: the shared table registers no sorting feature; a list sorts through its endpoint's sort
  parameter, chosen in the filter bar.

## Standard list page structure

**A standard list page is one card**, its four sections separated by dividers and all rendered by the shared table
parts: the filter header (filter conditions + Search/Reset + export) → the bulk action bar → the table body → the
footer (selected count on the left; total, page size and pager on the right). The filter header and the bulk bar are
**slots**: with no content, that section is not rendered (a page without a filter bar starts with the table; with no
bulk actions there is no selection column). The shared parts know nothing about filtering; `FilterBar` stays a
separate component, just placed into the slot. The shell belongs to the shared parts because the selected count,
button disabling and clearing on page change all touch the same row selection state; having the page assemble it would
mean exposing the table instance upwards and rewiring it on every page. Filtering stays out for the opposite reason:
it is not coupled to that state, and folding it in would give the shared parts parameters they do not understand. The
selection column is inserted by the shared parts and pinned leftmost; pages do not list it.

**A failed fetch replaces only the table body**: the body shows "Failed to load" and a retry, the filter header, bulk
bar and footer stay, and the footer keeps the last total it received. `DataTable` takes the query result (`query`, with
anything else to wait for in `requires`) and draws the skeleton, failure and data by one set of rules; reshaping the
data (taking `items`, building a tree) happens in `useQuery`'s `select`. List pages are wired by `useList`; a page that
needs the current page's rows or whether a fetch is in progress uses `list.rows` / `list.fetching`
(`docs/admin/errors.md` "A failed fetch affects only its block").

## Bulk actions

- **The bulk action bar is always shown, with its buttons greyed out when nothing is selected**: plugging it in and out
  by selection would make the table jump, and ticking rows is a continuous action.
- **The selected count is written only in the footer**, where this kind of table usually puts it; on the bulk bar the
  number goes into the button label ("Delete selected (3)"), which puts "how many" on the control being pressed.
- **Selection crosses neither pages nor result sets**: changing page, page size or result set clears it, and only rows
  you can see count as selected. A selection on another page or in another result set is an invisible selection: the
  confirmation can report how many but not which. Acting on every row that matches a filter is a different feature, a
  bulk operation by filter condition with its own confirmation, not ticking rows while paging.
- **The kernel's system domain pages enable no bulk actions**: those actions (disable, sign out, reset) each need a
  confirmation, and some need a fresh identity check on the spot, so batching them could only work by removing that
  gate. Whether a business page enables them is decided by the same test.

## Pagination

**The page size options are the kernel's `PAGE_SIZES` (`web/src/lib/table.ts`), defaulting to the first**, and each
page can customise them with `pageSizes`; **the largest option must equal the server-side cap of the endpoint's
`limit`** (the proto's `lte`). The two are a pair, and a mismatch breaks silently: the UI offers the option and
parameter validation rejects the request with a 400. Change both sides together.

## Filter bar

- **Filters sit in one row and take no separate label row**: dropdowns and date ranges put the label inside the control
  ("Status: All"), inputs carry no label and use it as the placeholder; changed conditions take effect when "Search" is
  clicked, and clicking "Search" or "Reset" with unchanged conditions still refetches the data (handled by `useList`).
- **Filter controls always use the component's default size and set no height** (buttons given to `actions` too): the
  default inputs, dropdowns and buttons are already the same height, and one sm control or a hand-written height makes
  them uneven. Width follows the content; controls set no width.
- **The "All" option of a dropdown filter is added first by `FilterBar`**, with an empty value meaning no filter; the
  page does not write it. A field that really must not have "All" sets `hideAllOption`.
