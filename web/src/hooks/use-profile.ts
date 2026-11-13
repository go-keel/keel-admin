import { useSuspenseQuery } from '@tanstack/react-query';
import { useCallback } from 'react';
import type { PermCode } from '@/generated/perm-code';
import { has, type Profile } from '@/lib/perm';
import { profileQueryOptions } from '@/lib/query';

/** Usable inside the auth shell: beforeLoad has already prefetched the profile into the cache and never evicts it (see
 * profileQueryOptions), so this never actually suspends. */
export const useProfile = (): Profile =>
  useSuspenseQuery(profileQueryOptions).data;

/** Element-level permission check: `const can = usePerm(); can('sys:account:create')`. */
export const usePerm = (): ((code?: PermCode) => boolean) => {
  const profile = useProfile();
  return useCallback((code?: PermCode) => has(profile, code), [profile]);
};
