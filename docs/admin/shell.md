<!-- Shared file: carried verbatim; never localized outside its source repository.
     Checked by scripts/check-cross-repo-sync.sh; see docs/upgrading-protocol.md. -->

# Frontend shell: brand, time zones, languages and appearance

Read this before changing the brand, the header, the time zones, the UI languages or the appearance of a derived repo,
or before placing a frontend file or adding a shadcn component. This is a keel-admin frontend rule page; the guides in
`docs/guides/` have no counterpart. The shell parts are assembly seams, changed without touching the kernel, but they
take effect at two different times, which must not be mixed up: **deploy time** (`sys.*` in `configs/app.yaml`,
effective after a restart) and **build time** (files under `web/src/`, effective after rebuilding the frontend). The
page's own structure (title, sidebar menu, controls) is in `docs/admin/pages.md`.

## Brand (deploy time and build time)

- **`sys.brand.title` is required**; startup fails when it is empty. The server injects it into `index.html`
  (`<title>` and `window.__BRAND__`), and the frontend consumes it from one source, `web/src/lib/brand.ts`.
- **The logo has exactly one outlet: `/v1/sys/brand/logo`.** All three states (embedded default / disk path /
  `http(s)://` external link) share it: the embedded and disk states serve the content with a `Content-Type` matching
  the real file, the external state redirects with 302. The in-page logo and the favicon reference the same address,
  so the frontend need not know the logo's format or location, and changing the brand in the disk state needs no
  rebuild.
- **The embedded default logo is a build-time assembly seam**: `logo.png` or `logo.svg` under `web/public/`;
  **replace the file, not the code**. `web/logo.go` embeds it into the binary unconditionally, regardless of the
  `embed_ui` build tag, so development and embedded deployments behave the same; with no `logo.*` at all under
  `public/` the build fails.
- Both brand paths are under `/v1/`: the server's raw routes all go under `/v1/`, and the root belongs to the SPA
  (`docs/admin/raw-http.md`).

## Business time zone (deploy time)

`sys.business_time_zone` (an IANA name) is the time zone a business day is counted in, and it is **required**. The
server converts business days to instants with it and, after login, sends it to the frontend through
`/v1/sys/meta/system-info` to label pinned filters. It is also the last fallback for the display time zone, so
deploying for another region changes only this one value, with no frontend rebuild. The three meanings of time are in
`GLOSSARY.md` and `docs/adr/admin-0003-three-meanings-of-time-and-how-to-pass-them.md`.

## Shell switches: four tri-state axes (build time, `web/src/shell.ts`)

The header's four axes, time zone `timeZone`, language `locale`, color mode `colorMode` and appearance preset
`themePreset`, share one tri-state:

| State | Control | Value | Changeable |
|---|---|---|---|
| `{ state: 'auto' }` | rendered | user choice → `default` → derived from the browser → fallback | yes |
| `{ state: 'hidden' }` | not rendered | same as auto | yes (the derived repo builds its own control) |
| `{ state: 'fixed', value }` | not rendered | `value` | no |

- `auto` and `hidden` may both carry an optional `default`: the starting point **while nobody has chosen yet**, ranked
  **before** the browser-derived value. Example: `locale: { state: 'auto', default: 'zh-CN' }` means "new users start in
  Chinese but can switch".
- `fixed` is a hard pin: the corresponding setter has no effect, and not even a derived repo's own component can change
  it; with the language fixed, the `Accept-Language` request header is fixed with it. An invalid `default` or `fixed`
  value (an unregistered preset or default time zone; a fixed time zone only has to be accepted by `Intl`) always
  falls back with a `console.warn`.
- An object rather than a flat `'auto' | 'hidden' | T`: a time zone id is a `string` alias, so the flat union collapses
  to `string`, and a misspelled `'auto'` silently becomes "fixed to a time zone that does not exist".
- In the header, time zone and language share one dropdown, and the button is not rendered when neither part is. The
  appearance preset is a dropdown and the color mode a button that flips to the other side on each click; each listens
  to its own axis's switch. The color mode has only light and dark; before anyone chooses, it follows the system.
- The same file holds a few layout constants (the sidebar's form and how it collapses, whether the header sticks,
  whether the content area is full width). **The layout itself cannot be switched**: left sidebar + top bar + content
  area.
- The header's **permission preview and user menu are not among the four axes and deliberately have no switch**: the
  former always judges as root and is the kernel's debugging entry; the latter is the session exit, and hiding it would
  leave people unable to log out.

The four axes' runtime choices are **preferences**: stored in localStorage, bound to the device, and effective on the
login page too (they must render correctly before the profile arrives, so they are not in the account-bound user
settings). **Read and write them only through `web/src/lib/preference.ts`**: in private browsing localStorage may throw
on the first read, and that safeguard is written only there.

## The two custom header slots (build time, `web/src/header.tsx`)

After the collapse button come the kernel's page name and favorite star; the whole stretch from there to the kernel
buttons belongs to the derived repo: `HeaderStart` (left slot, right after the page name) and `HeaderEnd` (right slot,
before the kernel buttons); both are empty in the baseline. A search box goes in the left slot, notifications and
shortcuts in the right slot; controls use shadcn's default sizes (icon buttons `size="icon"`) so they match the
kernel's row in height without effort.

## Time zone registry (build time, `web/src/timezones.ts`)

The kernel builds in only `UTC`; whatever else the header dropdown offers is registered here (the id must be an IANA
time zone supported by the browser's `Intl`; the display name's entry goes in the business copy). Before anyone
chooses, the order is: `timeZone.default` in `shell.ts` → the browser's location (only if registered) → the business
time zone (the kernel adds an entry automatically if it is not in the dropdown) → UTC. The backend always returns UTC
RFC3339, and the frontend always displays it with `DateTimeText` (`docs/admin/pages.md` "Displaying time").

## Login page layout (build time, `web/src/login-layout.tsx`)

The login form and flow (captcha, one-time code, forced authenticator binding) live in the kernel's `LoginForm`; the
layout file decides everything around the form: background, columns, the large image, the brand, where the language
switch sits, and the title and description above the form (their copy is `login.*` in the business copy). To change
only the large image, replace `web/public/login-cover.svg`, not the code; narrow screens do not render the image
column.

## Appearance presets (build time, `web/src/styles/`)

Each preset is one CSS file in `web/src/styles/presets/`, `@import`ed in `web/src/styles/index.css` and registered with
one line in `themePresets` in `shell.ts`. All three places are assembly seams, and **a derived repo may replace them
entirely**: delete the template's files, `@import`s and registration lines and register only its own; the kernel
recognises no specific preset id.

- **The first registered preset is the default, the only one on the bare `:root`**, and so the target of every
  fallback.
- **A non-default preset must provide both the light and the dark set in full**: with only one, the other falls back
  to the default preset and two palettes mix on one page.
- Changing `--radius` and `--spacing` scales the whole site's corner radius and density; the font is the single
  variable `--app-font-sans`. Switch fonts with a font package bundled into the frontend, not a font loaded over the
  network (in an intranet deployment it silently falls back).
- **Changing the brand color touches only the three primary groups** (`primary` / `ring` / `sidebar-primary`, each in
  light and dark): the nearly colorless variables for background, cards, borders and text decide what material the
  admin is made of and need no change.
- Each preset also provides the two status colors `--success` and `--warning` (used by `StatusBadge`). They are
  **semantic colors, not brand colors**: green = holds, yellow = to do, red = something went wrong; in a preset whose
  primary color is orange or yellow, `--warning` must move its hue away.

How "appearance preset", "color mode" and "style" differ is in `GLOSSARY.md` "Appearance". The shadcn style is fixed;
see "shadcn style and skill" below.

## Adding a UI language

Do these steps in order (they are assembly seam additions and need no question):

1. **Register**: append an entry to `web/src/languages.ts`: the language code, the dropdown display name (written in
   that language itself, not in the i18n resources), the business copy, an optional kernel translation, and an optional
   calendar locale (`react-day-picker/locale`; English calendar by default).
2. **Business copy**: write `web/src/locales/app/<code>.ts` with every entry you need translated; to protect yourself,
   add the `Translations<typeof appEnUS>` type constraint (the kernel does not enforce it).
3. **Optional: kernel translation**: write `web/src/locales/sys/<code>.ts`, possibly only partly; missing keys fall back
   to English (the fallback language, see `GLOSSARY.md`); register it in the `sys` field of the step 1 entry.
4. **Backend**: translate every business entry in `internal/i18n/app.<code>.toml`; optionally translate
   `internal/i18n/sys.<code>.toml` (partly is fine too). A file placed in `internal/i18n/` is loaded automatically, no
   code change needed.
5. **Widths**: the actions column of every business page gets the new language's measured value through
   `widthFor({ default, '<code>': N }, locale)` (`docs/admin/tables.md`); other column widths and widths in styles (such
   as the sidebar's, a `:root:lang(<language>)` override in `web/src/styles/`) as needed. Kernel pages keep the English
   widths for this language. Measure both numbers on real pages instead of scaling by ratio (`docs/admin/tables.md`,
   "Pinned columns and column
widths").

The kernel overwrites only the Chinese and English copy (`sys.en`, `sys.zh-CN`); other languages a derived repo adds
are neither checked nor overwritten by the kernel. The reasoning for ownership and fallback is in
`docs/adr/admin-0010-languages-and-copy-ownership.md`.

## Frontend zones

The frontend is split into the same zones as the backend, placed by shadcn / Vite / TanStack conventions, so that a
kernel upgrade can overwrite whole directories without touching the derived repo's code:

| Zone | Location (under `web/src/`) | How it is upgraded |
|---|---|---|
| Kernel | `components/` (except `ui/`), `hooks/`, `lib/`, `locales/sys/en-US.ts`, `locales/sys/zh-CN.ts`, from the route tree `routes/__root.tsx` `routes/login.tsx` `routes/_app.tsx` `routes/403.tsx`, the whole `routes/_app/sys/` directory | Whole directory / whole file overwritten, read-only for the derived repo (other language files the derived repo adds under `locales/sys/` are outside the overwrite) |
| Generated artifacts | `generated/`, `routeTree.gen.ts` | Never edited or copied by hand; after changing the sources (proto, `routes/`) regenerate with `make all` (`make routes` when only routes changed; `make perms` after changing permission codes) |
| Third-party components | `components/ui/` | The derived repo may add components with the shadcn CLI, never edits them by hand; on upgrade, the upstream files are overwritten one by one and the derived repo's additions are kept |
| Assembly seams | `main.tsx`, `shell.ts`, `nav.ts`, `languages.ts`, `timezones.ts`, `header.tsx`, `login-layout.tsx`, `styles/` | Upstream provides the baseline, the derived repo appends on top (what each does is in this page) |
| Business area | everything under `routes/_app/` except `sys/`, `locales/app/` | Owned by the derived repo, never touched upstream |

- **Page-private code sits next to its route with TanStack file routing's `-` prefix**
  (`routes/_app/orders/-columns.ts`), not in kernel directories, because kernel directories are overwritten on upgrade.
- **Code shared by several business pages goes in `routes/_app/-shared/`**: the `-` prefix keeps it out of the route
  tree, and it sits in the business area, which upgrades never touch.
- **The template has no `features/`**: how the business area is split into directories is up to the derived repo.
- **Copy is split the same way**: the backend's kernel files `internal/i18n/sys.*.toml` and business files
  `app.*.toml`, and on the frontend `locales/sys/` and `locales/app/`. The kernel overwrites only the Chinese and
  English files; other languages a derived repo adds are neither checked nor overwritten.

## shadcn style and skill

- **The shadcn style is fixed to Nova + Base UI + lucide** (`web/components.json`): changing the style reinstalls every
  component's source, it is not a config switch.
- **`components/ui/` and `hooks/use-mobile.ts` are CLI output**: read-only, never edited by hand, and excluded from
  Biome, so the next CLI run can overwrite them cleanly.
- **Before adding a component, changing `components.json` or writing a preset, read the official shadcn skill**, since
  it holds the CLI's current rules that this page does not repeat. It is not in the repository: if it is not installed,
  run `pnpm dlx skills add shadcn/ui -s shadcn -y` at the repository root, which installs it into
  `.agents/skills/shadcn/` (Claude Code discovers it through the `.claude/skills/` link), then read its `SKILL.md`. All
  three outputs are in `.gitignore`.
