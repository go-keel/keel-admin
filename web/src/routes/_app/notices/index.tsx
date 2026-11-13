// Example file: delete the whole file after forking

import { useQuery } from '@tanstack/react-query';
import { createFileRoute } from '@tanstack/react-router';
import type { TFunction } from 'i18next';
import { DownloadIcon } from 'lucide-react';
import { useMemo, useState } from 'react';
import { Controller } from 'react-hook-form';
import { useTranslation } from 'react-i18next';
import { z } from 'zod';
import { ActionDialog, FormDialog } from '@/components/containers';
import {
  DataTable,
  type DataTableColumn,
  RowActions,
} from '@/components/data-table';
import { DateTimeText } from '@/components/fields';
import { FilterBar } from '@/components/filter-bar';
import { PageHeader } from '@/components/page-header';
import { Button } from '@/components/ui/button';
import { Field, FieldError, FieldLabel } from '@/components/ui/field';
import { Input } from '@/components/ui/input';
import {
  Select,
  SelectContent,
  SelectGroup,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from '@/components/ui/select';
import { Textarea } from '@/components/ui/textarea';
import { useList } from '@/hooks/use-list';
import { usePerm } from '@/hooks/use-profile';
import { useZodForm } from '@/lib/form';
import { widthFor } from '@/lib/table';
import {
  batchDeleteNotices,
  categoryOptionsQueryOptions,
  createNotice,
  deleteNotice,
  exportNotices,
  type NoticeCategoryOption,
  type NoticeListItem,
  noticeList,
} from './-api';
import { categoryName, excerpt } from './-logic';

/**
 * Notice management page: the live example of a business area. What to copy: business permission codes in
 * `staticData.perm`, copy in the `app` namespace, API wrappers and page-private logic kept in place (`-` prefix), URL
 * parameters carry only filters, the create form uses FormDialog + useZodForm, a delete exposed on the row uses a
 * popover confirmation (RowAction's confirm). The page name is in the header bar, not written here.
 */
export const Route = createFileRoute('/_app/notices/')({
  staticData: { titleKey: 'app:notices.title', perm: 'ops:notice:view' },
  ...noticeList.route,
  component: NoticesPage,
});

function NoticesPage() {
  // The default namespace of a business page is app; shared kernel copy (actions, filters...) goes through tSys.
  const { t } = useTranslation('app');
  const { t: tSys } = useTranslation();
  const can = usePerm();
  const list = useList(noticeList, Route);
  // Creating is an action inside the page, not in the URL (container rules).
  const [creating, setCreating] = useState(false);
  // The ids of the batch awaiting delete confirmation. A single delete uses the row popover, so the page keeps no state
  // for it.
  const [batchDeleting, setBatchDeleting] = useState<string[]>();
  const [exporting, setExporting] = useState(false);
  const categoriesQuery = useQuery(categoryOptionsQueryOptions);
  const categories = categoriesQuery.data?.items ?? [];

  const columns = useNoticeColumns(categories);

  return (
    <>
      {can('ops:notice:create') ? (
        <PageHeader
          actions={
            <Button onClick={() => setCreating(true)}>
              {t('notices.create')}
            </Button>
          }
        />
      ) : null}
      <DataTable<NoticeListItem>
        tableKey="notices.list"
        toolbar={
          <FilterBar
            fields={[
              {
                key: 'categoryId',
                label: t('notices.category'),
                type: 'select',
                // "All" is added as the first item by FilterBar; do not write it here.
                options: categories.map((c) => ({
                  value: c.id ?? '',
                  label: c.name ?? '',
                })),
                load: categoriesQuery,
              },
              { key: 'keyword', label: t('notices.keyword'), type: 'input' },
              {
                key: 'created',
                label: t('notices.createdDate'),
                type: 'dateRange',
                zone: 'business',
              },
            ]}
            {...list.filterBar}
            // Export follows the filters, so it lives in the filter bar; "New", unrelated to filtering, is in the page
            // header.
            actions={
              <Button
                variant="outline"
                size="sm"
                disabled={exporting}
                onClick={async () => {
                  setExporting(true);
                  try {
                    await exportNotices(
                      list.search,
                      exportColumns(t, tSys, categories),
                    );
                  } finally {
                    setExporting(false);
                  }
                }}
              >
                <DownloadIcon />
                {t('notices.export')}
              </Button>
            }
          />
        }
        // Bulk action bar: **a business-area demo**. None of the five kernel sys pages has one: their actions (disable,
        // sign out, reset) each need confirmation and a code; they are not things done in batches.
        bulkActions={
          can('ops:notice:delete')
            ? (ids, clear) => (
                <>
                  <Button
                    variant="outline"
                    size="sm"
                    // Always shown, greyed out when nothing is selected: adding and removing it with the selection
                    // would make the table jump, and ticking boxes is a continuous action.
                    disabled={ids.length === 0}
                    onClick={() => setBatchDeleting(ids)}
                  >
                    {ids.length > 0
                      ? t('notices.batchDeleteN', { count: ids.length })
                      : t('notices.batchDelete')}
                  </Button>
                  <Button
                    variant="ghost"
                    size="sm"
                    disabled={ids.length === 0}
                    onClick={clear}
                  >
                    {tSys('table.clearSelection')}
                  </Button>
                </>
              )
            : undefined
        }
        columns={columns}
        pinning={{ start: ['id'], end: ['actions'] }}
        {...list.table}
      />

      {/* Batch delete confirms with a dialog: it must state how many will be deleted, and deleting a batch costs more
          than one. A single delete is a button exposed on the row and confirms with a popover; they are different
          actions, not a mix (container rules). */}
      <ActionDialog
        open={batchDeleting !== undefined}
        onOpenChange={(next) => next || setBatchDeleting(undefined)}
        title={t('notices.batchDelete')}
        description={t('notices.batchDeleteHint', {
          count: batchDeleting?.length ?? 0,
        })}
        destructive
        onConfirm={async () => {
          if (!batchDeleting) return;
          await batchDeleteNotices(batchDeleting);
        }}
      />
      <NoticeForm open={creating} onClose={() => setCreating(false)} />
    </>
  );
}

/** Matches the constraints of the server's CreateNoticeRequest; the server is still the last gate. */
const noticeSchema = z.object({
  title: z.string().trim().min(1).max(128),
  // The server requires a non-empty body: if the frontend does not block it, Save only earns a 400.
  content: z.string().trim().min(1),
  categoryId: z.string().min(1),
});

/** Create a notice. The owning department and creator are filled by the server from the operator; the form only
 * explains this. */
function NoticeForm({ open, onClose }: { open: boolean; onClose: () => void }) {
  const { t } = useTranslation('app');
  const form = useZodForm(noticeSchema, {
    open,
    defaultValues: { title: '', content: '', categoryId: '' },
  });
  // Options list only the **granted** categories: one cannot post to an invisible category, and listing it would only
  // fail on click. Not sent while the dialog is closed.
  const categoriesQuery = useQuery({
    ...categoryOptionsQueryOptions,
    enabled: open,
  });
  const categories = categoriesQuery.data?.items ?? [];

  return (
    <FormDialog
      open={open}
      onClose={onClose}
      title={t('notices.create')}
      form={form}
      queries={[categoriesQuery]}
      onSubmit={(v) => createNotice(v)}
    >
      <Controller
        name="title"
        control={form.control}
        render={({ field, fieldState }) => (
          <Field data-invalid={fieldState.invalid}>
            <FieldLabel htmlFor="notice-title">
              {t('notices.notice')}
            </FieldLabel>
            <Input
              {...field}
              id="notice-title"
              aria-required
              aria-invalid={fieldState.invalid}
            />
            {fieldState.invalid ? (
              <FieldError errors={[fieldState.error]} />
            ) : null}
          </Field>
        )}
      />
      <Controller
        name="categoryId"
        control={form.control}
        render={({ field, fieldState }) => (
          <Field data-invalid={fieldState.invalid}>
            <FieldLabel htmlFor="notice-category">
              {t('notices.category')}
            </FieldLabel>
            <Select
              value={field.value}
              onValueChange={(v) => field.onChange(v ?? '')}
              items={Object.fromEntries(
                categories.map((c) => [c.id ?? '', c.name]),
              )}
            >
              <SelectTrigger
                id="notice-category"
                className="w-full"
                aria-required
                aria-invalid={fieldState.invalid}
              >
                <SelectValue placeholder={t('notices.pickCategory')} />
              </SelectTrigger>
              <SelectContent>
                <SelectGroup>
                  {categories.map((c) => (
                    <SelectItem key={c.id} value={c.id ?? ''}>
                      {c.name}
                    </SelectItem>
                  ))}
                </SelectGroup>
              </SelectContent>
            </Select>
            {fieldState.invalid ? (
              <FieldError errors={[fieldState.error]} />
            ) : null}
          </Field>
        )}
      />
      <Controller
        name="content"
        control={form.control}
        render={({ field, fieldState }) => (
          <Field data-invalid={fieldState.invalid}>
            <FieldLabel htmlFor="notice-content">
              {t('notices.content')}
            </FieldLabel>
            <Textarea
              {...field}
              id="notice-content"
              rows={6}
              aria-required
              aria-invalid={fieldState.invalid}
            />
            {fieldState.invalid ? (
              <FieldError errors={[fieldState.error]} />
            ) : null}
          </Field>
        )}
      />
      <p className="text-muted-foreground text-xs">{t('notices.ownerHint')}</p>
    </FormDialog>
  );
}

/**
 * Export columns. **Deliberately not sharing one definition with the table columns**: table columns carry widths,
 * pinning and React cell renderers, while export needs only "a header + a plain value". Forcing one definition means
 * every new column has to ask "what does this look like in CSV", and for most columns the answer is "not in CSV".
 *
 * Content is not truncated (the table truncates it): getting the full content is the point of exporting.
 */
function exportColumns(
  t: TFunction<'app'>,
  tSys: TFunction<'sys'>,
  categories: readonly NoticeCategoryOption[],
): { label: string; value: (row: NoticeListItem) => unknown }[] {
  return [
    { label: 'ID', value: (r) => r.id },
    { label: t('notices.notice'), value: (r) => r.title },
    {
      label: t('notices.category'),
      value: (r) => categoryName(r.categoryId, categories),
    },
    { label: t('notices.content'), value: (r) => r.content },
    // Exports the API's UTC string as is, not converted to the header's time zone: once the CSV is in someone else's
    // hands, "the time zone selected at the time" neither travels with it nor has anywhere to be noted (DateTimeText
    // solves that with a suffix, a column of numbers cannot). UTC is the only self-explaining choice.
    { label: tSys('table.createdAt'), value: (r) => r.createdAt },
  ];
}

function useNoticeColumns(
  categories: { id?: string; name?: string }[],
): DataTableColumn<NoticeListItem>[] {
  const { t, i18n } = useTranslation('app');
  const { t: tSys } = useTranslation();
  const can = usePerm();

  return useMemo(
    () => [
      { id: 'id', accessorKey: 'id', header: 'ID', size: 90 },
      {
        id: 'title',
        accessorKey: 'title',
        header: t('notices.notice'),
        size: 240,
      },
      {
        id: 'category',
        header: t('notices.category'),
        size: 140,
        cell: ({ row }) => categoryName(row.original.categoryId, categories),
      },
      {
        id: 'content',
        header: t('notices.content'),
        size: 280,
        cell: ({ row }) => excerpt(row.original.content),
      },
      {
        id: 'ownerDept',
        header: t('notices.ownerDept'),
        size: 160,
        cell: ({ row }) =>
          row.original.ownerDeptName || `#${row.original.ownerDeptId}`,
      },
      {
        id: 'owner',
        header: t('notices.owner'),
        size: 140,
        cell: ({ row }) =>
          row.original.ownerUserName || `#${row.original.ownerUserId}`,
      },
      {
        id: 'createdAt',
        header: tSys('table.createdAt'),
        size: 220,
        cell: ({ row }) => <DateTimeText value={row.original.createdAt} />,
      },
      {
        id: 'actions',
        header: tSys('table.actions'),
        // Measured content width (buttons + padding + 8px margin): English 85px, Chinese 72px, Japanese 72px; only one
        // button, "Delete"
        size: widthFor(
          { default: 85, 'zh-CN': 72, 'ja-JP': 72 },
          i18n.language,
        ),
        cell: ({ row }) => (
          <RowActions
            actions={
              can('ops:notice:delete')
                ? [
                    {
                      kind: 'button',
                      key: 'delete',
                      label: tSys('action.delete'),
                      tone: 'destructive',
                      // A button exposed on the row confirms with a popover attached to it (container rules); if it is
                      // ever moved into the "⋯" menu, RowActions falls back to dialog confirmation automatically.
                      confirm: {
                        title: t('notices.delete'),
                        description: t('notices.deleteHint', {
                          title: row.original.title ?? '',
                        }),
                        onConfirm: () => deleteNotice(row.original.id ?? ''),
                      },
                    },
                  ]
                : []
            }
          />
        ),
      },
    ],
    [t, tSys, can, categories, i18n.language],
  );
}
