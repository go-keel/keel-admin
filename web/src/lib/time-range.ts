/**
 * Values and conversion of time filters (how they are passed:
 * docs/adr/admin-0003-three-meanings-of-time-and-how-to-pass-them.md).
 *
 * A filter value is a `from~to` string; either end may be empty. One end of a date range is `YYYY-MM-DD`, one end of a
 * time range is `YYYY-MM-DD HH:mm:ss` (to the second).
 *
 * A filter that follows the display time zone is converted into UTC instants when data is fetched, using the display
 * time zone of that moment, half-open; a fixed filter (business day) sends only the date, and the server converts it
 * in the business time zone.
 */

/** The wall-clock reading of an absolute instant in a time zone. */
export function wallClockIn(at: Date, timeZone: string): string {
  const parts = Object.fromEntries(
    new Intl.DateTimeFormat('en-CA', {
      timeZone,
      year: 'numeric',
      month: '2-digit',
      day: '2-digit',
      hour: '2-digit',
      minute: '2-digit',
      second: '2-digit',
      hourCycle: 'h23',
    })
      .formatToParts(at)
      .map((p) => [p.type, p.value]),
  );
  return `${parts.year}-${parts.month}-${parts.day} ${parts.hour}:${parts.minute}:${parts.second}`;
}

/**
 * The absolute instant that a wall-clock reading in a time zone corresponds to.
 *
 * **Two passes**: the first reads the wall-clock digits as UTC to get a rough guess and takes the offset at that
 * instant; the second **takes the offset again** at the corrected instant. A single pass is off by a whole hour on a
 * DST switch day: when the guess and the target lie on opposite sides of the switch, the first pass gets the stale
 * offset (New York 2026-03-08 06:30 would become 11:30Z; the correct value is 10:30Z).
 *
 * A wall-clock time in the spring-forward gap (e.g. 02:30) does not exist; after two passes it lands on the side after
 * the jump. Any answer is a convention; this one aligns forward.
 */
export function instantOfWallClock(wall: string, timeZone: string): Date {
  const asUTC = Date.parse(`${wall.replace(' ', 'T')}Z`);
  const offsetAt = (at: number): number =>
    Date.parse(`${wallClockIn(new Date(at), timeZone).replace(' ', 'T')}Z`) -
    at;
  const rough = asUTC - offsetAt(asUTC);
  return new Date(asUTC - offsetAt(rough));
}

/** Today in the display time zone (YYYY-MM-DD). */
export const todayIn = (timeZone: string, now = new Date()): string =>
  wallClockIn(now, timeZone).slice(0, 10);

/**
 * A quick range `from~to`: anchored on today in the display time zone, from `fromAgo` days back to `toAgo` days back
 * (0 is today). "Today" is not taken from the browser: when the two zones differ, the browser would pick today
 * somewhere else. Day arithmetic runs on UTC, through no time zone, so it never meets the DST hour.
 */
export function recentRange(
  fromAgo: number,
  toAgo: number,
  timeZone: string,
  now = new Date(),
): string {
  const today = Date.parse(`${todayIn(timeZone, now)}T00:00:00Z`);
  const day = (ago: number) =>
    new Date(today - ago * 86_400_000).toISOString().slice(0, 10);
  return `${day(fromAgo)}~${day(toAgo)}`;
}

/**
 * A whole-month quick range `1st~month end`: `offset` 0 is this month, -1 last month; the month comes from today in
 * `timeZone`. The month end takes the whole month rather than stopping at today: filtering into future days yields
 * no extra data, and the range matches the literal "this month".
 */
export function monthRange(
  offset: number,
  timeZone: string,
  now = new Date(),
): string {
  const today = todayIn(timeZone, now);
  const year = Number(today.slice(0, 4));
  const month = Number(today.slice(5, 7)) - 1 + offset;
  const iso = (d: Date) => d.toISOString().slice(0, 10);
  // Month overflow is carried by Date.UTC: month -1 is December of the previous year, day 0 of next month is this
  // month's end.
  return `${iso(new Date(Date.UTC(year, month, 1)))}~${iso(new Date(Date.UTC(year, month + 1, 0)))}`;
}

export type RangeKind = 'date' | 'time';

export type RangeValue = { from: string; to: string };

const END_PATTERN: Record<RangeKind, RegExp> = {
  date: /^\d{4}-\d{2}-\d{2}$/,
  time: /^\d{4}-\d{2}-\d{2} \d{2}:\d{2}:\d{2}$/,
};

/**
 * Raw value in the URL → filter value. A malformed value (a broken link, a hand-typed mistake) counts as not set, the
 * same rule as other filters ("unknown values are dropped"); both ends empty also counts as not set.
 */
export function parseRange(
  value: string | undefined,
  kind: RangeKind,
): RangeValue | undefined {
  if (!value) return undefined;
  const ends = value.split('~');
  if (ends.length !== 2) return undefined;
  const [from = '', to = ''] = ends;
  const valid = (end: string) => end === '' || END_PATTERN[kind].test(end);
  if ((!from && !to) || !valid(from) || !valid(to)) return undefined;
  return { from, to };
}

export const formatRange = ({ from, to }: RangeValue): string =>
  from || to ? `${from}~${to}` : '';

const nextDay = (day: string): string =>
  new Date(Date.parse(`${day}T00:00:00Z`) + 86_400_000)
    .toISOString()
    .slice(0, 10);

/**
 * Value of a filter that follows the display time zone → UTC instant range [from, to). **Both ends are inclusive in
 * the UI**: dates to the day, times to the second, and the end is pushed one unit later when converted: "to Sep 3"
 * sends "before Sep 4 00:00", "to 23:59:59" sends "before 00:00 the next day". This step happens only here, so neither
 * business pages nor endpoints need to remember the +1, and the last day or the milliseconds of the last second are
 * never lost. The next day is added on the calendar, through no time zone, so the DST day is right too.
 */
export function toInstantRange(
  { from, to }: RangeValue,
  kind: RangeKind,
  zone: string,
): { from?: string; to?: string } {
  const at = (wall: string) => instantOfWallClock(wall, zone).toISOString();
  if (kind === 'date') {
    return {
      from: from ? at(`${from} 00:00:00`) : undefined,
      to: to ? at(`${nextDay(to)} 00:00:00`) : undefined,
    };
  }
  const nextSecond = (wall: string) =>
    new Date(instantOfWallClock(wall, zone).getTime() + 1000).toISOString();
  return {
    from: from ? at(from) : undefined,
    to: to ? nextSecond(to) : undefined,
  };
}

/**
 * Instant → UTC RFC3339 to the second. The picker is only precise to the second, so milliseconds are always 0;
 * leaving them out is shorter and easier to compare.
 */
export const toRFC3339 = (at: Date): string =>
  at.toISOString().replace(/\.\d{3}Z$/, 'Z');

/**
 * Wall-clock readings of a time range (both ends inclusive, to the second) → UTC instant range [from, to), matching
 * how the endpoint takes it. This is the value the picker hands the form; the end is pushed one second later, as in
 * `toInstantRange`. An empty end yields `''`.
 */
export function instantRangeOfWall(
  range: RangeValue,
  zone: string,
): RangeValue {
  const { from, to } = toInstantRange(range, 'time', zone);
  return {
    from: from ? toRFC3339(new Date(from)) : '',
    to: to ? toRFC3339(new Date(to)) : '',
  };
}

/**
 * Inverse of `instantRangeOfWall`: UTC instant range [from, to) → wall-clock readings; the end steps back one second,
 * both ends inclusive.
 */
export function wallRangeOfInstant(
  range: RangeValue,
  zone: string,
): RangeValue {
  const wall = (at: string, back: number) =>
    at ? wallClockIn(new Date(Date.parse(at) - back), zone) : '';
  return { from: wall(range.from, 0), to: wall(range.to, 1000) };
}
