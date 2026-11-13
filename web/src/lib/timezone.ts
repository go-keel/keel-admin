import { shell } from '@/shell';
import { readPreference, writePreference } from './preference';

export type TimeZoneId = string;

export type TimeZoneDefinition = {
  id: TimeZoneId;
  // Omitted: fallbackLabel is shown as is; an auto-added business time zone has no message to point to.
  labelKey?: string;
  fallbackLabel: string;
  // A fixed abbreviation may be declared (e.g. CST / IST); otherwise Intl derives it for the given instant,
  // so America/New_York switches between EST and EDT on its own.
  shortName?: string;
};

export type TimeOfDay = 'morning' | 'afternoon' | 'evening';

const STORAGE_KEY = 'keel_timezone';

const registry = new Map<TimeZoneId, TimeZoneDefinition>([
  [
    'UTC',
    {
      id: 'UTC',
      labelKey: 'timezone.utc',
      fallbackLabel: 'UTC',
      shortName: 'UTC',
    },
  ],
]);

const listeners = new Set<() => void>();
// The user's explicit choice (a localStorage hit or a setTimeZone call), cached for good.
let selectedTimeZone: TimeZoneId | undefined;
// Cache of the derived value (browser derivation or fixed-value check). formatDateTime calls getTimeZone per row
// and cannot rerun Intl each time; registerTimeZones and setBusinessTimeZone clear it, otherwise a first
// derivation that happens before them (the login page can trigger one) stays pinned to UTC.
let derivedTimeZone: TimeZoneId | undefined;
// The business time zone sent by the server; only available after sign-in (see setBusinessTimeZone).
let businessTimeZone: TimeZoneId | undefined;

// Registers extra IANA time zones; id must be a time zone the browser's Intl supports. Derived repos call this in
// web/src/timezones.ts only.
export const registerTimeZones = (...definitions: TimeZoneDefinition[]) => {
  for (const definition of definitions) {
    new Intl.DateTimeFormat('en-US', { timeZone: definition.id });
    registry.set(definition.id, definition);
  }
  derivedTimeZone = undefined;
  for (const listener of listeners) listener();
};

/**
 * Called once by the authenticated shell (routes/_app.tsx) when the business time zone arrives: when nobody has chosen
 * and the browser time zone is not recognised, the display time zone falls to it instead of UTC. If it is not in the
 * dropdown it is added, otherwise the current value shows blank in the dropdown; a registered one is not
 * overwritten, so the display name and abbreviation the derived repo gave it are kept.
 */
export const setBusinessTimeZone = (zone: TimeZoneId) => {
  if (zone === businessTimeZone) return;
  try {
    new Intl.DateTimeFormat('en-US', { timeZone: zone });
  } catch {
    console.warn(
      `[timezone] business time zone ${zone} is not accepted by Intl; the display time zone does not follow it`,
    );
    return;
  }
  businessTimeZone = zone;
  if (!registry.has(zone))
    registry.set(zone, { id: zone, fallbackLabel: zone });
  derivedTimeZone = undefined;
  for (const listener of listeners) listener();
};

/**
 * Business time zone: the value the authenticated shell writes on entry (`setBusinessTimeZone`); UTC when never
 * written or not accepted by Intl.
 * Components read it through `useBusinessTimeZone`: a synchronous read that never suspends. It is not read from the
 * query cache; that copy only serves the shell's own fetching.
 */
export const getBusinessTimeZone = (): TimeZoneId => businessTimeZone ?? 'UTC';

export const listTimeZones = (): TimeZoneDefinition[] => [...registry.values()];

// The browser time zone is used only on a registry hit: the built-in UTC, zones registered with registerTimeZones
// and the auto-added business time zone all count as hits;
// everything else (including Intl being unavailable) falls through to the next tier.
const browserTimeZone = (): TimeZoneId | undefined => {
  try {
    const zone = Intl.DateTimeFormat().resolvedOptions().timeZone;
    return zone && registry.has(zone) ? zone : undefined;
  } catch {
    return undefined;
  }
};

// A fixed time zone skips the registry: the registry governs what the dropdown offers, and the fixed mode has no
// dropdown; requiring it would also hit the ordering problem that registerTimeZones is a runtime call while shell
// is a build-time constant.
// Only check that Intl accepts it and fall back to UTC if not; otherwise formatDateTime would throw on every row.
const resolveFixedTimeZone = (zone: TimeZoneId): TimeZoneId => {
  if (derivedTimeZone) return derivedTimeZone;
  try {
    new Intl.DateTimeFormat('en-US', { timeZone: zone });
    derivedTimeZone = zone;
  } catch {
    console.warn(
      `[shell] fixed time zone ${zone} is not accepted by Intl; falling back to UTC`,
    );
    derivedTimeZone = 'UTC';
  }
  return derivedTimeZone;
};

export const getTimeZone = (): TimeZoneId => {
  // The fixed mode beats everything: the user's choice and browser derivation are ignored.
  if (shell.timeZone.state === 'fixed') {
    return resolveFixedTimeZone(shell.timeZone.value);
  }
  if (selectedTimeZone && registry.has(selectedTimeZone)) {
    return selectedTimeZone;
  }
  if (typeof window === 'undefined') return 'UTC';
  const stored = readPreference(STORAGE_KEY);
  if (stored && registry.has(stored)) {
    selectedTimeZone = stored;
    return stored;
  }
  if (derivedTimeZone) return derivedTimeZone;
  // When nobody has chosen, look at the derived repo's default first; it beats browser derivation, otherwise
  // configuring it would have no effect. Unlike fixed, default must be in the registry: it is a dropdown item, and
  // a default that cannot be selected only makes the dropdown show blank.
  const preferred = shell.timeZone.default;
  if (preferred) {
    if (registry.has(preferred)) {
      derivedTimeZone = preferred;
      return derivedTimeZone;
    }
    console.warn(
      `[shell] default time zone ${preferred} is not registered; deriving from the browser time zone`,
    );
  }
  // If the browser zone is not recognised, follow the business time zone; it arrives after sign-in, and until then
  // the fallback is UTC (the login page shows no times).
  derivedTimeZone = browserTimeZone() ?? businessTimeZone ?? 'UTC';
  return derivedTimeZone;
};

export const setTimeZone = (timeZone: TimeZoneId) => {
  // The fixed mode is hard: configuration decides, and a switcher the business side builds itself has no effect.
  if (shell.timeZone.state === 'fixed') return;
  if (!registry.has(timeZone)) return;
  selectedTimeZone = timeZone;
  writePreference(STORAGE_KEY, timeZone);
  for (const listener of listeners) listener();
};

export const subscribeTimeZone = (listener: () => void) => {
  listeners.add(listener);
  return () => {
    listeners.delete(listener);
  };
};

const dateTimeFormatter = (timeZone: TimeZoneId) =>
  new Intl.DateTimeFormat('en-CA', {
    timeZone,
    year: 'numeric',
    month: '2-digit',
    day: '2-digit',
    hour: '2-digit',
    minute: '2-digit',
    second: '2-digit',
    hourCycle: 'h23',
  });

// Returns the UTC offset of the given IANA time zone at the current instant; it changes with the date in DST regions.
export const formatTimeZoneOffset = (
  timeZone: TimeZoneId,
  at: Date = new Date(),
): string => {
  const name = new Intl.DateTimeFormat('en-US', {
    timeZone,
    timeZoneName: 'longOffset',
  })
    .formatToParts(at)
    .find((part) => part.type === 'timeZoneName')?.value;
  if (!name || name === 'GMT') return 'UTC+00:00';
  return name.replace(/^GMT/, 'UTC');
};

export const formatTimeZoneShortName = (
  timeZone: TimeZoneId,
  at: Date = new Date(),
): string => {
  const configured = registry.get(timeZone)?.shortName;
  if (configured) return configured;
  return (
    new Intl.DateTimeFormat('en-US', {
      timeZone,
      timeZoneName: 'short',
    })
      .formatToParts(at)
      .find((part) => part.type === 'timeZoneName')?.value ??
    formatTimeZoneOffset(timeZone, at)
  );
};

export const getTimeOfDay = (
  timeZone: TimeZoneId,
  at: Date = new Date(),
): TimeOfDay => {
  const hour = Number(
    new Intl.DateTimeFormat('en-US', {
      timeZone,
      hour: '2-digit',
      hourCycle: 'h23',
    })
      .formatToParts(at)
      .find((part) => part.type === 'hour')?.value,
  );
  if (hour < 12) return 'morning';
  if (hour < 18) return 'afternoon';
  return 'evening';
};

/** Placeholder when there is no time to show. Exported so DateTimeText can recognise it and add no zone label. */
export const EMPTY_TIME = '-';

export const formatDateTime = (
  value: string | number | Date | undefined | null,
  timeZone: TimeZoneId = getTimeZone(),
): string => {
  if (value === undefined || value === null || value === '') return EMPTY_TIME;
  const date = value instanceof Date ? value : new Date(value);
  if (Number.isNaN(date.getTime())) return String(value);
  const parts = Object.fromEntries(
    dateTimeFormatter(timeZone)
      .formatToParts(date)
      .map((part) => [part.type, part.value]),
  );
  return `${parts.year}-${parts.month}-${parts.day} ${parts.hour}:${parts.minute}:${parts.second}`;
};
