import type { ReactNode } from 'react';
import { useTranslation } from 'react-i18next';
import { Button } from '@/components/ui/button';
import { Skeleton } from '@/components/ui/skeleton';
import { loadState, type QueryLike, retryFailed } from '@/lib/load-state';
import { cn } from '@/lib/utils';

/**
 * The "failed to load" block: one sentence plus retry. Data regions, tables, form dialogs and filter dropdowns all draw
 * it on failure. The reason is not written here: the request layer has already toasted it, and repeating it would be a
 * second copy.
 */
export function LoadFailed({
  onRetry,
  className,
}: {
  onRetry: () => void;
  className?: string;
}) {
  const { t } = useTranslation();
  return (
    <div
      className={cn(
        'flex flex-col items-center justify-center gap-2 py-6 text-center text-muted-foreground text-sm',
        className,
      )}
    >
      <p>{t('error.loadFailed')}</p>
      <Button variant="outline" size="sm" onClick={onRetry}>
        {t('error.retry')}
      </Button>
    </div>
  );
}

function DefaultSkeleton() {
  return (
    <div className="flex flex-col gap-3">
      <Skeleton className="h-8 w-full" />
      <Skeleton className="h-8 w-full" />
      <Skeleton className="h-8 w-2/3" />
    </div>
  );
}

/**
 * Data region: the single entry point for a page to get data (tables and form dialogs build in the same logic, so they
 * need no wrapping). Data is handed out only in `children`, so the frame (page header, buttons, filter bar) is drawn
 * outside it, touches no data and is unaffected by request success or failure; when data cannot be fetched, only this
 * block shows "failed to load" and retry, never an empty state.
 *
 * The logic is in `lib/load-state.ts`: with data in hand, a failed background refetch keeps showing the old data.
 */
export function DataRegion<T>({
  query,
  requires = [],
  skeleton,
  className,
  children,
}: {
  query: QueryLike & { data?: T };
  /** Other data this block waits for (such as the permission catalog used to show code names); it draws only when all
   * arrive, and any failure counts as failure. */
  requires?: readonly QueryLike[];
  /** Skeleton for the first fetch. Omitted, a generic three-line skeleton is used; pass one that fits the shape better
   * when there is one. */
  skeleton?: ReactNode;
  /** Outer class of the failed block, so it occupies the same space as the data would. */
  className?: string;
  children: (data: T) => ReactNode;
}) {
  const all = [query, ...requires];
  const state = loadState(all);
  if (state === 'failed') {
    return (
      <LoadFailed className={className} onRetry={() => retryFailed(all)} />
    );
  }
  if (state === 'loading' || query.data === undefined) {
    return skeleton ?? <DefaultSkeleton />;
  }
  return children(query.data);
}
