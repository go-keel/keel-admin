/**
 * The two header bar slots left to the derived repo (assembly seam). From left to right the header bar is: sidebar
 * toggle → separator, page name and favorites star (kernel) → `HeaderStart` → blank (fills the remaining width) →
 * `HeaderEnd` → the kernel's buttons (time zone, language, appearance preset, color mode, permission preview, avatar).
 * A derived repo puts its own things here (search on the left, notifications and shortcuts on the right) without
 * touching the kernel's header bar. Both slots are empty in the upstream baseline.
 *
 * Controls use the default size of shadcn components (icon buttons `size="icon"`), matching the kernel's row in height.
 * The left slot must be set apart from the page name with a vertical separator, as the kernel does:
 * `<Separator orientation="vertical" className="h-4 data-vertical:self-center" />`
 * (a vertical separator carries `data-vertical:self-stretch` and would fill the header bar; the override needs the same
 * variant prefix).
 *
 * A separate file rather than shell.ts: kernel modules such as lib/locale import shell.ts, so importing components
 * there would drag business code into a startup import cycle.
 */
export function HeaderStart() {
  return null;
}

export function HeaderEnd() {
  return null;
}
