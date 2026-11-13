import { queryOptions } from '@tanstack/react-query';
import type { SearchSelectSource } from '@/components/filter-bar';
import type { components } from '@/generated/openapi';
import { api, stepUp, unwrap } from './api';
import { defineList } from './list';
import { queryClient } from './query';
import { toTotal } from './table';

/**
 * Endpoint wrappers of the user domain (kernel, one file per domain). Business-area domains live in the business area,
 * not here.
 */
export type UserListItem =
  components['schemas']['api.keel_admin.v1.UserListItem'];
export type DeptOption = components['schemas']['api.keel_admin.v1.DeptOption'];
export type RoleOption = components['schemas']['api.keel_admin.v1.RoleOption'];

/**
 * User options (fetched in batches, for the user filter of login logs): spread into a `searchSelect` field of the
 * filter bar. 50 per batch, within the upper bound of `ListUserOptionsRequest.limit`.
 */
export const userOptionsSource: SearchSelectSource = {
  queryKey: ['sys', 'user-options'],
  pageSize: 50,
  load: async (query) => {
    const r = unwrap(
      await api.GET('/v1/sys/users/options', { params: { query } }),
    );
    return {
      items: (r.items ?? []).map((u) => ({
        value: u.id ?? '',
        label: u.username ?? '',
      })),
      total: toTotal(r.total),
    };
  },
};

/**
 * The user list definition: which filters exist, how each looks in the URL and in the request, and which endpoint the
 * data comes from. The query key prefix `['sys', 'users']` matches `invalidateUsers`, which invalidates by prefix;
 * the filter values and paging the definition appends to the key do not affect that match.
 */
export const userList = defineList({
  queryKey: ['sys', 'users'],
  fields: {
    keyword: { type: 'text' },
    deptId: { type: 'text' },
    status: { type: 'number' },
    otpBound: { type: 'number' },
    roleId: { type: 'text' },
  },
  fetch: async (params) =>
    unwrap(await api.GET('/v1/sys/users', { params: { query: params } })),
  getRowId: (row: UserListItem) => row.id ?? '',
});

/**
 * User detail: the view drawer fetches it by ID on demand (refetched on every open, dropped on close); the edit form
 * goes through `useDetailForm`, which enforces the fresh fetch (ADR admin-0006, form initial values fetch detail).
 */
export const userDetailQueryOptions = (id: string) =>
  queryOptions({
    queryKey: ['sys', 'user', id] as const,
    queryFn: async () =>
      unwrap(await api.GET('/v1/sys/users/{id}', { params: { path: { id } } })),
    staleTime: 0,
    gcTime: 0,
    enabled: id !== '' && id !== 'new',
  });

/** Department options (a tree, already sorted by level). Bound to sys:account:view, since they serve the account page. */
export const deptOptionsQueryOptions = queryOptions({
  queryKey: ['sys', 'dept-options'],
  queryFn: async () => unwrap(await api.GET('/v1/sys/depts/options')),
});

/** Roles I can assign: the role dropdown when editing a user, not the same set as the full options used for filtering. */
export const assignableRolesQueryOptions = queryOptions({
  queryKey: ['sys', 'roles-assignable'],
  queryFn: async () => unwrap(await api.GET('/v1/sys/roles/assignable')),
});

/** Invalidates the user domain's cache after any successful write. */
export const invalidateUsers = () =>
  queryClient.invalidateQueries({ queryKey: ['sys', 'users'] });

export type UserFormValues = {
  username: string;
  password: string;
  nickname: string;
  avatar: string;
  email: string;
  phone: string;
  deptId: string;
  roleIds: string[];
  ipWhitelist: string;
};

/**
 * Submit only the roles you can assign. The form's initial values come from the detail and carry roles you cannot see
 * (disabled, sensitive); submitted as is, the backend rejects them as overreach under "the submitted set must lie
 * entirely within what you can assign". The frontend need not handle the invisible roles: the backend adds them back
 * from the current roles it reads under the lock (`resolveRoleEdit`). root gets the full options, so nothing is lost.
 */
export function submittableRoleIds(
  roleIds: string[],
  assignable: { id?: string }[],
): string[] {
  const ok = new Set(assignable.map((r) => r.id));
  return roleIds.filter((id) => ok.has(id));
}

/** Create and edit share one form and hence one submit entry; one step-up for the whole form. */
export async function saveUser(
  id: string | undefined,
  values: UserFormValues,
  otpCode: string,
): Promise<void> {
  if (id) {
    unwrap(
      await api.PUT('/v1/sys/users/{id}', {
        params: { path: { id } },
        body: {
          id,
          nickname: values.nickname,
          avatar: values.avatar,
          email: values.email,
          phone: values.phone,
          deptId: values.deptId,
          // Full-replacement semantics: unchanged fields are sent back as is; the handler compares with the current
          // values and validates only those that really changed.
          roleIds: values.roleIds,
          ipWhitelist: values.ipWhitelist,
        },
        ...stepUp(otpCode),
      }),
    );
  } else {
    unwrap(
      await api.POST('/v1/sys/users', {
        body: {
          username: values.username,
          password: values.password,
          nickname: values.nickname,
          avatar: values.avatar,
          email: values.email,
          phone: values.phone,
          deptId: values.deptId,
          roleIds: values.roleIds,
          ipWhitelist: values.ipWhitelist,
        },
        ...stepUp(otpCode),
      }),
    );
  }
  await invalidateUsers();
}

/**
 * Grant / revoke super admin: a separate action in the user row's "⋯" menu, visible only to root, collecting a code
 * once in the dialog. Its gate (root-only) differs from editing a user, so it is split into its own operation by
 * permission, which leaves the edit form with a single step-up point (ADR admin-0007, operation split and step-up
 * placement).
 */
export async function setUserRoot(
  id: string,
  isRoot: boolean,
  otpCode: string,
): Promise<void> {
  unwrap(
    await api.POST('/v1/sys/users/{id}/root', {
      params: { path: { id } },
      body: { id, isRoot },
      ...stepUp(otpCode),
    }),
  );
  await invalidateUsers();
}

/** Row actions: each confirms and collects a code on its own; one-off actions are independent of each other. */
export async function resetPassword(
  id: string,
  password: string,
  otpCode: string,
): Promise<void> {
  unwrap(
    await api.POST('/v1/sys/users/{id}/reset_pwd', {
      params: { path: { id } },
      body: { id, password },
      ...stepUp(otpCode),
    }),
  );
}

export async function userAction(
  id: string,
  action: 'reset_otp' | 'disable' | 'enable' | 'kick',
  otpCode: string,
): Promise<void> {
  const path = {
    reset_otp: '/v1/sys/users/{id}/reset_otp',
    disable: '/v1/sys/users/{id}/disable',
    enable: '/v1/sys/users/{id}/enable',
    kick: '/v1/sys/users/{id}/kick',
  }[action] as '/v1/sys/users/{id}/reset_otp';
  unwrap(
    await api.POST(path, {
      params: { path: { id } },
      body: { id },
      ...stepUp(otpCode),
    }),
  );
  await invalidateUsers();
}
