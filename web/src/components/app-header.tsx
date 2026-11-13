import { useRouterState } from '@tanstack/react-router';
import { StarIcon } from 'lucide-react';
import { useTranslation } from 'react-i18next';
import { PreviewMenu } from '@/components/preview-menu';
import {
  ColorModeToggle,
  RegionMenu,
  ThemePresetMenu,
} from '@/components/shell-controls';
import { Button } from '@/components/ui/button';
import { Separator } from '@/components/ui/separator';
import { SidebarTrigger } from '@/components/ui/sidebar';
import {
  Tooltip,
  TooltipContent,
  TooltipTrigger,
} from '@/components/ui/tooltip';
import { UserMenu } from '@/components/user-menu';
import { HeaderEnd, HeaderStart } from '@/header';
import { useProfile } from '@/hooks/use-profile';
import { canFavorite, favoritePaths, toggleFavorite } from '@/lib/favorites';
import { tDynamic } from '@/lib/i18n';
import { currentPageOf } from '@/lib/route-meta';
import { cn } from '@/lib/utils';
import { shell } from '@/shell';

export function AppHeader() {
  // Subscribe to language changes: the page name goes through tDynamic with dynamic keys, which t cannot track
  // statically; without subscribing, after a language switch the header would stay in the old language (as with the
  // sidebar).
  useTranslation();
  const page = useRouterState({ select: (s) => currentPageOf(s.matches) });

  return (
    <header
      className={cn(
        'flex h-12 shrink-0 items-center gap-2 border-b px-4 lg:px-6',
        // Translucent with backdrop blur while sticky (as in the reference shadcn admin): content scrolling underneath
        // shows through faintly. When not sticky nothing passes beneath it, so its background is the content area's and
        // needs no extra color.
        shell.layout.stickyHeader &&
          'sticky top-0 z-20 bg-background/50 backdrop-blur-md',
      )}
    >
      {/* The left edge of the collapse button icon aligns with the content area below: the button has its own padding,
          so pull it back a little. */}
      <SidebarTrigger className="-ml-1" />
      {/* The page name is shown only here (the header layout of shadcn dashboard-01); the page header in the content
          area does not repeat it. On a sub-view it shows the page the sub-view belongs to, which is also what the star
          favorites; the tab bar shows the sub-view. */}
      {page ? (
        <>
          {/* The vertical separator has `data-vertical:self-stretch` and would stretch to the full header height; the
              override needs the same variant prefix. */}
          <Separator
            orientation="vertical"
            className="h-4 data-vertical:self-center"
          />
          <h1 className="truncate font-medium text-base">
            {tDynamic(page.titleKey, page.path)}
          </h1>
          {canFavorite(page.path) ? <FavoriteButton path={page.path} /> : null}
        </>
      ) : null}
      {/* Between the page name and the buttons on the right belongs to the derived repo (web/src/header.tsx): left
          slot, flexible space, right slot. */}
      <div className="flex items-center gap-2">
        <HeaderStart />
      </div>
      {/* Order: derived repo's right slot → time zone and language → appearance preset → color mode → permission
          preview → avatar. */}
      <div className="ml-auto flex items-center gap-2">
        <HeaderEnd />
        <RegionMenu />
        <ThemePresetMenu />
        <ColorModeToggle />
        <PreviewMenu />
        <UserMenu />
      </div>
    </header>
  );
}

/** Favorite the current page. Always visible (not on hover), keyboard reachable, present when the sidebar collapses to
 * icons. */
function FavoriteButton({ path }: { path: string }) {
  const { t } = useTranslation();
  const favorited = favoritePaths(useProfile()).includes(path);
  const label = t(favorited ? 'nav.unfavorite' : 'nav.favorite');
  return (
    <Tooltip>
      <TooltipTrigger
        render={
          <Button
            variant="ghost"
            size="icon"
            aria-label={label}
            onClick={() => void toggleFavorite(path, favorited)}
          />
        }
      >
        <StarIcon className={cn(favorited && 'fill-current')} />
      </TooltipTrigger>
      <TooltipContent>{label}</TooltipContent>
    </Tooltip>
  );
}
