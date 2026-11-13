import type { Profile } from './perm';
import { canEnter, type RouteLayers, viewTitleKey } from './route-meta';

/**
 * The tab bar of directly navigable sub-views: in list order, the sub-views this user can enter and their names.
 * Permission is checked level by level and the name comes from the sub-view level, the same logic as the sidebar and
 * the 403 guard. It only computes, never draws: the tabs are drawn by the page itself the shadcn way (the kernel wraps
 * no controls), and the current one is found by comparing URLs.
 */
export type SubViewTab<P extends string> = { path: P; titleKey?: string };

export function subViewTabs<P extends string>(
  paths: readonly P[],
  profile: Profile | undefined,
  layersOf: (path: string) => RouteLayers | undefined,
): SubViewTab<P>[] {
  return paths.flatMap((path) => {
    const layers = layersOf(path);
    return layers && canEnter(profile, layers)
      ? [{ path, titleKey: viewTitleKey(layers) }]
      : [];
  });
}
