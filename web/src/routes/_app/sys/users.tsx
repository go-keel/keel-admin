import { useQuery } from '@tanstack/react-query';
import { createFileRoute } from '@tanstack/react-router';
import type { TFunction } from 'i18next';
import {
  BanIcon,
  CircleCheckIcon,
  KeyRoundIcon,
  LogOutIcon,
  ShieldIcon,
  ShieldOffIcon,
  SquarePenIcon,
} from 'lucide-react';
import { useMemo, useState } from 'react';
import { useTranslation } from 'react-i18next';
import {
  DataTable,
  type DataTableColumn,
  type RowAction,
  RowActions,
} from '@/components/data-table';
import { DateTimeText, StatusBadge } from '@/components/fields';
import { FilterBar, type FilterOption } from '@/components/filter-bar';
import { PageHeader } from '@/components/page-header';
import { Badge } from '@/components/ui/badge';
import { Button } from '@/components/ui/button';
import type { PermCode } from '@/generated/perm-code';
import { type FilterFieldsOf, useList } from '@/hooks/use-list';
import { usePerm, useProfile } from '@/hooks/use-profile';
import { roleOptionsQueryOptions } from '@/lib/query';
import { widthFor } from '@/lib/table';
import {
  deptOptionsQueryOptions,
  type UserListItem,
  userList,
} from '@/lib/users';
import {
  type PendingAction,
  UserActionDialog,
  UserDetail,
  UserForm,
} from './-users';

export const Route = createFileRoute('/_app/sys/users')({
  staticData: { titleKey: 'nav.users', perm: 'sys:account:view' },
  ...userList.route,
  component: UsersPage,
});

function UsersPage() {
  const { t } = useTranslation();
  const can = usePerm();
  const list = useList(userList, Route);
  // Options use each domain's *Options endpoint; the gate follows the purpose, both bound to sys:account:view.
  const deptsQuery = useQuery(deptOptionsQueryOptions);
  const rolesQuery = useQuery(roleOptionsQueryOptions);
  const depts = deptsQuery.data?.items ?? [];
  const roles = rolesQuery.data?.items ?? [];

  // Opening and closing containers is an action inside the page, not in the URL (container rules). editing is an id or
  // 'new'.
  const [editing, setEditing] = useState<string>();
  const [viewing, setViewing] = useState<string>();
  const [pending, setPending] = useState<PendingAction>();

  const columns = useUserColumns({
    onView: setViewing,
    onEdit: setEditing,
    onAction: (kind, user) => setPending({ kind, user }),
  });

  const fields = useMemo(
    (): FilterFieldsOf<typeof userList> => [
      {
        key: 'deptId',
        label: t('users.dept'),
        type: 'treeSelect',
        options: deptTreeOptions(depts),
        load: deptsQuery,
      },
      {
        key: 'status',
        label: t('users.status'),
        type: 'select',
        options: [
          { value: '1', label: t('users.enabled') },
          { value: '2', label: t('users.disabled') },
        ],
      },
      {
        key: 'otpBound',
        label: t('users.otp'),
        type: 'select',
        options: [
          { value: '1', label: t('users.otpBound') },
          { value: '2', label: t('users.otpUnbound') },
        ],
      },
      {
        key: 'roleId',
        label: t('users.roles'),
        type: 'select',
        options: roles.map((r) => ({ value: r.id ?? '', label: r.name ?? '' })),
        load: rolesQuery,
      },
      {
        key: 'keyword',
        label: t('users.keyword'),
        type: 'input',
        // The keyword matches four fields at once; the placeholder says so, so nobody wonders whether searching by
        // email works.
        placeholder: t('users.keywordHint'),
      },
    ],
    [t, depts, roles, deptsQuery, rolesQuery],
  );

  return (
    <div className="flex flex-col gap-4">
      <PageHeader
        actions={
          can('sys:account:create') ? (
            <Button onClick={() => setEditing('new')}>
              {t('users.create')}
            </Button>
          ) : null
        }
      />

      <DataTable<UserListItem>
        tableKey="sys.users"
        toolbar={<FilterBar fields={fields} {...list.filterBar} />}
        columns={columns}
        // Pinned columns are declared explicitly by the page; the shared component does not infer them (see table
        // rules).
        pinning={{ start: ['id'], end: ['actions'] }}
        {...list.table}
      />

      <UserForm
        open={editing !== undefined}
        id={editing === 'new' ? undefined : editing}
        onClose={() => setEditing(undefined)}
      />
      <UserDetail
        open={viewing !== undefined}
        id={viewing}
        onClose={() => setViewing(undefined)}
      />
      <UserActionDialog
        pending={pending}
        onClose={() => setPending(undefined)}
      />
    </div>
  );
}

/**
 * Department tree → indented flat options. The filter bar does not know trees; building the tree is the page's job.
 * "All" is added as the first item by FilterBar; do not write it here.
 */
function deptTreeOptions(
  depts: { id?: string; parentId?: string; name?: string }[],
): FilterOption[] {
  const byParent = new Map<string, typeof depts>();
  for (const d of depts) {
    const key = d.parentId ?? '0';
    byParent.set(key, [...(byParent.get(key) ?? []), d]);
  }
  const out: FilterOption[] = [];
  const walk = (parent: string, depth: number) => {
    for (const d of byParent.get(parent) ?? []) {
      out.push({
        value: d.id ?? '',
        // Two no-break spaces per level: same indent in every UI language, and not collapsed like normal spaces.
        label: `${'\u00a0\u00a0'.repeat(depth)}${d.name ?? ''}`,
      });
      walk(d.id ?? '', depth + 1);
    }
  };
  walk('0', 0);
  return out;
}

/** Every column gets an explicit width: without one, the declared pinning silently stops working (see table rules). */
function useUserColumns({
  onView,
  onEdit,
  onAction,
}: {
  onView: (id: string) => void;
  onEdit: (id: string) => void;
  onAction: (kind: PendingAction['kind'], user: UserListItem) => void;
}): DataTableColumn<UserListItem>[] {
  const { t, i18n } = useTranslation();
  const can = usePerm();
  const me = useProfile();

  return useMemo(
    () => [
      { id: 'id', accessorKey: 'id', header: 'ID', size: 90 },
      {
        id: 'username',
        accessorKey: 'username',
        header: t('users.username'),
        size: 140,
      },
      {
        id: 'nickname',
        accessorKey: 'nickname',
        header: t('users.nickname'),
        size: 140,
      },
      {
        id: 'email',
        header: t('field.email'),
        size: 220,
        cell: ({ row }) => row.original.email || '-',
      },
      {
        id: 'phone',
        header: t('field.phone'),
        size: 160,
        cell: ({ row }) => row.original.phone || '-',
      },
      {
        id: 'deptName',
        accessorKey: 'deptName',
        header: t('users.dept'),
        size: 160,
      },
      {
        id: 'roles',
        header: t('users.roles'),
        size: 220,
        cell: ({ row }) => (
          <div className="flex flex-wrap gap-1">
            {/* The super admin badge is shown only to root: who holds all permissions matters only to those who can
                grant and revoke it. */}
            {me.isRoot && row.original.isRoot ? (
              <Badge>{t('users.root')}</Badge>
            ) : null}
            {(row.original.roles ?? []).map((r) => (
              <Badge key={r.id} variant={r.disabled ? 'outline' : 'secondary'}>
                {r.name}
                {r.disabled ? ` · ${t('users.disabled')}` : null}
              </Badge>
            ))}
          </div>
        ),
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
        id: 'otp',
        header: t('users.otp'),
        // The longest badge measured is the English "Required, not bound" at 131px + cell padding 16 + margin
        size: 160,
        cell: ({ row }) =>
          row.original.otpBound ? (
            <StatusBadge tone="ok">{t('users.otpBound')}</StatusBadge>
          ) : row.original.otpForced ? (
            // A verdict computed by the server; the frontend does not derive it from roles (see ADR "frontend does not
            // infer config effect").
            <StatusBadge tone="danger">
              {t('users.otpForcedUnbound')}
            </StatusBadge>
          ) : (
            // Neither bound nor forced: a neutral fact, neither a to-do nor a problem.
            <StatusBadge tone="muted">{t('users.otpUnbound')}</StatusBadge>
          ),
      },
      {
        id: 'ipWhitelist',
        header: t('field.ipWhitelist'),
        size: 200,
        // Only whether one is set: the allowlist itself is in the user's detail.
        cell: ({ row }) =>
          row.original.hasIpWhitelist ? t('users.ipRestricted') : '-',
      },
      {
        id: 'createdAt',
        header: t('table.createdAt'),
        // Measured "2026-07-25 22:01:59 (CST)" at 174px + cell padding 16 + margin
        size: 200,
        cell: ({ row }) => <DateTimeText value={row.original.createdAt} />,
      },
      {
        id: 'actions',
        header: t('table.actions'),
        // Measured content width (buttons + padding + 8px margin): English 225px, Chinese 163px. Measured on the
        // **degraded** variant: without edit permission this cell becomes "Account actions ⌄", wider than the usual
        // "Details | Edit + ⋯" (165 / 151); a fixed column width must fit it.
        size: widthFor({ default: 225, 'zh-CN': 163 }, i18n.language),
        cell: ({ row }) => (
          <RowActions
            actions={[
              {
                kind: 'button',
                key: 'view',
                label: t('action.detail'),
                onSelect: () => onView(row.original.id ?? ''),
              },
              // This row has seven or eight actions, a set of account maintenance actions: the main one, "Edit", sits
              // on the left to click directly, the rest go into "⋯". Without edit permission nothing else moves up (the
              // width is fixed and the names that would move up vary in length); the cell degrades itself into "Account
              // actions ⌄".
              {
                kind: 'split',
                key: 'manage',
                label: t('users.manage'),
                items: [
                  // A non-root user changes their own data in the profile, with no entry in user management; the server
                  // refuses too, so no button here that is refused only after filling it in.
                  ...(can('sys:account:update') &&
                  !selfBlocked(me, row.original)
                    ? [
                        {
                          key: 'edit',
                          label: t('action.edit'),
                          icon: <SquarePenIcon />,
                          primary: true,
                          onSelect: () => onEdit(row.original.id ?? ''),
                        },
                      ]
                    : []),
                  ...rowActions(can, me, row.original, t, onAction),
                ],
              },
            ]}
          />
        ),
      },
    ],
    [t, can, me, onView, onEdit, onAction, i18n.language],
  );
}

/**
 * A non-root user on their own row: user management gives no write entry at all. Changing one's own data, password or
 * authenticator goes through the profile, which asks for one's own credentials (the password), while this page asks for
 * permissions over others. root is exempt, but the anti-lockout exception still applies, see "Disable" below.
 */
function selfBlocked(
  me: { isRoot?: boolean; userId?: string },
  user: UserListItem,
): boolean {
  return !me.isRoot && user.id === me.userId;
}

/**
 * Row actions are filtered by permission one by one: **entries without permission are not rendered**, and page load
 * triggers no 403 (page gate = sys:account:view; loading sends only the list and options, none above it). Each confirms
 * and collects its code on its own: one-off actions are independent, not merged into a "batch action".
 */
function rowActions(
  can: (code?: PermCode) => boolean,
  me: { isRoot?: boolean; userId?: string },
  user: UserListItem,
  t: TFunction<'sys'>,
  onAction: (kind: PendingAction['kind'], user: UserListItem) => void,
) {
  const items: RowAction[] = [];
  if (selfBlocked(me, user)) return items;
  if (can('sys:account:reset_pwd')) {
    items.push({
      key: 'reset_pwd',
      icon: <KeyRoundIcon />,
      label: t('users.resetPassword'),
      onSelect: () => onAction('reset_pwd', user),
    });
  }
  // Nothing to reset if never bound: the entry would only teach that by a click.
  if (can('sys:account:reset_otp') && user.otpBound) {
    items.push({
      key: 'reset_otp',
      icon: <ShieldOffIcon />,
      label: t('users.resetOtp'),
      onSelect: () => onAction('reset_otp', user),
    });
  }
  // Disabling oneself is forbidden for everyone (root included): this is anti-lockout, not permission; once disabled
  // one cannot even log in, and the profile has no "enable myself".
  if (
    user.status === 1 &&
    can('sys:account:disable') &&
    user.id !== me.userId
  ) {
    items.push({
      key: 'disable',
      icon: <BanIcon />,
      label: t('users.disable'),
      onSelect: () => onAction('disable', user),
      tone: 'destructive',
    });
  }
  if (user.status !== 1 && can('sys:account:enable')) {
    items.push({
      key: 'enable',
      icon: <CircleCheckIcon />,
      label: t('users.enable'),
      onSelect: () => onAction('enable', user),
    });
  }
  if (can('sys:account:kick')) {
    items.push({
      key: 'kick',
      icon: <LogOutIcon />,
      label: t('users.kick'),
      onSelect: () => onAction('kick', user),
      tone: 'destructive',
    });
  }
  // Grant / revoke super admin: the endpoint is root-only and so is the entry; its gate differs from editing a user, so
  // it is a separate action that collects its own code in the dialog. The server forbids changing oneself, so one's own
  // row has no entry.
  if (me.isRoot && user.id !== me.userId) {
    items.push({
      key: 'root',
      icon: <ShieldIcon />,
      label: user.isRoot ? t('users.revokeRoot') : t('users.grantRoot'),
      onSelect: () => onAction('root', user),
      tone: user.isRoot ? undefined : 'destructive',
    });
  }
  return items;
}
