import { queryOptions } from '@tanstack/react-query';
import type { components } from '@/generated/openapi';
import { api, unwrap } from './api';
import { defineList } from './list';

/**
 * API wrappers for the audit domain (kernel). Both logs share this file, each with its own view code (`sys:audit:view`
 * / `sys:login_log:view`), so they can be granted separately.
 */
export type AuditLogListItem =
  components['schemas']['api.keel_admin.v1.AuditLogListItem'];
export type LoginLogListItem =
  components['schemas']['api.keel_admin.v1.LoginLogListItem'];

/**
 * Audit log definition: `created` is a following date range, sent as `createdFromAt`/`createdToAt`. operatorKeyword
 * fuzzy matches the operator's username or nickname, targetId is an ID, both of text kind; the options of
 * action/targetType are codes (strings), also of text kind.
 */
export const auditLogList = defineList({
  queryKey: ['sys', 'audit-logs'],
  fields: {
    operatorKeyword: { type: 'text' },
    action: { type: 'text' },
    targetType: { type: 'text' },
    targetId: { type: 'text' },
    ip: { type: 'text' },
    created: { type: 'dateRange' },
  },
  fetch: async (params) =>
    unwrap(await api.GET('/v1/sys/audit-logs', { params: { query: params } })),
  getRowId: (row: AuditLogListItem) => row.id ?? '',
});

/**
 * Login log definition: `userId` exactly matches an account ID (text kind, used to see one account's own logins);
 * `username` is a fuzzy match. `status` is a tri-state filter (0=all 1=success 2=failure), of number kind. `created` is
 * a time range precise to the second (following): investigating "who logged in during this period" needs narrowing to
 * exact instants.
 */
export const loginLogList = defineList({
  queryKey: ['sys', 'login-logs'],
  fields: {
    userId: { type: 'text' },
    username: { type: 'text' },
    status: { type: 'number' },
    ip: { type: 'text' },
    created: { type: 'timeRange' },
  },
  fetch: async (params) =>
    unwrap(await api.GET('/v1/sys/login-logs', { params: { query: params } })),
  getRowId: (row: LoginLogListItem) => row.id ?? '',
});

/**
 * Details of one audit record (including the request body): fetched by ID when viewing details; list rows carry no
 * request body. Records are append-only and never change, so what has been fetched need not be fetched again.
 */
export const auditDetailQueryOptions = (id: string) =>
  queryOptions({
    queryKey: ['sys', 'audit-log', id] as const,
    queryFn: async () =>
      unwrap(
        await api.GET('/v1/sys/audit-logs/{id}', { params: { path: { id } } }),
      ),
    staleTime: Number.POSITIVE_INFINITY,
    enabled: id !== '',
  });

/** Action options: grouped by target type, taken from the live registry rather than a hand-written list. */
export const auditActionsQueryOptions = queryOptions({
  queryKey: ['sys', 'audit-actions'],
  queryFn: async () => unwrap(await api.GET('/v1/sys/audit-logs/actions')),
});
