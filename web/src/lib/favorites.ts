import { api, unwrap } from './api';
import type { Profile } from './perm';
import { profileQueryOptions, queryClient } from './query';
import { canEnter, type RouteLayers } from './route-meta';

/**
 * Favorite pages: a **user setting** that follows the account (defined in GLOSSARY.md, "The user's two kinds of
 * choices"), stored on the server, delivered with the profile, written per item: favorite one item in each of two
 * browsers and both are kept.
 */

/**
 * The landing page cannot be favorited: the logo and "back to home" on the exception pages both point to it, the logo
 * is its permanent entry, and a favorite would only duplicate it. This rule follows "`/` is the landing page". Every
 * other page with a title can be favorited, whether or not it is in the menu.
 */
export const canFavorite = (path: string): boolean => path !== '/';

/** An empty proto3 message is absent entirely; normalizing to an array happens only here. The order is the favorites
 * order. */
export const favoritePaths = (profile: Profile | undefined): string[] =>
  profile?.settings?.favoriteMenus ?? [];

/**
 * Filtering before render: items whose route was deleted (layersOf returns undefined) or that lack permission are not
 * shown.
 *
 * This step is required, not a safety net: the server deliberately does not check whether a favorited path exists
 * (routes are the frontend's business, and a server-side whitelist that drifts would only block newly launched pages),
 * so after routes change or permissions are revoked, the sidebar would keep dead links that cannot be opened.
 */
export const visibleFavorites = (
  paths: readonly string[],
  profile: Profile | undefined,
  layersOf: (path: string) => RouteLayers | undefined,
): string[] =>
  paths.filter((path) => {
    const layers = layersOf(path);
    return layers !== undefined && canEnter(profile, layers);
  });

/** The full settings returned by the server overwrite the local cache directly; results are not computed on the
 * frontend. */
const writeBack = (settings: { favoriteMenus?: string[] }): void => {
  queryClient.setQueryData(profileQueryOptions.queryKey, (prev) =>
    prev ? { ...prev, settings } : prev,
  );
};

/**
 * Favorite / unfavorite. The server returns the full list after the operation, which overwrites the local cache
 * directly; results are not computed on the frontend, since two browser tabs would each compute their own. Both
 * endpoints are idempotent, so repeated clicks are safe.
 */
export async function toggleFavorite(
  path: string,
  favorited: boolean,
): Promise<void> {
  const settings = favorited
    ? unwrap(
        await api.POST('/v1/sys/auth/favorites/remove', { body: { path } }),
      )
    : unwrap(await api.POST('/v1/sys/auth/favorites/add', { body: { path } }));
  writeBack(settings);
}

/**
 * Drag to reorder. **A whole-order overwrite**, unlike per-item add and remove: when dragging, the user really is
 * saying "this is the whole order" (see ADR admin-0011). The server accepts only a permutation of the current set:
 * using it to add a favorite is rejected, and an item just added in another browser and missing from this list stays at
 * the end.
 */
export async function reorderFavorites(paths: string[]): Promise<void> {
  const settings = unwrap(
    await api.POST('/v1/sys/auth/favorites/reorder', { body: { paths } }),
  );
  writeBack(settings);
}
