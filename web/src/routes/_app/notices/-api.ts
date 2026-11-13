// Example file: delete the whole file after forking

import { queryOptions } from '@tanstack/react-query';
import type { components } from '@/generated/openapi';
import { api, unwrap } from '@/lib/api';
import { defineList, type ListSearch } from '@/lib/list';
import { queryClient } from '@/lib/query';
import { PAGE_SIZES } from '@/lib/table';
import { toCsv } from './-logic';

/**
 * API wrappers for the notice domain. **Kept in the business area** (next to the page, the `-` prefix keeps it out of
 * the route tree), not in the kernel's `lib/`: one file per domain holds for business areas too, only the location is
 * the derived repo's.
 */
export type NoticeListItem =
  components['schemas']['api.keel_admin.v1.NoticeListItem'];
export type NoticeCategoryListItem =
  components['schemas']['api.keel_admin.v1.NoticeCategoryListItem'];
export type NoticeCategoryOption =
  components['schemas']['api.keel_admin.v1.NoticeCategoryOption'];

/**
 * The notice list definition: keyword and categoryId are both text (categoryId is an ID). `created` is the example of a
 * **fixed** date range: the user picks business days, the request sends `createdFromDate`/`createdToDate`, and the
 * server converts them by the business time zone.
 */
export const noticeList = defineList({
  queryKey: ['app', 'notices'],
  fields: {
    keyword: { type: 'text' },
    categoryId: { type: 'text' },
    created: { type: 'dateRange', zone: 'business' },
  },
  fetch: async (params) =>
    unwrap(await api.GET('/v1/notices', { params: { query: params } })),
  getRowId: (row: NoticeListItem) => row.id ?? '',
});

/**
 * Category options: used by the dropdown and by the category-name mapping of list rows; not paged. The category
 * **management** list below uses the same data for another purpose, hence two endpoints: merging them is bound to break
 * one side (see admin-0008).
 */
export const categoryOptionsQueryOptions = queryOptions({
  queryKey: ['app', 'notice-category-options'],
  queryFn: async () => unwrap(await api.GET('/v1/notices/categories/options')),
});

/** Category management list definition: no filters, empty fields, so no FilterBar is rendered. */
export const noticeCategoryList = defineList({
  queryKey: ['app', 'notice-categories'],
  fields: {},
  fetch: async (params) =>
    unwrap(
      await api.GET('/v1/notices/categories', { params: { query: params } }),
    ),
  getRowId: (row: NoticeCategoryListItem) => row.id ?? '',
});

export const invalidateNotices = () =>
  queryClient.invalidateQueries({ queryKey: ['app', 'notices'] });

export const invalidateCategories = () =>
  queryClient.invalidateQueries({ queryKey: ['app'] });

export async function createNotice(v: {
  title: string;
  content: string;
  categoryId: string;
}): Promise<void> {
  // The owning department and the creator are filled in by the server from the operator; the request body has neither.
  unwrap(await api.POST('/v1/notices', { body: v }));
  await invalidateNotices();
}

/** Deleting a notice takes no code: it neither raises privileges nor touches other accounts, unlike user/role
 * writes. */
export async function deleteNotice(id: string): Promise<void> {
  unwrap(await api.DELETE('/v1/notices/{id}', { params: { path: { id } } }));
  await invalidateNotices();
}

/**
 * Batch delete: one request to BatchDeleteNotice, **not** deleteNotice in a loop. The server succeeds or fails as a
 * whole and the audit keeps one record: "one batch delete" is one thing.
 */
export async function batchDeleteNotices(ids: string[]): Promise<void> {
  unwrap(await api.POST('/v1/notices/batch-delete', { body: { ids } }));
  await invalidateNotices();
}

/**
 * Export all notices under the current filters as CSV. **A business-area demo**: none of the five kernel sys pages
 * opens this door. Exporting the user table is bulk retrieval of personal data; it belongs in a server endpoint with
 * audit and data scope, not in text stitched together by the frontend.
 *
 * Fetching in batches because the server's limit cap is the largest of PAGE_SIZES: fetching everything at once is
 * rejected by middleware, so the batch size takes that option set's maximum rather than a hard-coded number. Batches
 * run sequentially; an export is not worth racing for a few hundred milliseconds. The current filters come from
 * `useList` (the validated URL search, see the `search` return value of `hooks/use-list.ts`); no need to rebuild them.
 */
export async function exportNotices(
  search: ListSearch,
  // Values come from functions rather than field names: the exported "Category" must be the name, not the bare
  // categoryId.
  columns: { label: string; value: (row: NoticeListItem) => unknown }[],
): Promise<void> {
  const BATCH = Math.max(...PAGE_SIZES);
  const rows: NoticeListItem[] = [];
  for (let page = 1; ; page++) {
    const data = await queryClient.fetchQuery(
      noticeList.queryOptions(search, page, BATCH),
    );
    const items = data.items ?? [];
    rows.push(...items);
    if (items.length < BATCH) break;
  }

  const csv = toCsv([
    columns.map((c) => c.label),
    ...rows.map((r) => columns.map((c) => c.value(r))),
  ]);

  // BOM: without it Excel reads UTF-8 in the local code page and all non-ASCII text turns into mojibake.
  download(
    `notices-${new Date().toISOString().slice(0, 10)}.csv`,
    `\uFEFF${csv}`,
  );
}

function download(filename: string, text: string): void {
  const url = URL.createObjectURL(
    new Blob([text], { type: 'text/csv;charset=utf-8' }),
  );
  const a = document.createElement('a');
  a.href = url;
  a.download = filename;
  a.click();
  URL.revokeObjectURL(url);
}

export async function createCategory(name: string): Promise<void> {
  unwrap(await api.POST('/v1/notices/categories', { body: { name } }));
  await invalidateCategories();
}

export async function deleteCategory(id: string): Promise<void> {
  unwrap(
    await api.DELETE('/v1/notices/categories/{id}', {
      params: { path: { id } },
    }),
  );
  await invalidateCategories();
}
