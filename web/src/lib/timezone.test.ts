import { beforeEach, describe, expect, it, vi } from 'vitest';
import type { ShellControl } from '@/shell';
import type { TimeZoneId } from './timezone';

/**
 * The timezone module caches the derived time zone, so the first derivation of each tri-state mode must start from a
 * clean module. shell is re-imported too: after resetModules timezone gets a new shell instance, and changing the old
 * instance does not reach it.
 */
const fresh = async (
  timeZone: ShellControl<TimeZoneId> = { state: 'auto' },
) => {
  vi.resetModules();
  const { shell } = await import('@/shell');
  shell.timeZone = timeZone;
  return await import('./timezone');
};

// The kernel only builds in UTC; cases that need a second time zone register one the way the assembly seam
// web/src/timezones.ts does.
const SHANGHAI = {
  id: 'Asia/Shanghai',
  labelKey: 'app:timezone.beijing',
  fallbackLabel: 'Beijing Time',
  shortName: 'CST',
};
const freshWithShanghai = async (timeZone?: ShellControl<TimeZoneId>) => {
  const mod = await fresh(timeZone);
  mod.registerTimeZones(SHANGHAI);
  return mod;
};

/** Pretend the browser is in a given time zone: the test machine's own zone must not reach the assertions. */
const browserIn = (timeZone: string) =>
  vi
    .spyOn(Intl.DateTimeFormat.prototype, 'resolvedOptions')
    .mockReturnValue({ timeZone } as Intl.ResolvedDateTimeFormatOptions);

beforeEach(() => {
  localStorage.clear();
  vi.restoreAllMocks();
});

describe('time zone tri-state', () => {
  it('auto: uses the user choice when it can be read', async () => {
    localStorage.setItem('keel_timezone', 'Asia/Shanghai');
    const { getTimeZone } = await freshWithShanghai();
    expect(getTimeZone()).toBe('Asia/Shanghai');
  });

  it('auto: a stored value missing from the registry does not count', async () => {
    localStorage.setItem('keel_timezone', 'Mars/Olympus');
    const { getTimeZone } = await fresh();
    expect(getTimeZone()).not.toBe('Mars/Olympus');
  });

  it('fixed: ignores the user choice, and setTimeZone cannot change it', async () => {
    localStorage.setItem('keel_timezone', 'Asia/Shanghai');
    const { getTimeZone, setTimeZone } = await freshWithShanghai({
      state: 'fixed',
      value: 'UTC',
    });
    expect(getTimeZone()).toBe('UTC');
    setTimeZone('Asia/Shanghai');
    expect(getTimeZone()).toBe('UTC');
  });

  it('fixed: a fixed value Intl rejects falls back to UTC instead of making every time row throw', async () => {
    const { getTimeZone } = await fresh({
      state: 'fixed',
      value: 'Nowhere/Nothing',
    });
    expect(getTimeZone()).toBe('UTC');
  });

  it('hidden: the control is not rendered, but the value is resolved the same way as auto', async () => {
    localStorage.setItem('keel_timezone', 'Asia/Shanghai');
    const { getTimeZone } = await freshWithShanghai({ state: 'hidden' });
    expect(getTimeZone()).toBe('Asia/Shanghai');
  });
});

// Order when nobody has chosen: default (when configured, the browser is not consulted) → browser → business time
// zone → UTC.
describe('display time zone when nobody has chosen', () => {
  it('uses the browser time zone when it is registered; the business time zone does not override it', async () => {
    browserIn('Asia/Shanghai');
    const { getTimeZone, setBusinessTimeZone } = await freshWithShanghai();
    setBusinessTimeZone('Asia/Tokyo');
    expect(getTimeZone()).toBe('Asia/Shanghai');
  });

  it('follows the business time zone when the browser zone is not recognised, and adds it to the dropdown', async () => {
    browserIn('America/Sao_Paulo');
    const { getTimeZone, setBusinessTimeZone, listTimeZones } = await fresh();
    expect(getTimeZone()).toBe('UTC'); // before sign-in: the business time zone has not arrived yet
    setBusinessTimeZone('Asia/Tokyo');
    expect(getTimeZone()).toBe('Asia/Tokyo');
    expect(listTimeZones().map((z) => z.id)).toContain('Asia/Tokyo');
  });

  it('a registered business time zone keeps the display name and abbreviation the derived repo gave it', async () => {
    const { setBusinessTimeZone, listTimeZones } = await freshWithShanghai();
    setBusinessTimeZone('Asia/Shanghai');
    expect(listTimeZones().find((z) => z.id === 'Asia/Shanghai')).toEqual(
      SHANGHAI,
    );
  });

  it('with default configured, the browser is not consulted', async () => {
    browserIn('Asia/Shanghai');
    const { getTimeZone } = await freshWithShanghai({
      state: 'auto',
      default: 'UTC',
    });
    expect(getTimeZone()).toBe('UTC');
  });

  it('a business time zone Intl rejects is not adopted', async () => {
    browserIn('America/Sao_Paulo');
    const { getTimeZone, setBusinessTimeZone } = await fresh();
    setBusinessTimeZone('Nowhere/Nothing');
    expect(getTimeZone()).toBe('UTC');
  });
});

describe('reading the business time zone', () => {
  it('is UTC before the shell writes it and that zone afterwards', async () => {
    const { getBusinessTimeZone, setBusinessTimeZone } = await fresh();
    expect(getBusinessTimeZone()).toBe('UTC');
    setBusinessTimeZone('Asia/Tokyo');
    expect(getBusinessTimeZone()).toBe('Asia/Tokyo');
  });

  it('one Intl rejects is not adopted; it stays UTC', async () => {
    const { getBusinessTimeZone, setBusinessTimeZone } = await fresh();
    setBusinessTimeZone('Nowhere/Nothing');
    expect(getBusinessTimeZone()).toBe('UTC');
  });
});

describe('time formatting', () => {
  it('renders in the given time zone, not the browser local zone', async () => {
    const { formatDateTime } = await fresh();
    const at = '2026-01-02T03:04:05Z';
    expect(formatDateTime(at, 'UTC')).toBe('2026-01-02 03:04:05');
    expect(formatDateTime(at, 'Asia/Shanghai')).toBe('2026-01-02 11:04:05');
  });

  it('renders an empty value as the placeholder rather than Invalid Date', async () => {
    const { formatDateTime } = await fresh();
    expect(formatDateTime(undefined, 'UTC')).toBe('-');
    expect(formatDateTime('', 'UTC')).toBe('-');
  });

  it('morning, afternoon and evening follow the chosen time zone and differ across zones', async () => {
    const { getTimeOfDay } = await fresh();
    const at = new Date('2026-01-02T23:00:00Z');
    expect(getTimeOfDay('UTC', at)).toBe('evening');
    expect(getTimeOfDay('Asia/Shanghai', at)).toBe('morning');
  });

  // The short name is on the render path of every time row (DateTimeText's zone label): getting it wrong raises
  // no error, every row just carries a wrong zone label, which is exactly the ambiguity the label is meant to remove.
  it('uses the configured value when shortName is registered', async () => {
    const { formatTimeZoneShortName } = await freshWithShanghai();
    expect(formatTimeZoneShortName('UTC')).toBe('UTC');
    expect(formatTimeZoneShortName('Asia/Shanghai')).toBe('CST');
  });

  it('falls back to Intl when shortName is not registered, and still yields a label', async () => {
    const { formatTimeZoneShortName } = await fresh();
    const got = formatTimeZoneShortName(
      'America/New_York',
      new Date('2026-01-02T12:00:00Z'),
    );
    expect(got).toBeTruthy();
    expect(got).not.toBe('America/New_York');
  });
});

/**
 * A time zone's labelKey comes from the registry rather than a literal and is read through tDynamic, which bypasses
 * the key type check of t() and with it the "missing zh-CN key fails tsc" gate (see locales/types.ts).
 * A missing entry raises no error; the Chinese UI just shows an English fallback, which nobody would report as a bug.
 *
 * Time zones registered in web/src/timezones.ts are not covered here: their keys live in the business copy (the app
 * namespace). This only locks the kernel's built-in ones.
 */
describe('message completeness of time zone display names', () => {
  const lookup = (resource: object, key: string): unknown =>
    key
      .split('.')
      .reduce<unknown>(
        (node, seg) => (node as Record<string, unknown>)?.[seg],
        resource,
      );

  it('every built-in time zone has its labelKey in both language resources', async () => {
    const { listTimeZones } = await fresh();
    const [enUS, zhCN] = await Promise.all([
      import('@/locales/sys/en-US'),
      import('@/locales/sys/zh-CN'),
    ]);
    for (const zone of listTimeZones()) {
      expect(
        lookup(enUS.default, zone.labelKey ?? ''),
        `en-US is missing ${zone.labelKey}`,
      ).toBeTypeOf('string');
      expect(
        lookup(zhCN.default, zone.labelKey ?? ''),
        `zh-CN is missing ${zone.labelKey}`,
      ).toBeTypeOf('string');
    }
  });
});
