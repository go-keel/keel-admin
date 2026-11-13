import { afterEach, describe, expect, it, vi } from 'vitest';
import { shell } from '@/shell';
import { isDarkMode, resolveColorMode, resolveThemePreset } from './theme';

// The preset list lives in the assembly seam, and a derived repo may replace it entirely (delete all the template's
// presets and register only its own). The test brings its own list and does not depend on what shell.ts registers
// right now; otherwise kernel tests would turn red in derived repos.
vi.mock('@/shell', async (importOriginal) => ({
  ...(await importOriginal<typeof import('@/shell')>()),
  themePresets: [
    { id: 'first', label: 'First' },
    { id: 'second', label: 'Second' },
  ],
}));

afterEach(() => {
  shell.colorMode = { state: 'auto' };
  shell.themePreset = { state: 'auto' };
  localStorage.clear();
});

describe('color mode resolution', () => {
  it('follows the system when nobody has chosen', () => {
    expect(resolveColorMode(null)).toBe('system');
  });

  it('recognises the three valid values', () => {
    expect(resolveColorMode('light')).toBe('light');
    expect(resolveColorMode('dark')).toBe('dark');
    expect(resolveColorMode('system')).toBe('system');
  });

  it('a junk stored value falls back to system instead of pinning <html> to a mode that does not exist', () => {
    expect(resolveColorMode('midnight')).toBe('system');
  });

  it('fixed ignores the user choice', () => {
    shell.colorMode = { state: 'fixed', value: 'dark' };
    expect(resolveColorMode('light')).toBe('dark');
  });

  it('only system looks at the OS preference; light/dark are hard choices', () => {
    expect(isDarkMode('system', true)).toBe(true);
    expect(isDarkMode('system', false)).toBe(false);
    expect(isDarkMode('dark', false)).toBe(true);
    expect(isDarkMode('light', true)).toBe(false);
  });
});

describe('appearance preset resolution', () => {
  it('uses the first registered preset when nobody has chosen', () => {
    expect(resolveThemePreset(null)).toBe('first');
  });

  it('an unregistered id falls back to the default, so a preset deleted by a derived repo does not leave the site without tokens', () => {
    expect(resolveThemePreset('gone-with-the-fork')).toBe('first');
  });

  it('uses a registered id as is', () => {
    expect(resolveThemePreset('second')).toBe('second');
  });

  it('fixed ignores the user choice', () => {
    shell.themePreset = { state: 'fixed', value: 'second' };
    expect(resolveThemePreset('first')).toBe('second');
  });

  it('uses the default of the derived repo when nobody has chosen; it beats the first registered entry', () => {
    shell.themePreset = { state: 'auto', default: 'second' };
    expect(resolveThemePreset(null)).toBe('second');
    // Once the user has chosen, default has no say.
    expect(resolveThemePreset('first')).toBe('first');
  });

  it('an unregistered default is treated like an unregistered stored value and falls back too', () => {
    const warn = vi.spyOn(console, 'warn').mockImplementation(() => {});
    shell.themePreset = { state: 'auto', default: 'gone-with-the-fork' };
    expect(resolveThemePreset(null)).toBe('first');
    // default comes from the derived repo's build-time config; a misspelling must be reported to someone
    expect(warn).toHaveBeenCalledOnce();
    warn.mockRestore();
  });

  it('an unregistered fixed value also falls back and warns; otherwise the whole appearance fails silently', () => {
    const warn = vi.spyOn(console, 'warn').mockImplementation(() => {});
    shell.themePreset = { state: 'fixed', value: 'gone-with-the-fork' };
    // Without the warning, <html data-theme-preset> would get a value no CSS matches
    expect(resolveThemePreset('second')).toBe('first');
    expect(warn).toHaveBeenCalledOnce();
    warn.mockRestore();
  });

  it('a once-registered stored value falls back to the default without warning; a deleted preset is a normal generation change', () => {
    const warn = vi.spyOn(console, 'warn').mockImplementation(() => {});
    expect(resolveThemePreset('gone-with-the-fork')).toBe('first');
    expect(warn).not.toHaveBeenCalled();
    warn.mockRestore();
  });
});
