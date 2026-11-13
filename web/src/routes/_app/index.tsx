import { createFileRoute, Link, useRouter } from '@tanstack/react-router';
import {
  Building2Icon,
  FolderTreeIcon,
  type LucideIcon,
  NewspaperIcon,
  ScrollTextIcon,
  ShieldIcon,
  UsersIcon,
} from 'lucide-react';
import { useTranslation } from 'react-i18next';
import { Card, CardHeader, CardTitle } from '@/components/ui/card';
import { useProfile } from '@/hooks/use-profile';
import { tDynamic } from '@/lib/i18n';
import { canEnter, pageTitleKey, routeLayersOf } from '@/lib/route-meta';
import type { NavLeaf } from '@/nav';

export const Route = createFileRoute('/_app/')({
  staticData: { titleKey: 'app:workbench.title' },
  component: HomePage,
});

/**
 * Shortcuts on the workbench, in display order. Only paths and icons: the title and the permission check come from the
 * route's staticData, like the sidebar, so an entry the account cannot open is not shown.
 */
const shortcuts: { path: NavLeaf['path']; icon: LucideIcon }[] = [
  { path: '/notices', icon: NewspaperIcon }, // Example: delete after forking
  { path: '/notices/categories', icon: FolderTreeIcon }, // Example: delete after forking
  { path: '/sys/users', icon: UsersIcon },
  { path: '/sys/roles', icon: ShieldIcon },
  { path: '/sys/depts', icon: Building2Icon },
  { path: '/sys/audit', icon: ScrollTextIcon },
];

/** Landing page (`/`): a greeting with the current account's name and the shortcuts it may open. */
function HomePage() {
  const { t } = useTranslation('app');
  const profile = useProfile();
  const router = useRouter();
  const name = profile.nickname || profile.username || '';
  const entries = shortcuts.flatMap(({ path, icon }) => {
    const layers = routeLayersOf(router, path) ?? [];
    return canEnter(profile, layers)
      ? [{ path, icon, title: tDynamic(pageTitleKey(layers), path) }]
      : [];
  });

  return (
    <div className="flex flex-col gap-6">
      <h1 className="text-3xl tracking-tight">
        {t('workbench.greeting', { name })}
      </h1>

      {entries.length > 0 && (
        <section className="flex flex-col gap-3">
          <h2 className="font-medium text-muted-foreground text-sm">
            {t('workbench.shortcuts')}
          </h2>
          <div className="grid grid-cols-1 gap-4 sm:grid-cols-2 lg:grid-cols-3 xl:grid-cols-4">
            {entries.map(({ path, icon: Icon, title }) => (
              <Link key={path} to={path} className="rounded-xl">
                <Card className="transition-colors hover:bg-accent">
                  <CardHeader>
                    <CardTitle className="flex items-center gap-2">
                      <Icon className="size-4 text-muted-foreground" />
                      {title}
                    </CardTitle>
                  </CardHeader>
                </Card>
              </Link>
            ))}
          </div>
        </section>
      )}
    </div>
  );
}
