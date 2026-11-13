import { queryOptions } from '@tanstack/react-query';
import type { components } from '@/generated/openapi';
import { api, stepUp, unwrap } from './api';
import { defineList } from './list';
import { queryClient } from './query';

/**
 * Endpoint wrappers of the role domain (kernel). The whole group is root-only; the only two exceptions are explained in
 * the comments of perm.go.
 */
export type RoleListItem =
  components['schemas']['api.keel_admin.v1.RoleListItem'];
export type RoleInfo = components['schemas']['api.keel_admin.v1.RoleInfo'];
export type PermissionDef =
  components['schemas']['api.keel_admin.v1.PermissionDef'];
export type PermGroup = components['schemas']['api.keel_admin.v1.PermGroup'];
export type ResourceDef =
  components['schemas']['api.keel_admin.v1.ResourceDef'];
export type DataScopeItem =
  components['schemas']['api.keel_admin.v1.DataScopeItem'];

/**
 * The role list definition: which filters exist, how each looks in the URL and in the request, and which endpoint the
 * data comes from. status/sensitive/forceOtp are three tri-state filters (0=all 1=yes 2=no), of the number kind.
 * The query key prefix `['sys', 'roles']` matches `invalidateRoles`.
 */
export const roleList = defineList({
  queryKey: ['sys', 'roles'],
  fields: {
    keyword: { type: 'text' },
    status: { type: 'number' },
    sensitive: { type: 'number' },
    forceOtp: { type: 'number' },
  },
  fetch: async (params) =>
    unwrap(await api.GET('/v1/sys/roles', { params: { query: params } })),
  getRowId: (row: RoleListItem) => row.id ?? '',
});

/**
 * Role detail: the view drawer fetches it by ID on demand (refetched on every open, dropped on close); the edit form
 * goes through `useDetailForm`.
 */
export const roleDetailQueryOptions = (id: string) =>
  queryOptions({
    queryKey: ['sys', 'role', id] as const,
    queryFn: async () =>
      unwrap(await api.GET('/v1/sys/roles/{id}', { params: { path: { id } } })),
    staleTime: 0,
    gcTime: 0,
    enabled: id !== '' && id !== 'new',
  });

/**
 * All permission codes plus group names. Returned at once, so the UI needs no in-between state of codes without
 * names.
 */
export const permissionsQueryOptions = queryOptions({
  queryKey: ['sys', 'permissions'],
  queryFn: async () => unwrap(await api.GET('/v1/sys/meta/permissions')),
});

/**
 * Data scope resource list: which scope_type values are valid, whether explicit assignment is allowed, which
 * instances are the options.
 */
export const resourcesQueryOptions = queryOptions({
  queryKey: ['sys', 'resources'],
  queryFn: async () => unwrap(await api.GET('/v1/sys/meta/resources')),
});

/**
 * Options of "assignable roles": **sensitive roles are excluded** (`excludeSensitive`).
 *
 * This and the filter's set (`roleOptionsQueryOptions`, the full set) are two uses of the same data, hence two
 * queries: the filter must keep false, otherwise users with sensitive roles could not be filtered (see role.proto).
 * A sensitive role has a single meaning: non-root cannot assign it to others (admin-0015). Since putting it on the
 * assignable allow-list would not let it be sent either, it must not appear among the options.
 */
export const assignableCandidatesQueryOptions = queryOptions({
  queryKey: ['sys', 'role-options', 'assignable'] as const,
  queryFn: async () =>
    unwrap(
      await api.GET('/v1/sys/roles/options', {
        params: { query: { excludeSensitive: true } },
      }),
    ),
});

/** Department options (for the multi-select of manageable departments). */
export const deptTreeQueryOptions = queryOptions({
  queryKey: ['sys', 'dept-options'],
  queryFn: async () => unwrap(await api.GET('/v1/sys/depts/options')),
});

export const invalidateRoles = () =>
  queryClient.invalidateQueries({ queryKey: ['sys', 'roles'] });

export type RoleFormValues = {
  code: string;
  name: string;
  status: number;
  sensitive: number;
  forceOtp: number;
  perms: string[];
  assignableRoleIds: string[];
  manageDeptIds: string[];
  scopes: DataScopeItem[];
};

/**
 * Create and edit submit once and collect a code once. **Full-replacement semantics**: perms / the three boundary
 * dimensions / scopes are all delete-all + insert, so initial values must come from GetRole and unchanged parts are
 * sent back as is.
 */
export async function saveRole(
  id: string | undefined,
  v: RoleFormValues,
  otpCode: string,
): Promise<void> {
  const shared = {
    name: v.name,
    status: v.status,
    sensitive: v.sensitive,
    forceOtp: v.forceOtp,
    perms: v.perms,
    assignableRoleIds: v.assignableRoleIds,
    manageDeptIds: v.manageDeptIds,
    scopes: v.scopes,
  };
  if (id) {
    unwrap(
      await api.PUT('/v1/sys/roles/{id}', {
        params: { path: { id } },
        body: { id, ...shared },
        ...stepUp(otpCode),
      }),
    );
  } else {
    unwrap(
      await api.POST('/v1/sys/roles', {
        // code can be set only on create; it cannot be changed on edit.
        body: { code: v.code, ...shared },
        ...stepUp(otpCode),
      }),
    );
  }
  await invalidateRoles();
}

export async function deleteRole(id: string, otpCode: string): Promise<void> {
  unwrap(
    await api.DELETE('/v1/sys/roles/{id}', {
      params: { path: { id } },
      ...stepUp(otpCode),
    }),
  );
  await invalidateRoles();
}
