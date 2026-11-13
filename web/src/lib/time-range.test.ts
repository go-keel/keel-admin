import { describe, expect, it } from 'vitest';
import {
  instantOfWallClock,
  instantRangeOfWall,
  monthRange,
  parseRange,
  recentRange,
  todayIn,
  toInstantRange,
  wallClockIn,
  wallRangeOfInstant,
} from './time-range';

describe('todayIn / recentRange', () => {
  // 3 a.m. on Sep 11 in Beijing; UTC is still on Sep 10.
  const now = new Date('2026-09-10T19:00:00Z');

  it('takes "today" in the display time zone, not the browser', () => {
    expect(todayIn('Asia/Shanghai', now)).toBe('2026-09-11');
    expect(todayIn('UTC', now)).toBe('2026-09-10');
    expect(recentRange(0, 0, 'Asia/Shanghai', now)).toBe(
      '2026-09-11~2026-09-11',
    );
  });

  it('last N days include today; across a month boundary it counts back on the calendar', () => {
    const march1 = new Date('2026-03-01T12:00:00Z');
    expect(recentRange(1, 1, 'UTC', march1)).toBe('2026-02-28~2026-02-28');
    expect(recentRange(6, 0, 'UTC', march1)).toBe('2026-02-23~2026-03-01');
    expect(recentRange(29, 0, 'UTC', march1)).toBe('2026-01-31~2026-03-01');
  });
});

describe('monthRange', () => {
  it('this month and last month are whole months; year boundaries and leap Februaries carry on the calendar', () => {
    const now = new Date('2026-01-15T12:00:00Z');
    expect(monthRange(0, 'UTC', now)).toBe('2026-01-01~2026-01-31');
    expect(monthRange(-1, 'UTC', now)).toBe('2025-12-01~2025-12-31');
    expect(monthRange(-1, 'UTC', new Date('2028-03-10T00:00:00Z'))).toBe(
      '2028-02-01~2028-02-29',
    );
  });

  it('takes the month from today in the given zone: Oct 1 in Beijing while UTC is still in September', () => {
    const now = new Date('2026-09-30T17:00:00Z');
    expect(monthRange(0, 'Asia/Shanghai', now)).toBe('2026-10-01~2026-10-31');
    expect(monthRange(0, 'UTC', now)).toBe('2026-09-01~2026-09-30');
  });
});

describe('wallClockIn / instantOfWallClock', () => {
  it('round trip: wall clock → instant → wall clock', () => {
    const at = instantOfWallClock('2026-09-01 00:00:00', 'Asia/Shanghai');
    expect(at.toISOString()).toBe('2026-08-31T16:00:00.000Z');
    expect(wallClockIn(at, 'Asia/Shanghai')).toBe('2026-09-01 00:00:00');
    expect(wallClockIn(at, 'UTC')).toBe('2026-08-31 16:00:00');
  });

  it('takes the DST offset at the target instant, not at the present', () => {
    // New York is UTC-4 in July (DST) and UTC-5 in January.
    const summer = instantOfWallClock(
      '2026-07-01 12:00:00',
      'America/New_York',
    );
    const winter = instantOfWallClock(
      '2026-01-01 12:00:00',
      'America/New_York',
    );
    expect(summer.toISOString()).toBe('2026-07-01T16:00:00.000Z');
    expect(winter.toISOString()).toBe('2026-01-01T17:00:00.000Z');
  });

  // Taking the offset only once is off by a whole hour on a switch day: when the rough guess and the target lie on
  // opposite sides of the switch, the first pass gets the stale offset.
  it('is consistent on both sides of a switch day; a single pass is off by an hour', () => {
    const cases: [string, string][] = [
      ['2026-03-08 06:30:00', 'America/New_York'], // after spring forward
      ['2026-03-08 01:30:00', 'America/New_York'], // before spring forward
      ['2026-11-01 05:30:00', 'America/New_York'], // after fall back
      ['2026-03-29 03:30:00', 'Europe/Berlin'],
    ];
    for (const [wall, zone] of cases) {
      expect(wallClockIn(instantOfWallClock(wall, zone), zone)).toBe(wall);
    }
  });
});

describe('toInstantRange', () => {
  it('date range: from is 00:00 of the start day, to is 00:00 after the end day, in the filter time zone', () => {
    const r = parseRange('2026-09-01~2026-09-03', 'date');
    expect(r && toInstantRange(r, 'date', 'Asia/Shanghai')).toEqual({
      from: '2026-08-31T16:00:00.000Z',
      to: '2026-09-03T16:00:00.000Z',
    });
  });

  // New York springs forward on 2026-03-08, a 23-hour day: the next day is added on the calendar, not as 24 hours.
  it('on the DST day the end is still 00:00 of the next day', () => {
    const r = parseRange('2026-03-08~2026-03-08', 'date');
    const at = r && toInstantRange(r, 'date', 'America/New_York');
    expect(at).toEqual({
      from: '2026-03-08T05:00:00.000Z',
      to: '2026-03-09T04:00:00.000Z',
    });
  });

  it('time range: the last second is included; to 23:59:59 means before 00:00 the next day', () => {
    const r = parseRange('2026-09-15 00:00:00~2026-09-15 23:59:59', 'time');
    expect(r && toInstantRange(r, 'time', 'Asia/Shanghai')).toEqual({
      from: '2026-09-14T16:00:00.000Z',
      to: '2026-09-15T16:00:00.000Z',
    });
  });

  it('an empty end means unbounded', () => {
    const r = parseRange('~2026-09-03', 'date');
    expect(r && toInstantRange(r, 'date', 'UTC')).toEqual({
      from: undefined,
      to: '2026-09-04T00:00:00.000Z',
    });
  });
});

describe('instantRangeOfWall / wallRangeOfInstant', () => {
  it('both ends inclusive in the UI; the end handed over is the next second (exclusive), to the second, no milliseconds', () => {
    expect(
      instantRangeOfWall(
        { from: '2026-09-15 00:00:00', to: '2026-09-15 23:59:59' },
        'Asia/Shanghai',
      ),
    ).toEqual({ from: '2026-09-14T16:00:00Z', to: '2026-09-15T16:00:00Z' });
  });

  it('round trip: instant range → wall clock → the same instant range; an empty end stays empty', () => {
    const r = { from: '', to: '2026-03-09T04:00:00Z' };
    const wall = wallRangeOfInstant(r, 'America/New_York');
    expect(wall).toEqual({ from: '', to: '2026-03-08 23:59:59' });
    expect(instantRangeOfWall(wall, 'America/New_York')).toEqual(r);
  });
});
