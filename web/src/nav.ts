import {
  FolderTreeIcon,
  LayoutGridIcon,
  type LucideIcon,
  MegaphoneIcon,
  NewspaperIcon,
} from 'lucide-react';
import type { FileRouteTypes } from '@/routeTree.gen';

/**
 * Main menu list (assembly seam). A derived repo adds its business menus here without touching the sidebar component.
 *
 * Only paths and icons: titles and permission codes are in the routes' staticData (see lib/route-meta.ts), and the
 * sidebar filters visibility level by level from that staticData, so permission codes are written once. Menu items
 * always show the page name.
 *
 * The kernel owns only two groups, neither here: **My favorites** always first (driven by user settings, not a
 * build-time list) and **System** always last. This list sits between them, rendered in declaration order.
 */
export type NavLeaf = {
  /**
   * Only real route paths are accepted: the type comes from the generated route tree, so a wrong path fails to compile.
   * Uses `to` rather than `fullPaths`: an `index.tsx` under a directory carries a trailing slash in the latter
   * (`/xxx/`), while `<Link to>` takes the normalised one (`/xxx`).
   *
   * Points to a page, not to a sub-view inside a page: sub-views are reached from the page's own tab bar.
   */
  path: FileRouteTypes['to'];
  icon?: LucideIcon;
};

/**
 * A parent used only for grouping: not a page itself; clicking the name only expands or collapses it, the arrow on the
 * right turns with it; collapsed to icons, clicking the icon pops the children out on the right. Nesting stops here,
 * children have no children. Rules and reasons in `docs/admin/pages.md`, "Sidebar menu".
 */
export type NavFolder = {
  /** i18n key of the display name: it has no route, so there is no staticData.titleKey */
  labelKey: string;
  icon?: LucideIcon;
  /** Each child is a separate page; when the user has permission for none of them, the parent does not appear at all */
  children: NavLeaf[];
};

/** Page items cannot have children: only grouping parents have children, so the old form (a page with children) fails
 * to compile. */
export type NavItem = (NavLeaf & { children?: never }) | NavFolder;

export type NavGroup = {
  /** i18n key of the group label; without it the group shows no label (flat) */
  labelKey?: string;
  /** Clicking the label collapses the whole group. A group without a label cannot collapse (no handle to click), so
   * this is ignored */
  collapsible?: boolean;
  items: NavItem[];
};

export const nav: NavGroup[] = [
  // The workbench is a business menu too: the landing page `/` belongs to the derived repo, so its menu entry has no
  // reason to belong to the kernel. The group has no label: one entry needs no heading.
  //
  // The landing page has one kernel rule: the logo and the error pages' "Back to home" all point to `/`, so it gets no
  // favorites star; the logo is its permanent entry. This follows the path, not the group it is in.
  {
    items: [{ path: '/', icon: LayoutGridIcon }],
  },
  // Example: delete after forking (the whole group) A business group with a label. A group without labelKey is flat and
  // shows no name. Groups are not collapsible by default, like the kernel's Favorites and System groups; when the menu
  // grows long enough, write `collapsible: true` (the label is the handle; a group without a label cannot collapse).
  //
  // The group holds one grouping parent whose two children are separate pages.
  {
    labelKey: 'app:nav.ops',
    items: [
      {
        labelKey: 'app:nav.notices',
        icon: MegaphoneIcon,
        children: [
          { path: '/notices/categories', icon: FolderTreeIcon },
          { path: '/notices', icon: NewspaperIcon },
        ],
      },
    ],
  },
];
