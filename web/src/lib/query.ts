import { QueryClient, queryOptions } from '@tanstack/react-query';
import type { components } from '@/generated/openapi';
import { api, unwrap } from './api';
import type { Profile } from './perm';

export const queryClient = new QueryClient({
  defaultOptions: {
    queries: {
      // The request middleware has already toasted the error; retrying would only repeat the same message three times.
      retry: false,
      // Only 1 second, with no longer "do not refetch for a while": when a page uses data again (opening a page,
      // returning to a page already seen, back/forward, switching back to the browser tab) it shows the cached copy
      // first and refetches in the background. The 1 second only catches the route loader's prefetch, so there is no
      // second request between the prefetch returning and the page mounting (useSuspenseQuery's own 1-second floor
      // used to cover this; it moved here when pages switched to useQuery).
      // Data that truly does not change within a session declares that in its own queryOptions.
      staleTime: 1000,
    },
  },
});

/**
 * The signed-in identity and permissions. The authenticated shell's beforeLoad uses it as the gatekeeper, pages use it
 * for element-level checks.
 *
 * Never collected (infinite gcTime): `useProfile` is a suspending read, and once the cache is collected the next read
 * suspends the whole page. Keeping it resident is stated here rather than relying on the sidebar and header happening
 * to read it all the time; logging out still clears it with `queryClient.clear()`.
 */
export const profileQueryOptions = queryOptions({
  queryKey: ['sys', 'profile'],
  queryFn: async (): Promise<Profile> =>
    unwrap(await api.GET('/v1/sys/auth/profile')),
  gcTime: Number.POSITIVE_INFINITY,
  // 'always' instead of the default "refetch when stale": user settings (favorites) come with the profile and can be
  // changed in another browser. Refetching unconditionally when switching back to this browser tab keeps both in sync.
  refetchOnWindowFocus: 'always',
});

/**
 * Login page bootstrap info: whether the CAPTCHA is on, which provider, and the widget's site key.
 * Deployment configuration that does not change within a session, so it never goes stale.
 */
export type LoginMeta =
  components['schemas']['api.keel_admin.v1.LoginMetaReply'];

export const loginMetaQueryOptions = queryOptions({
  queryKey: ['sys', 'login-meta'],
  queryFn: async (): Promise<LoginMeta> =>
    unwrap(await api.GET('/v1/sys/auth/login-meta')),
  staleTime: Number.POSITIVE_INFINITY,
});

/**
 * Read-only system info: the business time zone (the label of fixed filters and "today" use it). Deployment
 * configuration that does not change within a session, so it never goes stale and is never collected: the
 * authenticated shell calls ensureQueryData on every entry, and a collected cache would mean a wasted wait for a
 * request. Only the shell reads it: the shell writes the business time zone into the time zone module, and components
 * read it from there (`useBusinessTimeZone`), not from this cache.
 */
export const systemInfoQueryOptions = queryOptions({
  queryKey: ['sys', 'system-info'],
  queryFn: async () => unwrap(await api.GET('/v1/sys/meta/system-info')),
  staleTime: Number.POSITIVE_INFINITY,
  gcTime: Number.POSITIVE_INFINITY,
});

/** Role options (unpaged). Shared by the reduced-permission preview's role picker and future filters. */
export const roleOptionsQueryOptions = queryOptions({
  queryKey: ['sys', 'role-options'],
  queryFn: async () => unwrap(await api.GET('/v1/sys/roles/options')),
});

/** Explicit logout. The cookie is httpOnly, so only the server can clear it. */
export async function logout(): Promise<void> {
  await api.POST('/v1/sys/auth/logout', { body: {} });
  queryClient.clear();
}
