// Example file: delete the whole file after forking

import type { NoticeCategoryOption } from './-api';

/**
 * Page-private adapter logic, kept in place (the `-` prefix keeps it out of the route tree). A live example of how a
 * business area is written: the little logic that can go wrong silently moves out of JSX, and unit tests cover only it.
 */

/**
 * List rows carry only `categoryId`; the category name is looked up in the options. When not found it **falls back to
 * the id** instead of a blank: if the category was deleted or is outside my grant, a blank suggests the notice has
 * none.
 */
export const categoryName = (
  categoryId: string | undefined,
  options: readonly NoticeCategoryOption[],
): string => {
  if (!categoryId || categoryId === '0') return '-';
  return options.find((o) => o.id === categoryId)?.name ?? `#${categoryId}`;
};

/** Content excerpt in the list: truncated when it does not fit one line; the full content is in details/edit. */
export const excerpt = (content: string | undefined, max = 40): string => {
  const text = (content ?? '').replace(/\s+/g, ' ').trim();
  if (text === '') return '-';
  return text.length <= max ? text : `${text.slice(0, max)}…`;
};

/**
 * Rows → CSV text. There are only three escaping rules, but getting any wrong raises no error: Excel just reads one
 * column as two, or a quoted title as half a sentence. Exactly the "goes wrong silently" kind of logic, hence the
 * tests.
 *
 * Line breaks are always CRLF: RFC 4180 says so, and Excel's tolerance of LF varies.
 */
export const toCsv = (rows: readonly (readonly unknown[])[]): string =>
  rows.map((cells) => cells.map(csvCell).join(',')).join('\r\n');

/** If a comma, double quote or line break appears, quote the whole cell and double the quotes inside it. */
export const csvCell = (v: unknown): string => {
  const s = v == null ? '' : String(v);
  return /[",\r\n]/.test(s) ? `"${s.replaceAll('"', '""')}"` : s;
};
