import {
  Building2Icon,
  LogInIcon,
  type LucideIcon,
  ScrollTextIcon,
  ShieldIcon,
  StarIcon,
  UsersIcon,
} from 'lucide-react';
import type { NavGroup, NavLeaf } from '@/nav';
import { visibleFavorites } from './favorites';
import type { Profile } from './perm';
import { canEnter, pageTitleKey, type RouteLayers } from './route-meta';

/**
 * Every rule for what the sidebar shows, computed and then handed to the component to draw; the component has no
 * filter/map. It computes and does not draw, so it can be unit-tested directly (this repo has no component rendering
 * tests, and a rule living in JSX is a rule without coverage).
 *
 * The kernel owns only two groups: **My Favorites** always first (driven by user settings, not a build-time list) and
 * **System** always last (overwritten as a whole on kernel upgrades). Everything in between is the `nav.ts` list, in
 * declaration order; the workbench is among them too, since it is a business page (the derived repo decides what its
 * landing page shows) and its menu entry has no reason to belong to the kernel.
 */

/** Key of the favorites group: the component uses it to recognise that this group needs the drag-to-sort container. */
export const FAVORITES_KEY = 'fav';

/**
 * The System group ranks with business groups: it has a label just like them, only its position is always last. It
 * deliberately gets no divider and is not pinned to the bottom: that would read it as a different kind of thing, and it
 * is not. It does not collapse (nor does the favorites group); its label is only a heading. A derived repo enabling
 * `collapsible` on its own groups does not extend that to these two kernel groups.
 */
const SYS_GROUP: NavGroup = {
  labelKey: 'nav.system',
  items: [
    { path: '/sys/users', icon: UsersIcon },
    { path: '/sys/roles', icon: ShieldIcon },
    { path: '/sys/depts', icon: Building2Icon },
    { path: '/sys/audit', icon: ScrollTextIcon },
    { path: '/sys/login-logs', icon: LogInIcon },
  ],
};

/**
 * A page's menu item, ready to draw. When `titleKey` is missing, `path` is the fallback; translation happens in the
 * component.
 */
export type MenuPage = {
  kind: 'page';
  /**
   * Route path. Its type shares the list's source, so the component's `<Link to>` needs no further cast; favorites are
   * arbitrary strings from the server, and the only cast is below, where they are taken in.
   */
  path: NavLeaf['path'];
  titleKey?: string;
  icon?: LucideIcon;
  /** Whether this is the current page (being on one of its sub-views counts too) */
  active: boolean;
};

/** Folder parent: not a page itself, only expands and collapses. */
export type MenuFolder = {
  kind: 'folder';
  labelKey: string;
  icon?: LucideIcon;
  /**
   * Whether the user is currently on one of its children. The sidebar uses it to expand the group on entry and not to
   * collapse it on leaving; any other opening and closing is up to the user and is not remembered.
   */
  inside: boolean;
  /** Permitted children, at least one; a parent with none is absent */
  children: MenuPage[];
};

export type MenuItem = MenuPage | MenuFolder;

export type MenuGroup = {
  key: string;
  /** Omitted = this group shows no group name (flat) */
  labelKey?: string;
  /** Collapsing needs a clickable handle, and the handle is the label: a group without a label cannot collapse */
  collapsible: boolean;
  items: MenuItem[];
};

export type MenuContext = {
  profile: Profile | undefined;
  /** Route metadata layers by URL, from routeLayersOf in lib/route-meta.ts */
  layersOf: (path: string) => RouteLayers | undefined;
  /**
   * URL of the current page, from currentPageOf in lib/route-meta.ts: on a sub-view it is the URL of the page it belongs
   * to, so the menu still marks that page.
   */
  currentPage: string | undefined;
  /** Raw favorites list (unfiltered, in favorites order) */
  favorites: readonly string[];
};

/**
 * Computes the groups the sidebar shows. `groups` is passed in by the caller (the component passes the `nav.ts` list,
 * tests pass their own), and this function appends the kernel's System group last.
 *
 * Filtering does only three things: pages without permission are absent, parents whose children are all gone are
 * absent, and groups left empty are absent (an empty heading taking up space is only noise).
 */
export function buildMenu(
  groups: readonly NavGroup[],
  ctx: MenuContext,
): MenuGroup[] {
  const all = [...groups, SYS_GROUP];
  // A favorite reuses its original menu icon: collapsed to icons only, a row of identical stars could not be told
  // apart. Items with no icon in the list (a derived repo may omit it) fall back to the star.
  const icons = new Map<string, LucideIcon>();
  for (const group of all) {
    for (const item of group.items) {
      for (const leaf of 'labelKey' in item ? item.children : [item]) {
        if (leaf.icon) icons.set(leaf.path, leaf.icon);
      }
    }
  }

  const out: MenuGroup[] = [];
  const favorites = visibleFavorites(ctx.favorites, ctx.profile, ctx.layersOf);
  if (favorites.length > 0) {
    out.push({
      key: FAVORITES_KEY,
      labelKey: 'nav.favorites',
      collapsible: false,
      items: favorites.map((path) =>
        // Items with no icon in the list fall back to the star: the favorites group needs some icon, and in icon mode
        // it is all that is left.
        toPage(
          { path: path as NavLeaf['path'], icon: icons.get(path) ?? StarIcon },
          ctx,
        ),
      ),
    });
  }
  all.forEach((group, i) => {
    const items = group.items.flatMap((item): MenuItem[] => {
      if (!('labelKey' in item)) {
        return canEnter(ctx.profile, ctx.layersOf(item.path) ?? [])
          ? [toPage(item, ctx)]
          : [];
      }
      // A parent has no permission of its own: it is present when any child is accessible.
      const children = item.children
        .filter((child) =>
          canEnter(ctx.profile, ctx.layersOf(child.path) ?? []),
        )
        .map((child) => toPage(child, ctx));
      if (children.length === 0) return [];
      return [
        {
          kind: 'folder',
          labelKey: item.labelKey,
          icon: item.icon,
          inside: children.some((child) => child.active),
          children,
        },
      ];
    });
    if (items.length === 0) return;
    out.push({
      key: `nav-${i}`,
      labelKey: group.labelKey,
      collapsible: group.collapsible ?? false,
      items,
    });
  });
  return out;
}

/** Menu items always show the page name: the list holds only pages, never sub-views. */
const toPage = (leaf: NavLeaf, ctx: MenuContext): MenuPage => ({
  kind: 'page',
  path: leaf.path,
  titleKey: pageTitleKey(ctx.layersOf(leaf.path) ?? []),
  icon: leaf.icon,
  active: ctx.currentPage === leaf.path,
});
