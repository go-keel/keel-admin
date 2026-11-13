import {
  createFileRoute,
  Outlet,
  redirect,
  useRouterState,
} from '@tanstack/react-router';
import { useState } from 'react';
import { AppHeader } from '@/components/app-header';
import { AppSidebar } from '@/components/app-sidebar';
import { ServerError } from '@/components/exception';
import { SidebarInset, SidebarProvider } from '@/components/ui/sidebar';
import { Skeleton } from '@/components/ui/skeleton';
import { isSessionKilled } from '@/lib/api';
import type { Profile } from '@/lib/perm';
import { profileQueryOptions, systemInfoQueryOptions } from '@/lib/query';
import { canEnter } from '@/lib/route-meta';
import { getSidebarOpen, setSidebarOpen } from '@/lib/sidebar';
import { setBusinessTimeZone } from '@/lib/timezone';
import { reloadIfUpdatePending } from '@/lib/update';
import { cn } from '@/lib/utils';
import { shell } from '@/shell';

/**
 * Authenticated shell: everything behind login hangs under it. Two things are done here so pages need not repeat them:
 *   1. Fetch the profile and system info (business time zone). If the identity does not hold (one of the three
 *      session-invalid reasons) redirect to login with `redirect`; any other failure (server error, offline) is not
 *      "not logged in", so the whole screen shows "Something went wrong" with a retry. Without the profile the menu
 *      cannot be drawn nor permissions checked; this is the only place a failed request may fail the whole screen
 *      (docs/admin/errors.md, "A failed fetch affects only its block");
 *   2. 403 gate: the staticData of every route level the target URL matches is checked (the page level and the sub-view
 *      each check their own code); if it cannot be entered, go to /403, and no loader sends anything.
 */
export const Route = createFileRoute('/_app')({
  beforeLoad: async ({ context, location, matches }) => {
    let profile: Profile;
    try {
      const [p, systemInfo] = await Promise.all([
        context.queryClient.ensureQueryData(profileQueryOptions),
        context.queryClient.ensureQueryData(systemInfoQueryOptions),
      ]);
      profile = p;
      // The business time zone doubles as the last-resort default for the display time zone; calling it again is
      // harmless.
      if (systemInfo.businessTimeZone) {
        setBusinessTimeZone(systemInfo.businessTimeZone);
      }
    } catch (error) {
      if (isSessionKilled(error)) {
        throw redirect({ to: '/login', search: { redirect: location.href } });
      }
      throw error;
    }
    if (
      !canEnter(
        profile,
        matches.map((match) => match.staticData),
      )
    ) {
      throw redirect({ to: '/403' });
    }
    return { profile };
  },
  component: AppShell,
  pendingComponent: ShellSkeleton,
  errorComponent: ServerError,
});

function AppShell() {
  const pathname = useRouterState({ select: (s) => s.location.pathname });
  // Once a new version has been detected, turn this page change into a full page load (see lib/update.ts)
  reloadIfUpdatePending(pathname);
  // Collapse state is controlled: shell preferences all live in localStorage, not the cookie built into SidebarProvider
  // (see lib/sidebar.ts).
  const [sidebarOpen, setOpen] = useState(getSidebarOpen);

  return (
    <SidebarProvider
      open={sidebarOpen}
      onOpenChange={(open) => {
        setOpen(open);
        setSidebarOpen(open);
      }}
      // The style prop documented by shadcn: wire --sidebar-width to a variable that styles/ can override per language
      // (still 16rem by default), without changing components/ui/sidebar.tsx itself.
      style={
        {
          '--sidebar-width': 'var(--keel-sidebar-width, 16rem)',
        } as React.CSSProperties
      }
    >
      <AppSidebar />
      {/* min-w-0: SidebarInset is a flex item; the default `min-width: auto` lets its widest descendant stretch it, so
          a wide table pushes out a horizontal scrollbar for the **whole page** instead of scrolling in its own
          container. */}
      <SidebarInset className="min-w-0">
        <AppHeader />
        <div
          className={cn(
            'p-4 md:p-6',
            shell.layout.contentFullWidth || 'mx-auto w-full max-w-6xl',
          )}
        >
          <Outlet />
        </div>
      </SidebarInset>
    </SidebarProvider>
  );
}

function ShellSkeleton() {
  return (
    <div className="flex flex-col gap-4 p-6">
      <Skeleton className="h-8 w-48" />
      <Skeleton className="h-64 w-full" />
    </div>
  );
}
