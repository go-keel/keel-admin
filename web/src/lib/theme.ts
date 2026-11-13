import { shell, type ThemePresetId, themePresets } from '@/shell';
import { readPreference, writePreference } from './preference';

/**
 * Two axes switchable at runtime: color mode and appearance preset. Both live in localStorage rather than user
 * settings: the login page must render correctly before the profile arrives, when account-bound settings are not yet
 * available.
 *
 * They land on <html class="dark"> (Tailwind's dark variant) and
 * <html data-theme-preset="…"> (the selector of styles/presets/*.css).
 */
export type ColorMode = 'light' | 'dark' | 'system';

const MODE_KEY = 'keel_color_mode';
const PRESET_KEY = 'keel_theme_preset';

const DEFAULT_PRESET: ThemePresetId = themePresets[0]?.id ?? 'neutral';

/**
 * Stored value → color mode. The fixed mode beats everything; when nobody has chosen, the derived repo's default is
 * used, and without one it falls to system (follow the OS).
 */
export const resolveColorMode = (stored: string | null): ColorMode => {
  if (shell.colorMode.state === 'fixed') return shell.colorMode.value;
  if (stored === 'light' || stored === 'dark' || stored === 'system') {
    return stored;
  }
  return shell.colorMode.default ?? 'system';
};

/** Color mode + system preference → whether it is dark right now. */
export const isDarkMode = (mode: ColorMode, prefersDark: boolean): boolean =>
  mode === 'system' ? prefersDark : mode === 'dark';

/**
 * Stored value → preset id. The fixed mode wins; when nobody has chosen, the derived repo's default is used. An
 * unregistered id (whether from storage, default or the fixed value) falls back to the first registered entry.
 *
 * **An unregistered fixed value or default warns** (like time zones): the preset id is the selector written into
 * `<html data-theme-preset>`, so one misspelt letter means no CSS matches and the whole appearance fails silently; both
 * values come from the derived repo's build-time config, and without a warning nobody would find out.
 * A stored value does not warn: it is what the user last picked, and falling back to the default after a preset is
 * removed is a normal generation change.
 */
export const resolveThemePreset = (stored: string | null): ThemePresetId => {
  const registered = (id: string | undefined): id is ThemePresetId =>
    themePresets.some((p) => p.id === id);
  if (shell.themePreset.state === 'fixed') {
    if (registered(shell.themePreset.value)) return shell.themePreset.value;
    console.warn(
      `[shell] fixed appearance preset ${shell.themePreset.value} is not registered; falling back to ${DEFAULT_PRESET}`,
    );
    return DEFAULT_PRESET;
  }
  if (registered(stored ?? undefined)) return stored as ThemePresetId;
  if (registered(shell.themePreset.default)) return shell.themePreset.default;
  if (shell.themePreset.default !== undefined) {
    console.warn(
      `[shell] default appearance preset ${shell.themePreset.default} is not registered; falling back to ${DEFAULT_PRESET}`,
    );
  }
  return DEFAULT_PRESET;
};

const listeners = new Set<() => void>();
const emit = () => {
  for (const listener of listeners) listener();
};

let colorMode = resolveColorMode(readPreference(MODE_KEY));
let themePreset = resolveThemePreset(readPreference(PRESET_KEY));

const prefersDarkQuery = (): MediaQueryList | undefined =>
  typeof window.matchMedia === 'function'
    ? window.matchMedia('(prefers-color-scheme: dark)')
    : undefined;

const apply = () => {
  const root = document.documentElement;
  root.classList.toggle(
    'dark',
    isDarkMode(colorMode, prefersDarkQuery()?.matches ?? false),
  );
  root.dataset.themePreset = themePreset;
  // Browser-drawn parts such as form controls and scrollbars follow it; otherwise dark mode shows white scrollbars.
  root.style.colorScheme = root.classList.contains('dark') ? 'dark' : 'light';
};

export const getColorMode = (): ColorMode => colorMode;

/**
 * Whether it is actually dark right now. The color-mode check in the top bar's appearance dropdown needs this rather
 * than colorMode: when nobody has chosen, colorMode is 'system', and the check must land on the side the OS resolves.
 */
export const getIsDark = (): boolean =>
  isDarkMode(colorMode, prefersDarkQuery()?.matches ?? false);
export const getThemePreset = (): ThemePresetId => themePreset;

export const setColorMode = (mode: ColorMode): void => {
  if (shell.colorMode.state === 'fixed') return;
  colorMode = mode;
  writePreference(MODE_KEY, mode);
  apply();
  emit();
};

export const setThemePreset = (preset: ThemePresetId): void => {
  if (shell.themePreset.state === 'fixed') return;
  if (!themePresets.some((p) => p.id === preset)) return;
  themePreset = preset;
  writePreference(PRESET_KEY, preset);
  apply();
  emit();
};

export const subscribeTheme = (listener: () => void): (() => void) => {
  listeners.add(listener);
  return () => {
    listeners.delete(listener);
  };
};

/** Called once at the entry point: writes the initial class/attribute and follows OS color-scheme changes. */
export const initTheme = (): void => {
  apply();
  prefersDarkQuery()?.addEventListener('change', () => {
    if (colorMode !== 'system') return;
    apply();
    emit();
  });
};
