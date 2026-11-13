import { QueryClientProvider } from '@tanstack/react-query';
import { createRouter, RouterProvider } from '@tanstack/react-router';
import { StrictMode } from 'react';
import { createRoot } from 'react-dom/client';
import { NotFound, ServerError } from '@/components/exception';
import { PageSkeleton } from '@/components/page-skeleton';
import { TooltipProvider } from '@/components/ui/tooltip';
import { setSessionKilledHandler } from '@/lib/api';
import { resolveBrand } from '@/lib/brand';
import '@/lib/i18n';
import '@/timezones';
import { queryClient } from '@/lib/query';
import { initTheme } from '@/lib/theme';
import '@/styles/index.css';
import { routeTree } from './routeTree.gen';

/**
 * Entry assembly (assembly seam). The order matters:
 *   1. initTheme: write <html class="dark"> and data-theme-preset before the first frame, otherwise a light flash
 *      appears first;
 *   2. resolveBrand: fully synchronous in embedded deployment (title already injected into window.__BRAND__), only dev
 *      actually sends a request;
 *   3. only then render.
 */
initTheme();

const router = createRouter({
  routeTree,
  context: { queryClient },
  defaultErrorComponent: ServerError,
  defaultNotFoundComponent: NotFound,
  // Show a skeleton in the content area while changing pages. A route's pending state **does not bubble up** to the
  // parent route's pendingComponent; without this default the page level gives no feedback at all: the URL changes at
  // once, the old page freezes in place and is replaced in one go once the chunk and loader return, which looks like a
  // full reload.
  defaultPendingComponent: PageSkeleton,
  // Changes done within 200ms show no skeleton (a flash is more jarring); once shown it stays at least 400ms to avoid
  // flicker.
  defaultPendingMs: 200,
  defaultPendingMinMs: 400,
});

declare module '@tanstack/react-router' {
  interface Register {
    router: typeof router;
  }
}

// On session invalidation navigate through the router instead of a full reload, saving a blank screen.
setSessionKilledHandler((redirect) => {
  void router.navigate({ to: '/login', search: { redirect }, replace: true });
});

await resolveBrand();

createRoot(document.getElementById('root')!).render(
  <StrictMode>
    <QueryClientProvider client={queryClient}>
      <TooltipProvider>
        <RouterProvider router={router} />
      </TooltipProvider>
    </QueryClientProvider>
  </StrictMode>,
);
