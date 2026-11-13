import { queryOptions } from '@tanstack/react-query';
import { api, stepUp, unwrap } from './api';
import { profileQueryOptions, queryClient } from './query';

/** Self-service actions (kernel): edit profile, change password, bind/unbind an authenticator. */

/**
 * Refresh after the own profile changes: the profile the header reads and the account details the profile page reads
 * are invalidated together.
 */
export const refreshProfile = () =>
  Promise.all([
    queryClient.invalidateQueries({ queryKey: profileQueryOptions.queryKey }),
    queryClient.invalidateQueries({ queryKey: ACCOUNT_KEY }),
  ]);

const ACCOUNT_KEY = ['sys', 'account'] as const;

const fetchAccount = async () => unwrap(await api.GET('/v1/sys/auth/account'));

/** Own account details (shown on the profile page, prefetched by the route loader). */
export const myAccountQueryOptions = queryOptions({
  queryKey: ACCOUNT_KEY,
  queryFn: fetchAccount,
});

/**
 * Fresh-fetch version of the account details: the initial values of the "edit profile" form, handed to `useDetailForm`
 * (which enforces a fresh fetch, sent only on open). A separate key, not sharing the cache with the page copy: the form
 * copy is dropped on close and should not take the page copy with it.
 */
export const myAccountFreshQueryOptions = queryOptions({
  queryKey: [...ACCOUNT_KEY, 'fresh'] as const,
  queryFn: fetchAccount,
});

/** Change nickname, avatar, email, phone. No permissions or password involved, so no step-up. */
export async function updateMyProfile(body: {
  nickname: string;
  avatar: string;
  email: string;
  phone: string;
}): Promise<void> {
  unwrap(await api.PUT('/v1/sys/auth/profile', { body }));
  // The avatar and nickname in the header read this profile too; invalidating keeps them in sync.
  await refreshProfile();
}

/**
 * Changing the password needs step-up: with a hijacked session, changing the password takes over the account, and the
 * old password is often obtainable in the same session. On success token_version is bumped, and the next request is
 * sent back to the login page by the session invalidation rule.
 */
export async function changeMyPassword(
  oldPassword: string,
  newPassword: string,
  otpCode: string,
): Promise<void> {
  unwrap(
    await api.POST('/v1/sys/auth/password', {
      body: { oldPassword, newPassword },
      ...stepUp(otpCode),
    }),
  );
}

/** Unbinding an authenticator verifies the current password, not step-up: someone who lost the authenticator must be
 * able to recover. */
export async function unbindOtp(password: string): Promise<void> {
  unwrap(await api.POST('/v1/sys/auth/otp/unbind', { body: { password } }));
  await refreshProfile();
}
