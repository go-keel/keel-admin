// type-only: lib/locale.ts, lib/timezone.ts and lib/theme.ts import shell in turn, so a value import would create a
// runtime import cycle.

import type { SupportedLocale } from '@/lib/locale';
import type { ColorMode } from '@/lib/theme';
import type { TimeZoneId } from '@/lib/timezone';

/**
 * The three states of a shell control:
 * - auto: render the switch; the value follows the manual choice, then default, then what the browser implies
 *   (the time zone falls back to the server's business time zone when the browser's is not recognised);
 * - hidden: do not render the switch; value resolution works as usual, so a derived repo can drive it with its own
 *   component;
 * - fixed: lock to value, ignoring the manual choice, default and the browser; the switch is not rendered.
 *
 * `default` is the starting point **before anyone has chosen**, not a fallback: it comes before the browser, otherwise
 * a derived repo's default would be overridden by the browser language/time zone, as if never written. An invalid
 * default (an unregistered time zone or preset) falls back by its own rules with a warning, like an invalid fixed.
 *
 * An object rather than a flat 'auto' | 'hidden' | T: TimeZoneId is only an alias of string, so a flat union would
 * collapse to string as a whole, and a misspelt 'auto' would raise no error but silently become "fixed to a nonexistent
 * time zone".
 */
export type ShellControl<T> =
  | { state: 'auto' | 'hidden'; default?: T }
  | { state: 'fixed'; value: T };

/** Registry of appearance presets: adding a preset = adding a styles/presets/*.css + an @import in index.css + one line
here. All three are inside assembly seams, none touches the kernel.
 */
export type ThemePresetId = string;

export type ThemePresetDef = {
  /** The value written to <html data-theme-preset>, matching the selector in the CSS file */
  id: ThemePresetId;
  /** Name shown in the dropdown. Preset names are proper names, not i18n resources */
  label: string;
};

/**
 * The single source of switches for a derived repo to customise the shell. Only "switches decided by the deployment",
 * no logic.
 *
 * Build-time constants: changing them needs a rebuild of the SPA (make build-web), not a hot change by ops. A
 * deliberate trade-off: a purely frontend look is not worth a server-side config delivery path.
 *
 * **The permission preview in the header bar is not here**, and deliberately has no switch: it always checks root and
 * is the kernel's own debugging entry, not appearance. A "hide" switch would be an option "make root unable to see a
 * power it has", which solves no deployment problem.
 */
type ShellConfig = {
  /**
   * Header bar time zone dropdown. fixed is hard: setTimeZone stops working too, and a derived repo's own component
   * cannot change it. value need not be in the registry (the registry governs what the dropdown offers, and fixed has
   * no dropdown), but Intl must accept it, otherwise the first read falls back to UTC with a warning.
   */
  timeZone: ShellControl<TimeZoneId>;

  /** Language dropdown in the header bar and login page. fixed locks i18next's initial language and Accept-Language. */
  locale: ShellControl<SupportedLocale>;

  /** Color mode. When fixed, setColorMode stops working and <html class="dark"> follows the fixed value. */
  colorMode: ShellControl<ColorMode>;

  /** Appearance preset. When fixed, setThemePreset stops working. */
  themePreset: ShellControl<ThemePresetId>;

  /**
   * Layout constants. The structure (left sidebar + top bar + content area) cannot be switched; only shadcn Sidebar's
   * two shape options and two content area switches are here.
   */
  layout: {
    /** shadcn Sidebar's variant */
    sidebarVariant: 'sidebar' | 'floating' | 'inset';
    /** shadcn Sidebar's collapsible */
    sidebarCollapsible: 'icon' | 'offcanvas' | 'none';
    /** Whether the header bar sticks to the top */
    stickyHeader: boolean;
    /** Whether the content area is full width; false centres it at a limited width */
    contentFullWidth: boolean;
  };
};

export const themePresets: readonly ThemePresetDef[] = [
  { id: 'neutral', label: 'Neutral' },
  { id: 'compact-slate', label: 'Compact Slate' },
  // The three below come from the tweakcn community; the source is noted at the top of each CSS file.
  { id: 'light-green', label: 'Light Green' },
  { id: 'portfolio-theme', label: 'Portfolio Theme' },
  { id: 'sage-green', label: 'Sage Green' },
];

export const shell: ShellConfig = {
  timeZone: { state: 'auto' },
  locale: { state: 'auto' },
  colorMode: { state: 'auto' },
  themePreset: { state: 'auto' },
  layout: {
    sidebarVariant: 'sidebar',
    sidebarCollapsible: 'icon',
    stickyHeader: true,
    contentFullWidth: true,
  },
};
