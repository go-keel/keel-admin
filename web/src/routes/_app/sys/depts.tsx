import { useQuery } from '@tanstack/react-query';
import { createFileRoute } from '@tanstack/react-router';
import { ChevronDownIcon, ChevronRightIcon } from 'lucide-react';
import { useMemo, useState } from 'react';
import { useTranslation } from 'react-i18next';
import {
  DataTable,
  type DataTableColumn,
  RowActions,
} from '@/components/data-table';
import { StatusBadge } from '@/components/fields';
import { PageHeader } from '@/components/page-header';
import { Button } from '@/components/ui/button';
import { usePerm, useProfile } from '@/hooks/use-profile';
import { type DeptListItem, deptListQueryOptions } from '@/lib/depts';
import { widthFor } from '@/lib/table';
import { buildTree, type TreeNode } from '@/lib/tree';
import { DeptDeleteDialog, DeptForm } from './-depts';

// The tree is not paged and has no filters, so the URL carries no parameters.
export const Route = createFileRoute('/_app/sys/depts')({
  staticData: { titleKey: 'nav.depts', perm: 'sys:dept:view' },
  loader: ({ context }) => {
    void context.queryClient.prefetchQuery(deptListQueryOptions);
  },
  component: DeptsPage,
});

type DeptRow = TreeNode<DeptListItem>;

// The table needs tree rows: converted in the query (select), so the table gets rows directly. Module-level so the
// function reference is stable and nothing is recomputed while the data is unchanged.
const toTree = (d: { items?: DeptListItem[] | null }): DeptRow[] =>
  buildTree(d.items ?? []);

function DeptsPage() {
  const { t } = useTranslation();
  const can = usePerm();
  // Only root may create a top-level department (the server answers NOT_ROOT otherwise), so non-root sees no entry
  // bound to be refused.
  const isRoot = useProfile().isRoot;
  const query = useQuery({ ...deptListQueryOptions, select: toTree });
  // Opening and closing containers is an action inside the page, not in the URL (container rules). editing is an id or
  // 'new'; when creating a sub-department, parent is its parent department.
  const [editing, setEditing] = useState<string>();
  const [parent, setParent] = useState<DeptListItem>();
  const [deleting, setDeleting] = useState<DeptListItem>();

  const columns = useDeptColumns({
    onEdit: setEditing,
    onAddChild: (dept) => {
      setParent(dept);
      setEditing('new');
    },
    onDelete: setDeleting,
  });

  return (
    <div className="flex flex-col gap-4">
      <PageHeader
        actions={
          can('sys:dept:create') && isRoot ? (
            <Button
              onClick={() => {
                setParent(undefined);
                setEditing('new');
              }}
            >
              {t('depts.createRoot')}
            </Button>
          ) : null
        }
      />

      <DataTable<DeptRow>
        tableKey="sys.depts"
        columns={columns}
        query={query}
        // Tree tables **do not pin the hierarchy column**: the name must be read with its indentation, and pinning it
        // keeps the widest column in the viewport, leaving even less horizontal space (see the table ADR). Only the
        // actions column is pinned.
        pinning={{ start: [], end: ['actions'] }}
        getSubRows={(row) => row.children}
        getRowId={(row) => row.id ?? ''}
      />

      <DeptForm
        open={editing !== undefined}
        id={editing === 'new' ? undefined : editing}
        parent={editing === 'new' ? parent : undefined}
        onClose={() => setEditing(undefined)}
      />
      <DeptDeleteDialog
        dept={deleting}
        onClose={() => setDeleting(undefined)}
      />
    </div>
  );
}

function useDeptColumns({
  onEdit,
  onAddChild,
  onDelete,
}: {
  onEdit: (id: string) => void;
  onAddChild: (dept: DeptListItem) => void;
  onDelete: (dept: DeptListItem) => void;
}): DataTableColumn<DeptRow>[] {
  const { t, i18n } = useTranslation();
  const can = usePerm();

  return useMemo(
    () => [
      {
        id: 'name',
        header: t('depts.name'),
        size: 320,
        cell: ({ row }) => (
          // Each level indents four spacing units, scaling with the appearance preset's --spacing. The expand arrow is
          // visible only with child rows: drawing one without children would do nothing on click. The button is still
          // rendered, just invisible (it keeps its space but cannot be seen or clicked), so names align with rows that
          // have children, with no separate placeholder width tracking the button size.
          <div
            className="flex items-center gap-1"
            style={{
              paddingInlineStart: `calc(var(--spacing) * ${4 * row.depth})`,
            }}
          >
            <Button
              variant="ghost"
              size="icon-xs"
              aria-label={t('depts.toggle')}
              className={row.getCanExpand() ? undefined : 'invisible'}
              onClick={row.getToggleExpandedHandler()}
            >
              {row.getIsExpanded() ? <ChevronDownIcon /> : <ChevronRightIcon />}
            </Button>
            {row.original.name}
          </div>
        ),
      },
      { id: 'sort', accessorKey: 'sort', header: t('depts.sort'), size: 100 },
      {
        id: 'userCount',
        accessorKey: 'userCount',
        header: t('depts.userCount'),
        size: 110,
      },
      {
        id: 'status',
        header: t('users.status'),
        size: 100,
        cell: ({ row }) =>
          row.original.status === 1 ? (
            <StatusBadge tone="ok">{t('users.enabled')}</StatusBadge>
          ) : (
            <StatusBadge tone="danger">{t('users.disabled')}</StatusBadge>
          ),
      },
      {
        id: 'actions',
        header: t('table.actions'),
        // Measured content width (buttons + padding + 8px margin): English 288px, Chinese 213px; all three buttons laid
        // out flat: "New sub-department | Edit | Delete"
        size: widthFor({ default: 288, 'zh-CN': 213 }, i18n.language),
        cell: ({ row }) => (
          <RowActions
            actions={[
              ...(can('sys:dept:create')
                ? [
                    {
                      kind: 'button' as const,
                      key: 'add',
                      label: t('depts.createChild'),
                      onSelect: () => onAddChild(row.original),
                    },
                  ]
                : []),
              ...(can('sys:dept:update')
                ? [
                    {
                      kind: 'button' as const,
                      key: 'edit',
                      label: t('action.edit'),
                      onSelect: () => onEdit(row.original.id ?? ''),
                    },
                  ]
                : []),
              ...(can('sys:dept:delete')
                ? [
                    {
                      kind: 'button' as const,
                      key: 'delete',
                      label: t('action.delete'),
                      tone: 'destructive' as const,
                      onSelect: () => onDelete(row.original),
                    },
                  ]
                : []),
            ]}
          />
        ),
      },
    ],
    [t, can, onEdit, onAddChild, onDelete, i18n.language],
  );
}
