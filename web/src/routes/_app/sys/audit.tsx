import { useQuery } from '@tanstack/react-query';
import { createFileRoute } from '@tanstack/react-router';
import { useMemo, useState } from 'react';
import { useTranslation } from 'react-i18next';
import { ResourceSheet } from '@/components/containers';
import { DataRegion } from '@/components/data-region';
import {
  DataTable,
  type DataTableColumn,
  RowActions,
} from '@/components/data-table';
import { DateTimeText } from '@/components/fields';
import { FilterBar, type FilterOption } from '@/components/filter-bar';
import {
  Tooltip,
  TooltipContent,
  TooltipTrigger,
} from '@/components/ui/tooltip';
import { useList } from '@/hooks/use-list';
import {
  type AuditLogListItem,
  auditActionsQueryOptions,
  auditDetailQueryOptions,
  auditLogList,
} from '@/lib/audit';
import { widthFor } from '@/lib/table';

/**
 * Audit log: records left by successful operations bound to Audit. **Filters are in the URL**: copying the link lands
 * straight on "this person, this action, on 1 September" instead of three more clicks. Login attempts are on their own
 * page (login logs): the two lists differ in data, columns and filters, and each has its own view code.
 */
export const Route = createFileRoute('/_app/sys/audit')({
  staticData: { titleKey: 'nav.audit', perm: 'sys:audit:view' },
  ...auditLogList.route,
  component: AuditPage,
});

function AuditPage() {
  const { t } = useTranslation();
  const list = useList(auditLogList, Route);
  // Action options use the same view code as the page (the gate follows the purpose, admin-0008).
  const actions = useQuery(auditActionsQueryOptions);
  const groups = actions.data?.groups ?? [];
  // Opening and closing the details drawer is an action inside the page, not in the URL (container rules).
  const [viewing, setViewing] = useState<string>();

  // Action options grouped by target type: a long flat list of action names is unsearchable. "All" is added as the
  // first item by FilterBar; do not write it here.
  const actionOptions: FilterOption[] = useMemo(
    () =>
      groups.flatMap((g) =>
        (g.actions ?? []).map((a) => ({
          value: a.action ?? '',
          label: `${g.name || g.target} · ${a.name || a.action}`,
        })),
      ),
    [groups],
  );

  // Target type options come from the same action list: it is grouped by target type, one group per type, with names
  // already translated by the server in the request language.
  const targetOptions: FilterOption[] = useMemo(
    () =>
      groups.map((g) => ({
        value: g.target ?? '',
        label: g.name || g.target || '',
      })),
    [groups],
  );

  const columns = useAuditColumns(setViewing);

  return (
    <div className="flex flex-col gap-4">
      <DataTable<AuditLogListItem>
        tableKey="sys.audit"
        toolbar={
          <FilterBar
            fields={[
              {
                key: 'operatorKeyword',
                label: t('audit.operator'),
                type: 'input',
              },
              {
                key: 'action',
                label: t('audit.action'),
                type: 'select',
                options: actionOptions,
                load: actions,
              },
              {
                key: 'targetType',
                label: t('audit.targetType'),
                type: 'select',
                options: targetOptions,
                load: actions,
              },
              { key: 'targetId', label: t('audit.targetId'), type: 'input' },
              { key: 'ip', label: 'IP', type: 'input' },
              { key: 'created', label: t('audit.range'), type: 'dateRange' },
            ]}
            {...list.filterBar}
          />
        }
        columns={columns}
        pinning={{ start: ['id'], end: ['actions'] }}
        {...list.table}
      />
      <PayloadSheet id={viewing} onClose={() => setViewing(undefined)} />
    </div>
  );
}

function useAuditColumns(
  onView: (id: string) => void,
): DataTableColumn<AuditLogListItem>[] {
  const { t, i18n } = useTranslation();
  return useMemo(
    () => [
      { id: 'id', accessorKey: 'id', header: 'ID', size: 100 },
      {
        id: 'operator',
        header: t('audit.operator'),
        size: 160,
        // When the operator has been deleted the backend gives empty (audit outlives accounts); fall back to the ID.
        cell: ({ row }) => row.original.operatorName || row.original.operatorId,
      },
      // Action and target type show the names translated by the server in the request language; the code stays in the
      // tooltip, useful when matching logs or filtering by code. When an entry is missing the server already falls back
      // to the code.
      {
        id: 'action',
        header: t('audit.action'),
        size: 240,
        cell: ({ row }) => (
          <Tooltip>
            <TooltipTrigger render={<span />}>
              {row.original.actionName || row.original.action}
            </TooltipTrigger>
            <TooltipContent>{row.original.action}</TooltipContent>
          </Tooltip>
        ),
      },
      {
        id: 'targetType',
        header: t('audit.targetType'),
        size: 140,
        cell: ({ row }) => (
          <Tooltip>
            <TooltipTrigger render={<span />}>
              {row.original.targetName || row.original.targetType}
            </TooltipTrigger>
            <TooltipContent>{row.original.targetType}</TooltipContent>
          </Tooltip>
        ),
      },
      {
        id: 'targetId',
        accessorKey: 'targetId',
        header: t('audit.targetId'),
        size: 120,
      },
      { id: 'ip', accessorKey: 'ip', header: 'IP', size: 140 },
      {
        id: 'createdAt',
        header: t('audit.time'),
        size: 220,
        cell: ({ row }) => <DateTimeText value={row.original.createdAt} />,
      },
      {
        id: 'actions',
        header: t('table.actions'),
        // Measured content width (buttons + padding + 8px margin): English 88px, Chinese 72px; the longest set is
        // "Details"
        size: widthFor({ default: 88, 'zh-CN': 72 }, i18n.language),
        cell: ({ row }) => (
          <RowActions
            actions={[
              {
                kind: 'button',
                key: 'view',
                label: t('action.detail'),
                onSelect: () => onView(row.original.id ?? ''),
              },
            ]}
          />
        ),
      },
    ],
    [t, onView, i18n.language],
  );
}

/**
 * The request body of one audit record: fetched by ID from the detail endpoint (list rows carry no body). Valid JSON is
 * indented, invalid shown as is, empty gets a note; on failure only this block of the drawer shows "Failed to load".
 */
function PayloadSheet({ id, onClose }: { id?: string; onClose: () => void }) {
  const { t } = useTranslation();
  const query = useQuery(auditDetailQueryOptions(id ?? ''));
  const row = query.data;

  return (
    <ResourceSheet
      open={id !== undefined}
      onClose={onClose}
      size="wide"
      title={t('audit.payload')}
      description={
        row
          ? `${row.actionName || row.action} · ${row.targetName || row.targetType} ${row.targetId}`
          : undefined
      }
    >
      <DataRegion query={query}>
        {(detail) => (
          <div className="flex flex-col gap-4">
            {detail.reason ? (
              <div className="flex flex-col gap-1">
                <span className="text-muted-foreground text-sm">
                  {t('audit.reason')}
                </span>
                <p className="text-sm">{detail.reason}</p>
              </div>
            ) : null}
            <Payload raw={detail.requestPayload ?? ''} />
          </div>
        )}
      </DataRegion>
    </ResourceSheet>
  );
}

function Payload({ raw }: { raw: string }) {
  const { t } = useTranslation();
  let body = raw;
  try {
    body = raw ? JSON.stringify(JSON.parse(raw), null, 2) : '';
  } catch {
    // Not valid JSON: show it as is; that is itself the evidence to look at.
  }
  return body ? (
    <pre className="overflow-x-auto rounded-md bg-muted p-3 text-xs">
      {body}
    </pre>
  ) : (
    <p className="text-muted-foreground text-sm">{t('audit.noPayload')}</p>
  );
}
