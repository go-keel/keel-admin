/**
 * What a data block should draw: data, a skeleton or "failed to load". A failed request affects only the block that
 * uses that data (docs/admin/errors.md, "A failed fetch affects only its block"); this is that block's logic, used by
 * data regions, tables, form dialogs and filter dropdowns.
 *
 * The input needs only these fields of a TanStack Query result; pass the return value of `useQuery` directly.
 */
export type QueryLike = {
  data?: unknown;
  isError: boolean;
  isPending: boolean;
  fetchStatus: 'fetching' | 'paused' | 'idle';
  refetch: () => unknown;
};

export type LoadState = 'ready' | 'loading' | 'failed';

/**
 * State of a block made of several pieces of data: any failure means failed, any in flight means loading, otherwise
 * ready.
 *
 * - **Data in hand means ready**, even if a background refetch failed: what is already shown is not replaced because of
 *   a later failed request; the request layer has already shown the message.
 * - **A disabled query does not hold the block back** (with `enabled: false` it stays pending but never fetches):
 *   conditional data (options fetched only with some permission, details fetched only when editing) counts as not
 *   needed when disabled.
 */
export function loadState(queries: readonly QueryLike[]): LoadState {
  let loading = false;
  for (const q of queries) {
    if (q.data !== undefined) continue;
    if (q.isError) return 'failed';
    if (q.isPending && q.fetchStatus !== 'idle') loading = true;
  }
  return loading ? 'loading' : 'ready';
}

/** Retry refetches only the failed pieces; what has arrived is not requested again. */
export function retryFailed(queries: readonly QueryLike[]): void {
  for (const q of queries) {
    if (q.data === undefined && q.isError) void q.refetch();
  }
}
