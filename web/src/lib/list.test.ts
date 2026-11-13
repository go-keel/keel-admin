import { describe, expect, it, vi } from 'vitest';
import {
  buildSearchSchema,
  defineList,
  type ListFields,
  resetUrlPatch,
  toFilterValues,
  toRequestParams,
  toUrlPatch,
  urlToFilterValues,
} from './list';
import { DEFAULT_PAGE_SIZE, intersectSelection } from './table';
import { registerTimeZones, setTimeZone } from './timezone';

describe('URL → filter bar values', () => {
  const fields: ListFields = {
    keyword: { type: 'text' },
    status: { type: 'number' },
  };

  it('text: strings pass as is; numbers, booleans and old quoted values become strings', () => {
    expect(urlToFilterValues(fields, { keyword: 'abc' }).keyword).toBe('abc');
    expect(urlToFilterValues(fields, { keyword: 3 }).keyword).toBe('3');
    // When the router reads the URL, "3" is a string; accept it the same way as the number 3 above.
    expect(urlToFilterValues(fields, { keyword: '3' }).keyword).toBe('3');
  });

  it('text: null / arrays / objects count as no value without throwing; unknown values are silently dropped', () => {
    expect(urlToFilterValues(fields, { keyword: null }).keyword).toBe('');
    expect(urlToFilterValues(fields, { keyword: [1, 2] }).keyword).toBe('');
    expect(urlToFilterValues(fields, { keyword: { a: 1 } }).keyword).toBe('');
  });

  it('number: a bare number (hand-built link) and a quoted numeric string (old link) read the same', () => {
    expect(urlToFilterValues(fields, { status: 1 }).status).toBe('1');
    expect(urlToFilterValues(fields, { status: '1' }).status).toBe('1');
  });

  it('number: no value when it does not convert to a finite number', () => {
    expect(urlToFilterValues(fields, { status: 'abc' }).status).toBe('');
    expect(urlToFilterValues(fields, { status: null }).status).toBe('');
  });

  it('time fields keep dates (times) only; malformed means no value: hours in a date range, both ends empty', () => {
    const f: ListFields = {
      day: { type: 'dateRange' },
      biz: { type: 'dateRange', zone: 'business' },
      time: { type: 'timeRange' },
    };
    expect(
      urlToFilterValues(f, {
        day: '2026-09-01~2026-09-03',
        biz: '~2026-09-03',
        time: '2026-09-01 10:00:00~2026-09-01 11:00:00',
      }),
    ).toEqual({
      day: '2026-09-01~2026-09-03',
      biz: '~2026-09-03',
      time: '2026-09-01 10:00:00~2026-09-01 11:00:00',
    });
    expect(
      urlToFilterValues(f, {
        day: '2026-09-01 10:00~',
        biz: '~',
        time: '2026-09-01 10:00~',
      }),
    ).toEqual({ day: '', biz: '', time: '' });
  });

  it('unknown URL keys are silently dropped, absent keys get an empty string', () => {
    const search = buildSearchSchema(fields).parse({
      keyword: 'a',
      page: 3,
      edit: 'new',
    });
    expect(search).toEqual({ keyword: 'a' });
    expect(toFilterValues(fields, search)).toEqual({
      keyword: 'a',
      status: '',
    });
  });
});

describe('buildSearchSchema: the validated output of number fields is a real number', () => {
  const fields: ListFields = { status: { type: 'number' } };

  // TanStack serializes the URL from the output of validateSearch: a numeric string "1" is treated by the default
  // stringifySearch as JSON text and gets quotes added again (status="1"); only the real number 1 is written as the
  // unquoted status=1 (root cause: ADR admin-0005). A clean link ?status=1 arrives here as a number already (the
  // router's default parseSearch runs JSON.parse on number-like values first); a quoted link ?status=%221%22 is the
  // string "1" after the same JSON.parse strips the quotes. Both arrival forms must become the same number, not a
  // string.
  it('numbers and numeric strings parsed from quoted links both become numbers, not strings', () => {
    const clean = buildSearchSchema(fields).parse({ status: 1 });
    const quoted = buildSearchSchema(fields).parse({ status: '1' });
    expect(clean.status).toBe(1);
    expect(quoted.status).toBe(1);
    expect(typeof clean.status).toBe('number');
    expect(typeof quoted.status).toBe('number');
  });

  it('no value when it does not convert to a finite number, so the route does not become an error surface', () => {
    expect(
      buildSearchSchema(fields).parse({ status: 'abc' }).status,
    ).toBeUndefined();
    expect(
      buildSearchSchema(fields).parse({ status: null }).status,
    ).toBeUndefined();
  });
});

describe('submit → URL', () => {
  const fields: ListFields = {
    keyword: { type: 'text' },
    status: { type: 'number' },
  };

  it('an empty string means no value and removes the key from the URL', () => {
    expect(toUrlPatch(fields, { keyword: '', status: '' })).toEqual({
      keyword: undefined,
      status: undefined,
    });
  });

  it('number fields become real numbers, so the URL reads the unquoted status=1', () => {
    expect(toUrlPatch(fields, { keyword: 'a', status: '1' })).toEqual({
      keyword: 'a',
      status: 1,
    });
  });
});

describe('resetUrlPatch', () => {
  it('reset clears every key in the definition, without per-page lists', () => {
    const fields: ListFields = {
      keyword: { type: 'text' },
      status: { type: 'number' },
      created: { type: 'dateRange' },
    };
    expect(resetUrlPatch(fields)).toEqual({
      keyword: undefined,
      status: undefined,
      created: undefined,
    });
  });
});

describe('URL → request parameters', () => {
  it('text: empty or absent values are not sent, the rest as is', () => {
    const fields: ListFields = { keyword: { type: 'text' } };
    expect(toRequestParams(fields, { keyword: '' })).toEqual({});
    expect(toRequestParams(fields, {})).toEqual({});
    expect(toRequestParams(fields, { keyword: 'abc' })).toEqual({
      keyword: 'abc',
    });
  });

  it('number: numbers as is, absent values not sent', () => {
    const fields: ListFields = { status: { type: 'number' } };
    expect(toRequestParams(fields, { status: '1' })).toEqual({ status: 1 });
    expect(toRequestParams(fields, {})).toEqual({});
  });

  it('following kind: uses the display time zone at fetch time; the end is 00:00 the next day (exclusive)', () => {
    const fields: ListFields = {
      day: { type: 'dateRange' },
      time: { type: 'timeRange' },
    };
    // The kernel has only UTC built in; setTimeZone rejects zones that are not registered.
    registerTimeZones({ id: 'Asia/Shanghai', fallbackLabel: 'Beijing Time' });
    setTimeZone('Asia/Shanghai');
    expect(
      toRequestParams(fields, {
        day: '2026-09-01~2026-09-01',
        time: '2026-09-01 10:00:00~2026-09-01 11:30:00',
      }),
    ).toEqual({
      dayFromAt: '2026-08-31T16:00:00.000Z',
      dayToAt: '2026-09-01T16:00:00.000Z',
      timeFromAt: '2026-09-01T02:00:00.000Z',
      // The end second shown in the UI is included: up to 11:30:00 sends "before 11:30:01".
      timeToAt: '2026-09-01T03:30:01.000Z',
    });
    setTimeZone('UTC');
    expect(
      toRequestParams(fields, { day: '2026-09-01~2026-09-01' }).dayFromAt,
    ).toBe('2026-09-01T00:00:00.000Z');
  });

  it('fixed kind: sends the dates as is, both inclusive; the server converts', () => {
    const fields: ListFields = {
      created: { type: 'dateRange', zone: 'business' },
    };
    expect(
      toRequestParams(fields, { created: '2026-09-01~2026-09-03' }),
    ).toEqual({ createdFromDate: '2026-09-01', createdToDate: '2026-09-03' });
    expect(toRequestParams(fields, { created: '~2026-09-03' })).toEqual({
      createdToDate: '2026-09-03',
    });
  });

  it('custom escape hatch: toParams computes the request parameters', () => {
    const fields: ListFields = {
      categoryId: { type: 'custom', toParams: (v) => ({ categoryIds: [v] }) },
    };
    expect(toRequestParams(fields, { categoryId: '3' })).toEqual({
      categoryIds: ['3'],
    });
  });
});

describe('staleTime of defineList', () => {
  const fetch = async () => ({ items: [], total: 0 });

  it('left out of the query options when not given, keeping the global default; passed through when given', () => {
    const plain = defineList({ queryKey: ['t', 'a'], fields: {}, fetch });
    const slow = defineList({
      queryKey: ['t', 'b'],
      fields: {},
      fetch,
      staleTime: 60_000,
    });
    expect('staleTime' in plain.queryOptions({}, 1, 20)).toBe(false);
    expect(slow.queryOptions({}, 1, 20).staleTime).toBe(60_000);
  });
});

describe('list definition with no fields (no filters)', () => {
  it('buildSearchSchema({}) validates to an empty object; unknown keys are silently dropped as usual', () => {
    expect(buildSearchSchema({}).parse({ keyword: 'a', page: 3 })).toEqual({});
  });

  it('defineList({ fields: {} }) declares the route normally and the loader prefetches page 1', async () => {
    const fetch = vi.fn(async () => ({ items: [], total: 0 }));
    const list = defineList({ queryKey: ['test', 'empty'], fields: {}, fetch });

    expect(list.route.validateSearch.parse({ page: 3 })).toEqual({});
    const deps = list.route.loaderDeps({ search: {} });
    expect(deps).toEqual({});

    // Fake queryClient: only checks that the loader builds the request and prefetches page 1, not how
    // react-query caches; that is upstream behaviour.
    const fakeQueryClient = {
      prefetchQuery: (opts: { queryFn: () => unknown }) => opts.queryFn(),
    };
    await list.route.loader({
      context: { queryClient: fakeQueryClient as never },
      deps,
    });
    expect(fetch).toHaveBeenCalledWith(
      expect.objectContaining({ offset: 0, limit: DEFAULT_PAGE_SIZE }),
    );
  });
});

describe('intersectSelection (intersection of checked rows)', () => {
  it('keeps only IDs in the current rows: leftover IDs deleted by a bulk delete no longer count', () => {
    const selection = { '1': true, '2': true, '3': false };
    expect(intersectSelection(selection, ['1', '4'])).toEqual(['1']);
  });

  it('empty current rows give an empty intersection', () => {
    expect(intersectSelection({ '1': true }, [])).toEqual([]);
  });
});
