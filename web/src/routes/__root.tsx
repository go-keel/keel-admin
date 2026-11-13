import type { QueryClient } from '@tanstack/react-query';
import {
  createRootRouteWithContext,
  Outlet,
  useRouterState,
} from '@tanstack/react-router';
import { useEffect } from 'react';
import { useTranslation } from 'react-i18next';
import { NotFound, ServerError } from '@/components/exception';
import { OfflineBanner } from '@/components/offline-banner';
import { Toaster } from '@/components/ui/sonner';
import { UpdateChecker } from '@/components/update-checker';
import { brandTitle } from '@/lib/brand';
import { tDynamic } from '@/lib/i18n';

/** Root layout. The only site-wide resident widgets live here: the toast container, offline banner, update check. */
export const Route = createRootRouteWithContext<{ queryClient: QueryClient }>()(
  {
    component: RootLayout,
    notFoundComponent: NotFound,
    errorComponent: ServerError,
  },
);

function RootLayout() {
  return (
    <>
      <DocumentTitle />
      <Outlet />
      <Toaster position="top-center" richColors />
      <OfflineBanner />
      <UpdateChecker />
    </>
  );
}

/**
 * Browser tab title: the titles of each level joined from inner to outer, then the brand name (`Categories · Articles ·
 * Brand`; a page without levels is `Current page · Brand`). Pages without a title (such as the login page) show only
 * the brand name. Titles are still written only in the routes' staticData, the same single source as the menu and page
 * header. The router's `head`/`HeadContent` is deliberately not used: it would declare the title again on every route,
 * a second copy that drifts.
 */
function DocumentTitle() {
  // Subscribe to language changes: the title uses tDynamic with dynamic keys, so t's static dependency does not track
  // it.
  useTranslation();
  const page = useRouterState({
    select: (s) =>
      s.matches
        .filter((m) => m.staticData.titleKey)
        .map((m) => tDynamic(m.staticData.titleKey ?? '', m.pathname))
        .reverse()
        .join(' · '),
  });
  useEffect(() => {
    document.title = page ? `${page} · ${brandTitle()}` : brandTitle();
  }, [page]);
  return null;
}
