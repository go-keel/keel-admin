import { useSyncExternalStore } from 'react';
import {
  type ColorMode,
  getColorMode,
  getIsDark,
  getThemePreset,
  subscribeTheme,
} from '@/lib/theme';
import type { ThemePresetId } from '@/shell';

export const useColorMode = (): ColorMode =>
  useSyncExternalStore(subscribeTheme, getColorMode, getColorMode);

/** Whether it is dark right now. In system mode it follows the system preference and re-renders when that changes. */
export const useIsDark = (): boolean =>
  useSyncExternalStore(subscribeTheme, getIsDark, getIsDark);

export const useThemePreset = (): ThemePresetId =>
  useSyncExternalStore(subscribeTheme, getThemePreset, getThemePreset);
