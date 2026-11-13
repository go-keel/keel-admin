import type { components } from '@/generated/openapi';
import type { PermCode } from '@/generated/perm-code';

export type Profile = components['schemas']['api.keel_admin.v1.ProfileReply'];

/**
 * The single entry point for permission checks. root bypasses all permission codes, matching the backend's
 * perm.Principal (root's perms is an empty array meaning "do not look at this array", not "has nothing").
 * No code means public.
 */
export const has = (profile: Profile | undefined, code?: PermCode): boolean =>
  !code || (!!profile && (!!profile.isRoot || !!profile.perms?.includes(code)));
