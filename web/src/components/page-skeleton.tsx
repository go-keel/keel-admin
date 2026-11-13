import { Skeleton } from '@/components/ui/skeleton';

/**
 * Content-area skeleton during page changes. **It covers only the content area; the shell stays as is.** A route's
 * pending state does not bubble up to the parent route by default, so a page-level one must be given here; otherwise on
 * a page change the old page freezes in place until the data arrives and is replaced in one piece, which looks like a
 * full page reload.
 *
 * Mounted as the router's defaultPendingComponent (see main.tsx), so new pages need not write one each; a page with a
 * skeleton that fits its shape better overrides it with pendingComponent on its own route.
 */
export function PageSkeleton() {
  return (
    <div className="flex flex-col gap-4">
      <Skeleton className="h-7 w-40" />
      <Skeleton className="h-20 w-full" />
      <Skeleton className="h-64 w-full" />
    </div>
  );
}
