import { describe, expect, it } from 'vitest';
import type { ListFields, ListSearch } from './list';
import {
  initialListState,
  type ListEvent,
  type ListState,
  nextListState,
  resultSetKey,
} from './list-state';
import { DEFAULT_PAGE_SIZE } from './table';

const A: ListSearch = { keyword: 'a' };
const B: ListSearch = { keyword: 'b' };

const run = (state: ListState, ...events: ListEvent[]) =>
  events.reduce(nextListState, state);

describe('list state', () => {
  it('first open: conditions from the URL, page 1, default page size', () => {
    expect(initialListState(A)).toEqual({
      applied: A,
      revision: 0,
      page: 1,
      pageSize: DEFAULT_PAGE_SIZE,
    });
  });

  it('changing conditions goes back to page 1', () => {
    const s = run(
      initialListState(A),
      { type: 'pageChanged', page: 3 },
      { type: 'submitted', search: B },
    );
    expect(s.applied).toEqual(B);
    expect(s.page).toBe(1);
  });

  it('page size survives a condition change; changing page size goes back to page 1', () => {
    const sized = run(
      initialListState(A),
      { type: 'pageChanged', page: 3 },
      { type: 'pageSizeChanged', pageSize: 50 },
    );
    expect(sized.page).toBe(1);
    expect(run(sized, { type: 'submitted', search: B }).pageSize).toBe(50);
  });

  it('search with unchanged conditions: stays on the page, returns the same state (the caller only refetches)', () => {
    const s = run(initialListState(A), { type: 'pageChanged', page: 3 });
    expect(nextListState(s, { type: 'submitted', search: { ...A } })).toBe(s);
  });

  it('the URL catching up with conditions this page just wrote: no change', () => {
    const s = run(
      initialListState(A),
      { type: 'submitted', search: B },
      { type: 'pageChanged', page: 2 },
    );
    expect(nextListState(s, { type: 'urlChanged', search: { ...B } })).toBe(s);
  });

  it('URL changed externally: restores conditions from it and goes back to page 1', () => {
    const s = run(
      initialListState(A),
      { type: 'pageChanged', page: 3 },
      { type: 'urlChanged', search: B },
    );
    expect(s.applied).toEqual(B);
    expect(s.page).toBe(1);
  });

  it('same URL opened again: same conditions, page 1, a new revision', () => {
    const s = run(
      initialListState(A),
      { type: 'pageChanged', page: 3 },
      { type: 'reopened' },
    );
    expect(s.applied).toEqual(A);
    expect(s.page).toBe(1);
    expect(s.revision).toBe(1);
  });
});

describe('list state: switching away and back does not restore the old page', () => {
  it('A on page 3 → search B → search A again: page 1', () => {
    const s = run(
      initialListState(A),
      { type: 'pageChanged', page: 3 },
      { type: 'submitted', search: B },
      { type: 'submitted', search: A },
    );
    expect(s.page).toBe(1);
  });

  it('A on page 3 → search B → back to A: page 1', () => {
    const s = run(
      initialListState(A),
      { type: 'pageChanged', page: 3 },
      { type: 'submitted', search: B },
      { type: 'urlChanged', search: B },
      { type: 'urlChanged', search: A },
    );
    expect(s.applied).toEqual(A);
    expect(s.page).toBe(1);
  });
});

describe('resultSetKey', () => {
  const fields: ListFields = {
    keyword: { type: 'text' },
    created: { type: 'dateRange' },
  };
  const key = (s: ListState) => resultSetKey(fields, s);

  it('paging and changing page size keep the result set', () => {
    const s = initialListState(A);
    expect(key(run(s, { type: 'pageChanged', page: 2 }))).toBe(key(s));
    expect(key(run(s, { type: 'pageSizeChanged', pageSize: 50 }))).toBe(key(s));
  });

  it('changing conditions and reopening both change the result set', () => {
    const s = initialListState(A);
    expect(key(run(s, { type: 'submitted', search: B }))).not.toBe(key(s));
    expect(key(run(s, { type: 'reopened' }))).not.toBe(key(s));
  });

  it('depends only on filter values: absent and empty are the same set, display time zone ignored', () => {
    const empty = initialListState({});
    expect(key(empty)).toBe(key(initialListState({ keyword: undefined })));
    const ranged = initialListState({ created: '2026-09-01~2026-09-01' });
    expect(key(ranged)).toBe(key(initialListState({ ...ranged.applied })));
  });
});
