// Example file: delete the whole file after forking

import { describe, expect, it } from 'vitest';
import { categoryName, csvCell, excerpt, toCsv } from './-logic';

const options = [
  { id: '1', name: 'Company news' },
  { id: '2', name: 'Tech weekly' },
];

describe('categoryName', () => {
  it('shows the name when it is found', () => {
    expect(categoryName('2', options)).toBe('Tech weekly');
  });

  it('falls back to the id instead of leaving it blank when not found', () => {
    // The category was deleted, or is outside my grant: a blank would suggest the notice has no category.
    expect(categoryName('99', options)).toBe('#99');
  });

  it('gives a placeholder when there is no category', () => {
    expect(categoryName(undefined, options)).toBe('-');
    expect(categoryName('0', options)).toBe('-');
  });
});

describe('excerpt', () => {
  it('keeps short content as is', () => {
    expect(excerpt('One sentence')).toBe('One sentence');
  });

  it('truncates long content and appends an ellipsis', () => {
    expect(excerpt('x'.repeat(50), 10)).toBe(`${'x'.repeat(10)}…`);
  });

  it('collapses whitespace and gives a placeholder for empty content', () => {
    expect(excerpt('  multi\n  line  ')).toBe('multi line');
    expect(excerpt('')).toBe('-');
    expect(excerpt(undefined)).toBe('-');
  });
});

describe('csvCell', () => {
  it('does not quote plain values', () => {
    expect(csvCell('Company news')).toBe('Company news');
    expect(csvCell(42)).toBe('42');
  });

  it('turns empty values into an empty cell', () => {
    expect(csvCell(null)).toBe('');
    expect(csvCell(undefined)).toBe('');
  });

  it('quotes commas, quotes and newlines, doubling the quotes', () => {
    expect(csvCell('a,b')).toBe('"a,b"');
    expect(csvCell('say "hi"')).toBe('"say ""hi"""');
    expect(csvCell('line 1\nline 2')).toBe('"line 1\nline 2"');
  });
});

describe('toCsv', () => {
  it('separates rows with CRLF and cells with commas', () => {
    expect(
      toCsv([
        ['ID', 'Title'],
        ['1', 'Notice'],
      ]),
    ).toBe('ID,Title\r\n1,Notice');
  });

  it('content with an ASCII comma does not split one column into two', () => {
    expect(toCsv([['1', 'a,b']])).toBe('1,"a,b"');
  });

  it('a full-width comma is not a separator and needs no escaping', () => {
    expect(toCsv([['1', '通知，含全角逗号']])).toBe('1,通知，含全角逗号');
  });
});
