import { Controller } from 'react-hook-form';
import { useTranslation } from 'react-i18next';
import { z } from 'zod';
import { ActionDialog, FormDialog } from '@/components/containers';
import { Field, FieldError, FieldLabel } from '@/components/ui/field';
import { Input } from '@/components/ui/input';
import { Switch } from '@/components/ui/switch';
import {
  type DeptFormValues,
  type DeptListItem,
  deleteDept,
  deptDetailQueryOptions,
  saveDept,
} from '@/lib/depts';
import { useDetailForm } from '@/lib/form';

const EMPTY: DeptFormValues = {
  parentId: '0',
  name: '',
  sort: 0,
  status: 1,
};

/** Matches the constraints of the server's CreateDeptRequest / UpdateDeptRequest; the server is still the last gate. */
const deptSchema = z.object({
  parentId: z.string(),
  name: z.string().trim().min(1).max(64),
  sort: z.number().int(),
  status: z.number().int(),
});

/** New root department / new sub-department / edit share one form and one dialog; all use step-up. */
export function DeptForm({
  open,
  id,
  parent,
  onClose,
}: {
  open: boolean;
  id?: string;
  /** The parent when creating a sub-department: prefills parentId and puts its name in the title. */
  parent?: DeptListItem;
  onClose: () => void;
}) {
  const { t } = useTranslation();
  // Initial values are fetched from the detail endpoint (useDetailForm), not taken from the list row snapshot.
  const { form, detail } = useDetailForm(deptSchema, {
    open,
    defaultValues: { ...EMPTY, parentId: parent?.id ?? '0' },
    detail: id
      ? {
          query: deptDetailQueryOptions(id),
          toValues: (d) => ({
            parentId: d.parentId ?? '0',
            name: d.name ?? '',
            sort: d.sort ?? 0,
            status: d.status ?? 1,
          }),
        }
      : undefined,
  });

  const title = id
    ? t('depts.edit')
    : parent
      ? t('depts.createChildOf', { name: parent.name ?? '' })
      : t('depts.createRoot');

  return (
    <FormDialog
      open={open}
      onClose={onClose}
      title={title}
      form={form}
      needOtp
      queries={[detail]}
      onSubmit={(v, otp) => saveDept(id, v, otp)}
    >
      <Controller
        name="name"
        control={form.control}
        render={({ field, fieldState }) => (
          <Field data-invalid={fieldState.invalid}>
            <FieldLabel htmlFor="dept-name">{t('depts.name')}</FieldLabel>
            <Input
              {...field}
              id="dept-name"
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
        name="sort"
        control={form.control}
        render={({ field, fieldState }) => (
          <Field data-invalid={fieldState.invalid}>
            <FieldLabel htmlFor="dept-sort">{t('depts.sort')}</FieldLabel>
            <Input
              id="dept-sort"
              type="number"
              name={field.name}
              ref={field.ref}
              onBlur={field.onBlur}
              value={String(field.value)}
              onChange={(e) => field.onChange(Number(e.target.value) || 0)}
              aria-invalid={fieldState.invalid}
            />
            {fieldState.invalid ? (
              <FieldError errors={[fieldState.error]} />
            ) : null}
          </Field>
        )}
      />
      {/* Enabled status is editable only when editing: the create endpoint has no such field (enabled by default). */}
      {id ? (
        <Controller
          name="status"
          control={form.control}
          render={({ field }) => (
            <Field orientation="horizontal">
              <Switch
                id="dept-status"
                checked={field.value === 1}
                onCheckedChange={(c) => field.onChange(c ? 1 : 0)}
              />
              <FieldLabel htmlFor="dept-status">
                {t('users.enabled')}
              </FieldLabel>
            </Field>
          )}
        />
      ) : null}
    </FormDialog>
  );
}

/**
 * Delete. **The frontend does not pre-judge "has sub-departments"**: that would hand-copy the server's check, and the
 * two copies are bound to drift (docs/admin/permission-model/README.md §3). The server's localised message gives the reason for a refusal, and the
 * global rule toasts it.
 */
export function DeptDeleteDialog({
  dept,
  onClose,
}: {
  dept?: DeptListItem;
  onClose: () => void;
}) {
  const { t } = useTranslation();

  return (
    <ActionDialog
      open={dept !== undefined}
      onOpenChange={(next) => next || onClose()}
      title={t('depts.delete')}
      description={t('depts.deleteHint', { name: dept?.name ?? '' })}
      needOtp
      destructive
      onConfirm={async (otp) => {
        if (!dept) return;
        await deleteDept(dept.id ?? '', otp);
      }}
    />
  );
}
