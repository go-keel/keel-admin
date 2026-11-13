import { queryOptions } from '@tanstack/react-query';
import type { components } from '@/generated/openapi';
import { api, stepUp, unwrap } from './api';
import { queryClient } from './query';

/** API wrappers for the department domain (kernel). The department tree is not paged: parents and children cannot span
 * pages, see ADR admin-0004. */
export type DeptListItem =
  components['schemas']['api.keel_admin.v1.DeptListItem'];

export const deptListQueryOptions = queryOptions({
  queryKey: ['sys', 'depts'],
  queryFn: async () => unwrap(await api.GET('/v1/sys/depts')),
});

/** Department details: the initial values of the edit form, handed to `useDetailForm` (which enforces a fresh
 * fetch). */
export const deptDetailQueryOptions = (id: string) =>
  queryOptions({
    queryKey: ['sys', 'dept', id] as const,
    queryFn: async () =>
      unwrap(await api.GET('/v1/sys/depts/{id}', { params: { path: { id } } })),
  });

export const invalidateDepts = () =>
  queryClient.invalidateQueries({ queryKey: ['sys', 'depts'] });

export type DeptFormValues = {
  parentId: string;
  name: string;
  sort: number;
  status: number;
};

/** Creating a root department, creating a child department and editing share this one entry: same people, same bar. */
export async function saveDept(
  id: string | undefined,
  v: DeptFormValues,
  otpCode: string,
): Promise<void> {
  if (id) {
    unwrap(
      await api.PUT('/v1/sys/depts/{id}', {
        params: { path: { id } },
        body: { id, name: v.name, sort: v.sort, status: v.status },
        ...stepUp(otpCode),
      }),
    );
  } else {
    unwrap(
      await api.POST('/v1/sys/depts', {
        body: { parentId: v.parentId, name: v.name, sort: v.sort },
        ...stepUp(otpCode),
      }),
    );
  }
  await invalidateDepts();
}

/**
 * Delete. Having child departments / users / business references is rejected by the server with a localized message;
 * **the frontend does not pre-check**: that would copy the server's logic by hand and the two would drift
 * (docs/admin/permission-model/README.md §3).
 */
export async function deleteDept(id: string, otpCode: string): Promise<void> {
  unwrap(
    await api.DELETE('/v1/sys/depts/{id}', {
      params: { path: { id } },
      ...stepUp(otpCode),
    }),
  );
  await invalidateDepts();
}
