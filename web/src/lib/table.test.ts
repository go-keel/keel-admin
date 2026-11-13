import { describe, expect, it } from 'vitest';
import {
  inlineCount,
  nextBatchOffset,
  PAGE_SIZES,
  pageCount,
  pageWindow,
  parseColumnWidths,
  rememberColumnWidths,
  restoreColumnWidths,
  spreadWidths,
  toOffsetParams,
  toTotal,
  widthFor,
} from './table';

describe('spreadWidths', () => {
  const cols = [
    { id: 'id', size: 80, pinned: true },
    { id: 'name', size: 100, pinned: false },
    { id: 'time', size: 300, pinned: false },
    { id: 'actions', size: 120, pinned: true },
  ];

  it('extra width goes in proportion to non-fixed columns only, fixed columns stay', () => {
    expect(spreadWidths(cols, 1000)).toEqual({
      id: 80,
      name: 200,
      time: 600,
      actions: 120,
    });
  });

  it('returned as is when the container is not wider, leaving it to horizontal scrolling', () => {
    expect(spreadWidths(cols, 400)).toEqual({
      id: 80,
      name: 100,
      time: 300,
      actions: 120,
    });
  });
});

describe('toOffsetParams', () => {
  it('page numbers start at 1, offsets at 0', () => {
    expect(toOffsetParams(1, 20)).toEqual({ offset: 0, limit: 20 });
    expect(toOffsetParams(2, 20)).toEqual({ offset: 20, limit: 20 });
    expect(toOffsetParams(3, 50)).toEqual({ offset: 100, limit: 50 });
  });

  it('a page number below 1 counts as the first page, never sending a negative offset', () => {
    expect(toOffsetParams(0, 20)).toEqual({ offset: 0, limit: 20 });
    expect(toOffsetParams(-5, 20)).toEqual({ offset: 0, limit: 20 });
  });
});

describe('toTotal', () => {
  it('int64 is a string over JSON', () => {
    expect(toTotal('42')).toBe(42);
    expect(toTotal(42)).toBe(42);
  });

  it('absent and invalid values become 0, so NaN never reaches the pager', () => {
    expect(toTotal(undefined)).toBe(0);
    expect(toTotal(null)).toBe(0);
    expect(toTotal('')).toBe(0);
    expect(toTotal('abc')).toBe(0);
    expect(toTotal('-1')).toBe(0);
  });
});

describe('pageCount', () => {
  it('rounds up', () => {
    expect(pageCount(0, 20)).toBe(1);
    expect(pageCount(1, 20)).toBe(1);
    expect(pageCount(20, 20)).toBe(1);
    expect(pageCount(21, 20)).toBe(2);
  });
});

describe('pageWindow', () => {
  it('gives all pages when there are fewer than the window', () => {
    expect(pageWindow(1, 3)).toEqual([1, 2, 3]);
    expect(pageWindow(2, 1)).toEqual([1]);
  });

  it('centres the current page', () => {
    expect(pageWindow(7, 10)).toEqual([5, 6, 7, 8, 9]);
  });

  it('clamps at both ends, never out of range', () => {
    expect(pageWindow(1, 10)).toEqual([1, 2, 3, 4, 5]);
    expect(pageWindow(10, 10)).toEqual([6, 7, 8, 9, 10]);
  });

  it('gives a valid window even when the current page is out of range', () => {
    expect(pageWindow(99, 5)).toEqual([1, 2, 3, 4, 5]);
    expect(pageWindow(0, 5)).toEqual([1, 2, 3, 4, 5]);
  });

  it('an empty list counts as one page, not an empty array', () => {
    // pageCount guarantees pages >= 1, but the window itself must not break on 0 either.
    expect(pageWindow(1, 0)).toEqual([1]);
  });
});

describe('PAGE_SIZES', () => {
  // The contract with the server: every list request in proto declares limit with lte: 200. An option above it
  // can be picked in the UI but the request is rejected with 400, a silent failure; change both sides together.
  it('the largest option equals the server limit cap of 200', () => {
    expect(Math.max(...PAGE_SIZES)).toBe(200);
  });
});

describe('widthFor (column width per language)', () => {
  const widths = { default: 258, 'zh-CN': 168 };

  it('uses the value of the current language when it is declared', () => {
    expect(widthFor(widths, 'zh-CN')).toBe(168);
  });

  it('falls back to the default when the language is not declared', () => {
    expect(widthFor(widths, 'ja-JP')).toBe(258);
    expect(widthFor({ default: 90 }, 'zh-CN')).toBe(90);
  });
});

describe('nextBatchOffset (next batch of an option set)', () => {
  const page = (n: number) => ({ items: Array.from({ length: n }) });

  it('continues from the fetched count until total is reached', () => {
    expect(nextBatchOffset([page(50)], 120)).toBe(50);
    expect(nextBatchOffset([page(50), page(50)], 120)).toBe(100);
  });

  it('stops when total is reached or the previous batch was empty', () => {
    expect(
      nextBatchOffset([page(50), page(50), page(20)], 120),
    ).toBeUndefined();
    expect(nextBatchOffset([page(50), page(0)], 120)).toBeUndefined();
  });
});

describe('inlineCount (inline cells of the actions column)', () => {
  it('everything inline without maxInline', () => {
    expect(inlineCount(5)).toBe(5);
  });

  it('everything inline when it fits, no "⋯"', () => {
    expect(inlineCount(3, 3)).toBe(3);
  });

  it('when it does not fit, the last cell goes to "⋯" and the total stays maxInline', () => {
    expect(inlineCount(4, 3)).toBe(2);
    expect(inlineCount(9, 3)).toBe(2);
  });
});

describe('remembered column widths', () => {
  const cols = [
    { id: 'id', size: 80, minSize: 60 },
    { id: 'name', size: 160, minSize: 60 },
    { id: 'actions', size: 120, minSize: 100 },
  ];

  it('unrecognized stored content counts as never stored', () => {
    expect(parseColumnWidths(null)).toEqual({});
    expect(parseColumnWidths('not json')).toEqual({});
    expect(parseColumnWidths('[1,2]')).toEqual({});
    expect(
      parseColumnWidths(
        '{"en-US":{"name":200,"id":"x","bad":-1},"zh-CN":3,"ja-JP":{}}',
      ),
    ).toEqual({ 'en-US': { name: 200 } });
  });

  it('takes only the widths dragged in the current language', () => {
    const memory = { 'en-US': { name: 240 }, 'zh-CN': { id: 90 } };
    expect(restoreColumnWidths(memory, 'zh-CN', cols)).toEqual({ id: 90 });
    expect(restoreColumnWidths(memory, 'ja-JP', cols)).toEqual({});
  });

  it('drops columns that no longer exist and clamps narrower ones to the minimum', () => {
    const memory = { 'en-US': { gone: 300, actions: 70 } };
    expect(restoreColumnWidths(memory, 'en-US', cols)).toEqual({
      actions: 100,
    });
  });

  it('writing back touches only the current language and skips widths equal to the default', () => {
    const memory = { 'zh-CN': { id: 90 } };
    expect(
      rememberColumnWidths(memory, 'en-US', { name: 240, id: 80 }, cols),
    ).toEqual({ 'zh-CN': { id: 90 }, 'en-US': { name: 240 } });
  });

  it('writing back also clears missing columns, and removes the language entry when nothing was dragged in it', () => {
    const memory = { 'en-US': { name: 240, gone: 300 } };
    expect(
      rememberColumnWidths(memory, 'en-US', { name: 240, gone: 300 }, cols),
    ).toEqual({ 'en-US': { name: 240 } });
    expect(rememberColumnWidths(memory, 'en-US', {}, cols)).toEqual({});
  });
});
