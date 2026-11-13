import { createFileRoute } from '@tanstack/react-router';
import { useMemo, useState } from 'react';
import { useTranslation } from 'react-i18next';
import {
  DataTable,
  type DataTableColumn,
  RowActions,
} from '@/components/data-table';
import { DateTimeText, StatusBadge } from '@/components/fields';
import { FilterBar } from '@/components/filter-bar';
import { PageHeader } from '@/components/page-header';
import { Button } from '@/components/ui/button';
import { useList } from '@/hooks/use-list';
import { type RoleListItem, roleList } from '@/lib/roles';
import { widthFor } from '@/lib/table';
import { RoleDeleteDialog, RoleDetail, RoleForm } from './-roles';

export const Route = createFileRoute('/_app/sys/roles')({
  // root-only: roles are the permission surface; seeing or editing them is touching the door to privilege escalation
  // (admin-0013).
  staticData: { titleKey: 'nav.roles', root: true },
  ...roleList.route,
  component: RolesPage,
});

function RolesPage() {
  const { t } = useTranslation();
  const list = useList(roleList, Route);
  // Opening and closing containers is an action inside the page, not in the URL (container rules). editing is an id or
  // 'new'.
  const [editing, setEditing] = useState<string>();
  const [viewing, setViewing] = useState<string>();
  const [deleting, setDeleting] = useState<RoleListItem>();

  // "All" is added as the first item by FilterBar; do not write it here.
  const tri = useMemo(
    () => [
      { value: '1', label: t('roles.yes') },
      { value: '2', label: t('roles.no') },
    ],
    [t],
  );

  const columns = useRoleColumns({
    onView: setViewing,
    onEdit: setEditing,
    onDelete: setDeleting,
  });

  return (
    <div className="flex flex-col gap-4">
      <PageHeader
        actions={
          <Button onClick={() => setEditing('new')}>{t('roles.create')}</Button>
        }
      />

      <DataTable<RoleListItem>
        tableKey="sys.roles"
        toolbar={
          <FilterBar
            fields={[
              {
                key: 'status',
                label: t('roles.status'),
                type: 'select',
                options: tri,
              },
              {
                key: 'sensitive',
                label: t('roles.sensitive'),
                type: 'select',
                options: tri,
              },
              {
                key: 'forceOtp',
                label: t('roles.forceOtp'),
                type: 'select',
                options: tri,
              },
              { key: 'keyword', label: t('roles.keyword'), type: 'input' },
            ]}
            {...list.filterBar}
          />
        }
        columns={columns}
        pinning={{ start: ['id'], end: ['actions'] }}
        {...list.table}
      />

      <RoleForm
        open={editing !== undefined}
        id={editing === 'new' ? undefined : editing}
        onClose={() => setEditing(undefined)}
      />
      <RoleDetail
        open={viewing !== undefined}
        id={viewing}
        onClose={() => setViewing(undefined)}
      />
      <RoleDeleteDialog
        role={deleting}
        onClose={() => setDeleting(undefined)}
      />
    </div>
  );
}

function useRoleColumns({
  onView,
  onEdit,
  onDelete,
}: {
  onView: (id: string) => void;
  onEdit: (id: string) => void;
  onDelete: (role: RoleListItem) => void;
}): DataTableColumn<RoleListItem>[] {
  const { t, i18n } = useTranslation();

  return useMemo(() => {
    // "Yes" means the role carries an extra constraint (forced authenticator / sensitive), worth seeing at a glance;
    // "No" is the norm and uses the neutral tone, otherwise a column full of colour has no emphasis.
    const yesNo = (v?: number) =>
      v === 1 ? (
        <StatusBadge tone="warn">{t('roles.yes')}</StatusBadge>
      ) : (
        <StatusBadge tone="muted">{t('roles.no')}</StatusBadge>
      );
    return [
      { id: 'id', accessorKey: 'id', header: 'ID', size: 90 },
      { id: 'code', accessorKey: 'code', header: t('roles.code'), size: 200 },
      { id: 'name', accessorKey: 'name', header: t('roles.name'), size: 160 },
      {
        id: 'forceOtp',
        header: t('roles.forceOtp'),
        size: 110,
        cell: ({ row }) => yesNo(row.original.forceOtp),
      },
      {
        id: 'sensitive',
        header: t('roles.sensitive'),
        size: 110,
        cell: ({ row }) => yesNo(row.original.sensitive),
      },
      {
        id: 'permCount',
        accessorKey: 'permCount',
        header: t('roles.permCount'),
        size: 110,
      },
      {
        id: 'status',
        header: t('roles.status'),
        size: 100,
        cell: ({ row }) =>
          row.original.status === 1 ? (
            <StatusBadge tone="ok">{t('users.enabled')}</StatusBadge>
          ) : (
            <StatusBadge tone="danger">{t('users.disabled')}</StatusBadge>
          ),
      },
      {
        id: 'createdAt',
        header: t('table.createdAt'),
        size: 220,
        cell: ({ row }) => <DateTimeText value={row.original.createdAt} />,
      },
      {
        id: 'actions',
        header: t('table.actions'),
        // Measured content width (buttons + padding + 8px margin): English 203px, Chinese 175px; all three buttons laid
        // out flat: "Details | Edit | Delete"
        size: widthFor({ default: 203, 'zh-CN': 175 }, i18n.language),
        cell: ({ row }) => (
          <RowActions
            actions={[
              {
                kind: 'button',
                key: 'view',
                label: t('action.detail'),
                onSelect: () => onView(row.original.id ?? ''),
              },
              {
                kind: 'button',
                key: 'edit',
                label: t('action.edit'),
                onSelect: () => onEdit(row.original.id ?? ''),
              },
              {
                kind: 'button',
                key: 'delete',
                label: t('action.delete'),
                tone: 'destructive',
                onSelect: () => onDelete(row.original),
              },
            ]}
          />
        ),
      },
    ];
  }, [t, onView, onEdit, onDelete, i18n.language]);
}
