<!-- Shared file: carried verbatim; never localized outside its source repository.
     Checked by scripts/check-cross-repo-sync.sh; see docs/upgrading-protocol.md. -->

# Page structure: title, layout, sub-views, sidebar menu and controls

Read this before creating or reshaping a page: where its title comes from, what goes in the page header, how sub-views
get routes, where it hangs in the menu and which controls it uses. This is a keel-admin frontend rule page; the guides
in `docs/guides/` have no counterpart. Tables and list pages are in `docs/admin/tables.md`; forms, containers and URL
state in `docs/admin/forms.md`; the shell's assembly seams (header, brand, time zones, languages) in
`docs/admin/shell.md`.

## Title

**The route's `titleKey` is the page name's only source**: the header, the sidebar menu, the tab bar and the browser
tab title all read it, so there is no second copy to drift. The page name shows once, in the header (the content
area's page header carries no title). On a page with sub-views, the header and the sidebar take the page level, the
tab bar takes the sub-view level, and the browser tab title joins them from the inside out
(`sub-view name · page name · brand name`; a page without `titleKey` shows only the brand name). Do not write a second
title in the page, and do not declare one again through the router's `head`/`HeadContent`: that is a second copy that
drifts.

## Page layout

The content area is full width with `p-4 md:p-6`. The page header (`PageHeader`) = an optional description + the
primary action on the right (a standalone function **unrelated to the filters**, such as Create; Export follows the
filters and goes in the filter bar's `actions`); with neither, write no page header. A page with a tab bar has no page
header: its primary action sits on the right of the tab bar's row. A list page = the page header + one table card
(the four-part structure is in `docs/admin/tables.md`). **Data is drawn inside the data block, the frame outside it**:
a failed fetch makes only that data block show the failure, while the page header and the filter bar render as usual
(`docs/admin/errors.md` "A failed fetch affects only its block").

**URL parameters carry only filters**, declared by the list's list definition (`defineList` in `lib/list.ts`, written
in the domain's API file, spread into the route as `...xxxList.route`), so a refresh restores the filters, a link can
be shared, and Back returns to the previous query's criteria. **The query follows page state; the URL only mirrors
it**: only Search and Reset write the URL, and a URL change the query did not cause (first open, Back/Forward, the
sidebar link to the current page) is restored as a first open (`docs/adr/admin-0005-list-definitions.md` item 7).
URL reading and writing keep the router's defaults with no extra configuration, so ID-like values carry quotes (the
trade-off is in `docs/adr/admin-0005-list-definitions.md`).

The page number, page size and whether a dialog / popover / drawer is open stay out of the URL: the filter bar's
values and reset, pagination (back to page 1 whenever the result set changes) and the `loader` prefetching only page 1
are all derived from the definition by `useList(xxxList, Route)`, and container open state is the page's local state.
Edit forms fetch their initial values with `useDetailForm`, which rules out pre-filling a form from a cache by
construction (`docs/admin/forms.md`).

## Sub-views

**Each sub-view that must be directly navigable (such as a tab) gets its own route, not a URL parameter**: the sidebar
finds the current item by URL path, so a sub-item cannot point at `?tab=`; a sub-view's own permission code would have
nowhere to attach; and the list cache keys on the whole query string, so every switch would send a wasted request.
Each navigable sub-view therefore adds a route file, and switching does not carry the other sub-view's filters. The page
level (the directory's `route.tsx`) draws the tab bar and the primary action; each sub-view is a route that declares its
own list definition; the default sub-view is the directory's `index.tsx`, at the same URL as the page. A sub-view's own
permission codes go in its `staticData`, stacked on the page level, and the menu and the 403 gate check level by level.
The tab list is written in the page, filtered to the reachable ones with `subViewTabs` from `lib/sub-views.ts`, and the
tabs render as real links the shadcn way (`render={<Link />}` + `nativeButton={false}`). A sub-view that needs no
navigation is the page's local state.

## Sidebar menu

**The sidebar menu is organised only in `web/src/nav.ts`**: groups (`labelKey` optional: without it the items are
flat; `collapsible` optional, off by default: only a labelled group can collapse, the label being the handle) → items:
a page (`path` + `icon`) or a grouping parent (`labelKey` + `icon` + `children`, whose children are pages; nesting
stops there). A derived repo changes this one file and leaves the sidebar component alone.

- **A parent only groups; every child must be a standalone page**: the parent itself is not a page and its whole row
  only expands or collapses it; a child does not point at a sub-view, since sub-views are reached from the page's tab
  bar. Menu items always show the page name; the icon is optional. Not "a parent may also be a page": every new parent
  would first need a choice of kind, and a row whose name enters a page while its arrow expands does two jobs. Not
  "children may point at sub-views": the menu and the tab bar would become two copies of the same entries. So a page
  with a tab bar takes one menu item; to list its parts in the menu separately, split them into pages.
- A parent has no permission of its own and appears when at least one child is reachable. Entering any page of the
  group expands it automatically; it only expands, never collapses, and the open state is not remembered. With the
  sidebar collapsed to icons, clicking a parent's icon pops its children out to the right without expanding the
  sidebar (collapsing was the user's choice); not on hover, since touch screens have no hover and it opens by
  accident.
- **The list holds no permission codes**: menu visibility and the 403 gate both read `perm` / `root` level by level
  from the route's `staticData`, and `path` takes its type from the generated route tree, so a wrong path does not
  compile.
- **The kernel owns only two groups**, overwritten as a whole with the sidebar on kernel upgrades: "My Favorites" is
  always first (driven by user settings; the whole group is not rendered when there are no favorites) and "System" is
  always last; the nav.ts list sits between them, in declaration order.
- **The landing page is in nav.ts too**: `/` is a business page, so its menu entry has no reason to
  belong to the kernel.
- **Favorites are per page; the star sits after the page name in the header, and the header is the only place to add
  or remove one**: the sidebar has no star and "My Favorites" has no remove button. Favoriting while on a sub-view
  favorites its page, and pages not in the menu can be favorited too. The landing page `/` gets no star: the logo and
  the error pages' "Back to home" all point to it, so the logo is its permanent entry.
- **System sits on the same level as the business groups**: it has a label like them; only its position is fixed. The
  kernel's two groups do not collapse, and a business group's `collapsible` does not extend to them.

**The sidebar's rules are computed in the kernel's `web/src/lib/nav-model.ts`** (group order, permission filtering,
skipping a group that ends up empty, the current item, which group starts expanded, each item's icon and title key);
the sidebar component only draws the result. A business need for another sidebar rule is a kernel change: report it
upstream (`AGENTS.md`, "Ask before acting", point 6).

## Controls and sizes

**Build the UI from shadcn/ui's own components or compositions of them; do not write raw `<input>` `<textarea>`
`<select>` `<button>`.** Composition means placing components together and passing only the props their docs list. A
custom implementation means writing a control the library lacks (handling interaction, focus or keyboard yourself), or
using `className` to draw a look the components lack (a hand-drawn selection ring, a dashed frame, a separate status
palette). **When composition is not enough and a new custom implementation is needed, stop and ask first** (item 7 of
"Ask before acting" in `AGENTS.md`): explain why composition is not enough, wait for explicit consent, then write it
with a comment next to it saying why. Text inputs, multi-line text, radios, checkboxes, switches, selects, buttons
inside inputs (`InputGroup`) and link-styled buttons all come from `components/ui/`; add a missing one with the CLI
first (`pnpm dlx shadcn@latest add <component>`), after reading the official shadcn skill (installation in
`docs/admin/shell.md` "shadcn style and skill"); a component that brings a new third-party dependency still goes
through "Ask before acting". shadcn components come with theme tokens, focus rings, `aria-invalid` styling and keyboard
interaction; a raw control needs each of these added by hand, and anything missed makes it unlike the rest of the site
(the browser's native date input looks and behaves differently in each browser). The only approved raw exceptions are
two: the hidden file input (choosing an avatar image) and the table's column-width handle.

- **Dropdown and select popovers take no width**: global CSS already sizes them to their content, at least as wide as
  the trigger and capped by the available space (`styles/index.css`); writing `className="w-auto …"` at the call site
  is redundant.
- **Pick dates and times only with the kernel's controls in `components/date-picker.tsx`**: `DatePicker` (one date),
  `DateRangePicker` (a date range), `DateTimePicker` (one instant, to the second), `DateTimeRangePicker` (a time range,
  to the second), plus the date-less `TimePicker` (a time of day, such as the daily send time) and `TimeRangePicker`
  (a span within a day). shadcn only gives a Popover + Calendar composition example, no ready component; composing it
  yourself from the example misses which time zone "today" is computed in, labelling the time zone, and month and
  weekday names following the UI language. The value they hand the form is the value the API wants (see
  `docs/admin/api-endpoint.md` "Passing time: meaning first"): a date is `YYYY-MM-DD`, an instant is a UTC RFC3339
  string, a time of day is `HH:mm` (`HH:mm:ss` with seconds), and a range is `{ from, to }`; a time range's end is
  already pushed one second later to make it half-open, so the page does no conversion. **`zone="business"` reads,
  writes and labels in the business time zone from system settings and does not follow the header switch**; without
  it, instant controls use the header's display time zone, date controls are calendar dates (no time zone label) and
  time-of-day controls show no label. Put a fixed width directly in `className`.
- **One-time code inputs always use the kernel's `OtpInput`** (`OtpField` already contains it): browsers and
  1Password recognise a code field by its `autocomplete="one-time-code"`, and it accepts only 6 digits; a hand-written
  Input easily misses that.
- **Password fields always use the kernel's `PasswordInput`**: it ends with an eye button that toggles plain text,
  written as shadcn's `InputGroup`; give `autoComplete` for the context (`current-password` / `new-password`). A
  hand-written `type="password"` Input lacks the button and differs from other pages.
- **Hover hints always use shadcn's `Tooltip`, not an element's `title` attribute**: the browser's native hint takes a
  second or two to appear, looks different in each browser and ignores the theme, unlike every other popover on the
  site. `TooltipProvider` is already mounted at the root in `main.tsx`, so pages use `Tooltip` / `TooltipTrigger` /
  `TooltipContent` directly; when the trigger should be the element itself, pass it through `render` instead of adding
  a wrapper. Icon buttons still get an `aria-label`, with the hover hint added through `Tooltip`; a component's own
  `title` prop (the title of a dialog, drawer or card) is not a hover hint and is not covered here.
- **Where something aligns with a component, write no extra size number**: for a same-width placeholder render the
  same component with `invisible`; for nested indentation use `grid-cols-[auto_1fr]` so the collapse button sizes the
  first column; other spacing uses the `--spacing` steps. A hard-coded number that tracks a component's step falls one
  step out of line as soon as the component changes size; one that truly cannot be avoided goes into a single constant
  with a note saying which step it aligns with.

## Where frontend code lands

**In the frontend, the home of a concept is a hook or a component** (the frontend shape of "One concept, one home" in
`AGENTS.md`): reading a value, its intermediate states and its fallback go into one hook and pages take only the
result, so the fallback is written once and no page can skip it. When the same interaction (such as search + check +
create) appears a second time, extract it into a component, so the two copies cannot drift apart. Hooks and components
shared by several business pages go in `routes/_app/-shared/` (under `web/src/`).

## User-facing text

**`docs/practices/user-text.md` also covers all frontend text in an admin**: hints, help text, confirmation bodies
and empty states, because users read them just like error messages. A success message states the consequence on the
user's side, neither "Operation succeeded" nor the implementation, since that is the only part the user can act on.

## Displaying time

**The frontend always displays time with `DateTimeText`; never call `formatDateTime` yourself**, so the UTC → display
time zone conversion happens in exactly one place. It **suffixes the time zone** (`2026-07-25 22:01:59 (UTC)`): the
same digits are different instants in different time zones, and the header dropdown can switch zones at any moment.
Repeating the suffix on every row of a column is redundant, but without it, a screenshot sent to a colleague, or your
own view the next day, cannot tell whose midnight it is. By default it follows the header's display time zone; an
instant that only makes sense in the business time zone (a send time scheduled in the business time zone, a business
day's cutoff) gets `zone="business"`, which pins it to the business time zone from system settings regardless of the
header switch. **Switching the display time zone does not refetch data**: the server always returns UTC, so the change
is only a re-render. How time is passed (instant / business day / calendar date) is in `docs/admin/api-endpoint.md`
"Passing time: meaning first".
