import { createFileRoute } from '@tanstack/react-router';
import { useMemo } from 'react';
import { useTranslation } from 'react-i18next';
import { DataTable, type DataTableColumn } from '@/components/data-table';
import { DateTimeText, StatusBadge } from '@/components/fields';
import { FilterBar } from '@/components/filter-bar';
import { useList } from '@/hooks/use-list';
import { type LoginLogListItem, loginLogList } from '@/lib/audit';
import { userOptionsSource } from '@/lib/users';

/**
 * Login logs: the outcome and reason of every login attempt. Filters go into the URL: "View all" in the user details
 * links here with `userId`. A separate page from the audit log: the two lists differ in data, columns and filters, and
 * each has its own view code.
 */
export const Route = createFileRoute('/_app/sys/login-logs')({
  staticData: { titleKey: 'nav.loginLogs', perm: 'sys:login_log:view' },
  ...loginLogList.route,
  component: LoginLogsPage,
});

function LoginLogsPage() {
  const { t } = useTranslation();
  const list = useList(loginLogList, Route);
  const columns = useLoginColumns();

  return (
    <div className="flex flex-col gap-4">
      <DataTable<LoginLogListItem>
        tableKey="sys.loginLogs"
        toolbar={
          <FilterBar
            fields={[
              {
                key: 'userId',
                label: t('audit.user'),
                type: 'searchSelect',
                ...userOptionsSource,
              },
              { key: 'username', label: t('users.username'), type: 'input' },
              {
                key: 'status',
                label: t('audit.result'),
                type: 'select',
                // "All" is added as the first item by FilterBar; do not write it here.
                options: [
                  { value: '1', label: t('audit.success') },
                  { value: '2', label: t('audit.failure') },
                ],
              },
              { key: 'ip', label: 'IP', type: 'input' },
              { key: 'created', label: t('audit.range'), type: 'timeRange' },
            ]}
            {...list.filterBar}
          />
        }
        columns={columns}
        // Login logs have no details to view (no request body), hence no actions column, so nothing is pinned on the
        // right. "Is there an actions column" decides only what is pinned right, not whether the whole behaviour is on.
        pinning={{ start: ['id'], end: [] }}
        {...list.table}
      />
    </div>
  );
}

function useLoginColumns(): DataTableColumn<LoginLogListItem>[] {
  const { t } = useTranslation();
  return useMemo(
    () => [
      { id: 'id', accessorKey: 'id', header: 'ID', size: 100 },
      {
        id: 'username',
        header: t('users.username'),
        size: 160,
        // The username of a failed attempt is what was typed; that account may not exist.
        cell: ({ row }) => row.original.username || row.original.userId || '-',
      },
      {
        id: 'success',
        header: t('audit.result'),
        size: 100,
        cell: ({ row }) =>
          row.original.success ? (
            <StatusBadge tone="ok">{t('audit.success')}</StatusBadge>
          ) : (
            <StatusBadge tone="danger">{t('audit.failure')}</StatusBadge>
          ),
      },
      {
        id: 'msg',
        header: t('audit.reason'),
        size: 280,
        cell: ({ row }) => row.original.msg || '-',
      },
      { id: 'ip', accessorKey: 'ip', header: 'IP', size: 140 },
      {
        id: 'createdAt',
        header: t('audit.time'),
        size: 220,
        cell: ({ row }) => <DateTimeText value={row.original.createdAt} />,
      },
    ],
    [t],
  );
}
