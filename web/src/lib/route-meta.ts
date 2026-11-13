import type { AnyRouter } from '@tanstack/react-router';
import type { PermCode } from '@/generated/perm-code';
import { has, type Profile } from './perm';

/**
 * Route metadata lives in staticData: the permission code and the title are each written once, and menu visibility,
 * the tab bar, the page header title and the 403 guard all read from here. The menu carries no permission codes of its
 * own: that would be a second copy, and two copies always drift.
 */
declare module '@tanstack/react-router' {
  interface StaticDataRouteOption {
    /** i18n key (with the namespace prefix, e.g. 'app:workbench.title'), the title and menu text */
    titleKey?: string;
    /** Permission code, from the same source as the backend perm registry */
    perm?: PermCode;
    /** root-only route (RootOnly in the backend Bindings, no permission code applies) */
    root?: boolean;
  }
}

export type RouteMeta = {
  titleKey?: string;
  perm?: PermCode;
  root?: boolean;
};

/**
 * Metadata of every route layer a URL matches, outermost first. On a page with sub-views, the page and the sub-view
 * each take a layer, and the default sub-view shares the page's URL, so a URL yields a list, not a single entry.
 */
export type RouteLayers = readonly RouteMeta[];

/**
 * Whether it can be entered: every layer must pass. root bypasses all permission codes (matching the backend's
 * perm.Principal: root's perms is an empty array meaning "do not look at this array", not "has nothing").
 * A layer with neither perm nor root is public.
 */
export const canEnter = (
  profile: Profile | undefined,
  layers: RouteLayers,
): boolean =>
  !!profile?.isRoot ||
  layers.every((meta) => !meta.root && has(profile, meta.perm));

/** Page name: the outermost layer with a title. The top bar, sidebar menu and favorites show it. */
export const pageTitleKey = (layers: RouteLayers): string | undefined =>
  layers.find((meta) => meta.titleKey)?.titleKey;

/**
 * Sub-view name: the innermost layer with a title. The tab bar shows it; for a page without layers it equals the page
 * name.
 */
export const viewTitleKey = (layers: RouteLayers): string | undefined =>
  layers.findLast((meta) => meta.titleKey)?.titleKey;

/**
 * Current page: the outermost matched layer with a title, its URL and title key. On a sub-view it gives the page the
 * sub-view belongs to (the top bar's page name and the favorites star follow it); a page without a title gives
 * undefined.
 *
 * The URL drops the trailing slash: when a directory's `index.tsx` is itself the page, its layer's URL has one
 * (`/xxx/`), while the menu and favorites write `/xxx`.
 */
export const currentPageOf = (
  matches: readonly { pathname: string; staticData: RouteMeta }[],
): { path: string; titleKey: string } | undefined => {
  const page = matches.find((match) => match.staticData.titleKey);
  return page?.staticData.titleKey
    ? {
        path: page.pathname.replace(/(.)\/$/, '$1'),
        titleKey: page.staticData.titleKey,
      }
    : undefined;
};

/**
 * The metadata layers a URL matches. `undefined` means exactly **no such route**: favorites filtering relies on it to
 * tell "route deleted" from "route exists but has no staticData (= public)".
 *
 * Uses `matchRoutes` rather than `routesByPath`: the latter's keys drop the trailing slash, so the default sub-view
 * (the index route) takes the page layer's key and a URL yields only one layer. When a URL has no matching route,
 * `matchRoutes` still returns the outer layers that match, so the innermost full path does not match, and that is how
 * "none" is detected.
 */
export const routeLayersOf = (
  router: AnyRouter,
  path: string,
): RouteLayers | undefined => {
  const matches = router.matchRoutes(path);
  const fullPath = matches.at(-1)?.fullPath;
  if (fullPath !== path && fullPath !== `${path}/`) return undefined;
  return matches.map((match) => match.staticData);
};
