import { useQuery } from '@tanstack/react-query';
import { ChevronDownIcon, ChevronRightIcon, LockIcon } from 'lucide-react';
import { useMemo, useState } from 'react';
import { Controller } from 'react-hook-form';
import { useTranslation } from 'react-i18next';
import { z } from 'zod';
import {
  ActionDialog,
  FormDialog,
  ResourceSheet,
} from '@/components/containers';
import { DataRegion } from '@/components/data-region';
import { Section } from '@/components/fields';
import { Badge } from '@/components/ui/badge';
import { Button } from '@/components/ui/button';
import { Checkbox } from '@/components/ui/checkbox';
import {
  Collapsible,
  CollapsibleContent,
  CollapsibleTrigger,
} from '@/components/ui/collapsible';
import {
  Field,
  FieldError,
  FieldLabel,
  FieldLegend,
  FieldSet,
} from '@/components/ui/field';
import { Input } from '@/components/ui/input';
import { RadioGroup, RadioGroupItem } from '@/components/ui/radio-group';
import { Switch } from '@/components/ui/switch';
import {
  Tooltip,
  TooltipContent,
  TooltipTrigger,
} from '@/components/ui/tooltip';
import { useDetailForm } from '@/lib/form';
import { tDynamic } from '@/lib/i18n';
import {
  catalogOf,
  depsClosure,
  deselect,
  lockedMap,
  matchesQuery,
  selectWithDeps,
  toggleMany,
} from '@/lib/perm-deps';
import {
  assignableCandidatesQueryOptions,
  type DataScopeItem,
  deleteRole,
  deptTreeQueryOptions,
  type PermGroup,
  type PermissionDef,
  permissionsQueryOptions,
  type ResourceDef,
  type RoleFormValues,
  type RoleListItem,
  resourcesQueryOptions,
  roleDetailQueryOptions,
  saveRole,
} from '@/lib/roles';

const EMPTY: RoleFormValues = {
  code: '',
  name: '',
  status: 1,
  sensitive: 0,
  forceOtp: 0,
  perms: [],
  assignableRoleIds: [],
  manageDeptIds: [],
  scopes: [],
};

/**
 * Matches the constraints of the server's CreateRoleRequest / UpdateRoleRequest; the server is still the last gate. The
 * code can be set only on create; on edit the original value from the details goes through validation.
 */
const roleSchema = z.object({
  code: z.string().trim().min(1).max(64),
  name: z.string().trim().min(1).max(64),
  status: z.number().int(),
  sensitive: z.number().int(),
  forceOtp: z.number().int(),
  perms: z.array(z.string()),
  assignableRoleIds: z.array(z.string()),
  manageDeptIds: z.array(z.string()),
  scopes: z.array(z.custom<DataScopeItem>()),
});

/** All scope_types. Which are valid for a resource is decided by the resource's anchor (see the ResourceDef
 * comment). */
const SCOPE_TYPES = [
  'ALL',
  'DEPT',
  'ALL_SUB',
  'SELF',
  'RESOURCE_GRANT',
] as const;

const scopeAllowed = (res: ResourceDef, type: string): boolean => {
  if (type === 'ALL') return true;
  if (type === 'SELF') return Boolean(res.anchorSelf);
  if (type === 'DEPT' || type === 'ALL_SUB') return Boolean(res.anchorDept);
  if (type === 'RESOURCE_GRANT') return Boolean(res.grantable);
  return false;
};

/**
 * Role configuration dialog: wide size, stacked sections without tabs. Everything one submission changes must be
 * visible at once; with tabs comes the invisible state "something unfilled on another tab". One code for the whole
 * form.
 */
export function RoleForm({
  open,
  id,
  onClose,
}: {
  open: boolean;
  id?: string;
  onClose: () => void;
}) {
  const { t } = useTranslation();

  // Not sent while the dialog is closed: the whole permission catalogue should not be fetched to render a list page.
  const permsQuery = useQuery({ ...permissionsQueryOptions, enabled: open });
  const resourcesQuery = useQuery({ ...resourcesQueryOptions, enabled: open });
  const deptsQuery = useQuery({ ...deptTreeQueryOptions, enabled: open });
  const rolesQuery = useQuery({
    ...assignableCandidatesQueryOptions,
    enabled: open,
  });
  const perms = permsQuery.data;
  const resources = resourcesQuery.data?.items ?? [];
  const depts = deptsQuery.data?.items ?? [];
  const allRoles = rolesQuery.data?.items ?? [];

  // Full-replacement semantics: initial values must be fetched from details (useDetailForm), unchanged parts are sent
  // back as is.
  const { form, detail } = useDetailForm(roleSchema, {
    open,
    defaultValues: EMPTY,
    detail: id
      ? {
          query: roleDetailQueryOptions(id),
          toValues: (r) => ({
            code: r.code ?? '',
            name: r.name ?? '',
            status: r.status ?? 1,
            sensitive: r.sensitive ?? 0,
            forceOtp: r.forceOtp ?? 0,
            perms: r.perms ?? [],
            assignableRoleIds: (r.assignableRoles ?? []).map((x) => x.id ?? ''),
            manageDeptIds: (r.manageDepts ?? []).map((x) => x.id ?? ''),
            scopes: r.scopes ?? [],
          }),
        }
      : undefined,
  });

  // The permission tree, assignable roles, manageable departments and data scope are custom composite controls: they
  // read with watch, write with setValue and mark themselves dirty, so "Discard unsaved changes" recognises them.
  const [selectedPerms, assignable, manageDepts, scopes] = form.watch([
    'perms',
    'assignableRoleIds',
    'manageDeptIds',
    'scopes',
  ]);
  const dirty = { shouldDirty: true } as const;

  const setScope = (res: string, patch: Partial<DataScopeItem>) => {
    const rest = scopes.filter((s) => s.resourceType !== res);
    const cur = scopes.find((s) => s.resourceType === res) ?? {
      resourceType: res,
    };
    const next = { ...cur, ...patch };
    // No scope type selected = this resource has no scope, so remove the entry entirely rather than keeping an empty
    // one.
    form.setValue('scopes', next.scopeType ? [...rest, next] : rest, dirty);
  };

  return (
    <FormDialog
      open={open}
      onClose={onClose}
      size="wide"
      title={id ? t('roles.edit') : t('roles.create')}
      form={form}
      needOtp
      // Options and details together are this form's data: with any part missing it cannot be filled, so nobody ticks
      // boxes over incomplete options.
      queries={[detail, permsQuery, resourcesQuery, deptsQuery, rolesQuery]}
      onSubmit={(v, otp) => saveRole(id, v, otp)}
      // Creating adds a row, which the screen shows by itself; editing mostly changes permissions, assignable roles,
      // manageable departments or data scope, none shown in the list, so only edit speaks up (criterion in
      // docs/admin/forms.md, "Success messages").
      success={id ? t('roles.saved') : undefined}
    >
      <Section title={t('roles.basic')}>
        <Controller
          name="code"
          control={form.control}
          render={({ field, fieldState }) => (
            <Field data-invalid={fieldState.invalid}>
              <FieldLabel htmlFor="role-code">{t('roles.code')}</FieldLabel>
              <Input
                {...field}
                id="role-code"
                // The code is the role's identity and cannot be changed on edit: changing it means a different role.
                disabled={Boolean(id)}
                aria-required={!id}
                aria-invalid={fieldState.invalid}
              />
              {fieldState.invalid ? (
                <FieldError errors={[fieldState.error]} />
              ) : null}
            </Field>
          )}
        />
        <Controller
          name="name"
          control={form.control}
          render={({ field, fieldState }) => (
            <Field data-invalid={fieldState.invalid}>
              <FieldLabel htmlFor="role-name">{t('roles.name')}</FieldLabel>
              <Input
                {...field}
                id="role-name"
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
          name="forceOtp"
          control={form.control}
          render={({ field }) => (
            <SwitchRow
              id="role-force-otp"
              label={t('roles.forceOtp')}
              checked={field.value === 1}
              onChange={(c) => field.onChange(c ? 1 : 0)}
            />
          )}
        />
        <Controller
          name="sensitive"
          control={form.control}
          render={({ field }) => (
            <SwitchRow
              id="role-sensitive"
              label={t('roles.sensitive')}
              hint={t('roles.sensitiveHint')}
              checked={field.value === 1}
              onChange={(c) => field.onChange(c ? 1 : 0)}
            />
          )}
        />
        <Controller
          name="status"
          control={form.control}
          render={({ field }) => (
            <SwitchRow
              id="role-status"
              label={t('users.enabled')}
              checked={field.value === 1}
              onChange={(c) => field.onChange(c ? 1 : 0)}
            />
          )}
        />
      </Section>

      <Section title={t('roles.perms')} hint={t('roles.permsHint')}>
        <PermTree
          defs={perms?.items ?? []}
          groups={perms?.groups ?? []}
          selected={selectedPerms}
          onChange={(next) => form.setValue('perms', next, dirty)}
        />
      </Section>

      <Section title={t('roles.assignable')} hint={t('roles.assignableHint')}>
        <div className="grid grid-cols-2 gap-1">
          {allRoles
            // Exclude itself: a role being able to hand out itself is meaningless.
            .filter((x) => x.id !== id)
            .map((x) => (
              <CheckRow
                key={x.id}
                id={`assign-${x.id}`}
                label={x.name ?? ''}
                checked={assignable.includes(x.id ?? '')}
                onChange={(c) =>
                  form.setValue(
                    'assignableRoleIds',
                    c
                      ? [...assignable, x.id ?? '']
                      : assignable.filter((y) => y !== x.id),
                    dirty,
                  )
                }
              />
            ))}
        </div>
      </Section>

      <Section title={t('roles.manageDepts')} hint={t('roles.manageDeptsHint')}>
        <div className="grid grid-cols-2 gap-1">
          {depts.map((d) => (
            <CheckRow
              key={d.id}
              id={`dept-${d.id}`}
              label={d.name ?? ''}
              checked={manageDepts.includes(d.id ?? '')}
              onChange={(c) =>
                form.setValue(
                  'manageDeptIds',
                  c
                    ? [...manageDepts, d.id ?? '']
                    : manageDepts.filter((y) => y !== d.id),
                  dirty,
                )
              }
            />
          ))}
        </div>
      </Section>

      <Section title={t('roles.scopes')} hint={t('roles.scopesHint')}>
        {resources.map((res) => {
          const cur = scopes.find((s) => s.resourceType === res.res);
          return (
            <FieldSet key={res.res} className="gap-2 rounded-md border p-2">
              <FieldLegend variant="label">{res.name || res.res}</FieldLegend>
              <RadioGroup
                // "Not configured" uses none as a placeholder: the radio group needs a value meaning it is selected.
                value={cur?.scopeType || 'none'}
                onValueChange={(v) =>
                  setScope(
                    res.res ?? '',
                    v === 'none'
                      ? { scopeType: '' }
                      : { scopeType: String(v), resourceIds: [] },
                  )
                }
                className="flex flex-wrap gap-3"
              >
                {[
                  ...SCOPE_TYPES.filter((tp) => scopeAllowed(res, tp)),
                  'none' as const,
                ].map((tp) => (
                  <Field key={tp} orientation="horizontal" className="w-auto">
                    <RadioGroupItem value={tp} id={`scope-${res.res}-${tp}`} />
                    <FieldLabel
                      htmlFor={`scope-${res.res}-${tp}`}
                      className="font-normal text-xs"
                    >
                      {t(`roles.scope.${tp}`)}
                    </FieldLabel>
                  </Field>
                ))}
              </RadioGroup>
              {/* Explicit assignment is selectable only when the resource registered an option source; otherwise there
                  is nothing to pick. */}
              {cur?.scopeType === 'RESOURCE_GRANT' ? (
                <div className="grid grid-cols-2 gap-1 pt-1">
                  {(res.grantOptions ?? []).map((o) => (
                    <CheckRow
                      key={o.id}
                      id={`grant-${res.res}-${o.id}`}
                      label={o.label ?? ''}
                      checked={(cur.resourceIds ?? []).includes(o.id ?? '')}
                      onChange={(c) =>
                        setScope(res.res ?? '', {
                          resourceIds: c
                            ? [...(cur.resourceIds ?? []), o.id ?? '']
                            : (cur.resourceIds ?? []).filter((x) => x !== o.id),
                        })
                      }
                    />
                  ))}
                </div>
              ) : null}
            </FieldSet>
          );
        })}
      </Section>
    </FormDialog>
  );
}

function SwitchRow({
  id,
  label,
  hint,
  checked,
  onChange,
}: {
  id: string;
  label: string;
  hint?: string;
  checked: boolean;
  onChange: (checked: boolean) => void;
}) {
  return (
    <Field orientation="horizontal">
      <Switch id={id} checked={checked} onCheckedChange={onChange} />
      <Tooltip disabled={!hint}>
        <TooltipTrigger render={<FieldLabel htmlFor={id} />}>
          {label}
        </TooltipTrigger>
        <TooltipContent>{hint}</TooltipContent>
      </Tooltip>
    </Field>
  );
}

function CheckRow({
  id,
  label,
  checked,
  onChange,
}: {
  id: string;
  label: string;
  checked: boolean;
  onChange: (checked: boolean) => void;
}) {
  return (
    <div className="flex items-center gap-2 text-sm">
      <Checkbox
        id={id}
        checked={checked}
        onCheckedChange={(c) => onChange(Boolean(c))}
      />
      <FieldLabel htmlFor={id}>{label}</FieldLabel>
    </div>
  );
}

/**
 * Indeterminate state of group checkboxes. shadcn's Checkbox draws only the tick and has no indeterminate look (the
 * latest official version neither); Base UI sets `data-indeterminate` on the root when indeterminate, used here to draw
 * a solid dash.
 */
const HALF_CHECKED =
  'data-indeterminate:border-primary data-indeterminate:bg-primary data-indeterminate:text-primary-foreground data-indeterminate:[&_svg]:hidden data-indeterminate:before:h-0.5 data-indeterminate:before:w-2 data-indeterminate:before:rounded-full data-indeterminate:before:bg-current';

/**
 * Function permission tree: business domain → function module → function action. All three levels collapse, expanded by
 * default and forced open while searching. Business domains and modules are only groups, not permissions: their
 * checkboxes select the whole group (partial selection shows indeterminate), next to selected / total.
 *
 * Prerequisites (deps) are a different structure (one code may have several, which cannot be drawn as a tree),
 * expressed by the "Requires N" badge, auto-selection and locking.
 */
function PermTree({
  defs,
  groups,
  selected,
  onChange,
}: {
  defs: PermissionDef[];
  groups: PermGroup[];
  selected: string[];
  onChange: (next: string[]) => void;
}) {
  const { t, i18n } = useTranslation();
  // Search term and collapse state are not form data: closing the dialog unmounts the tree, and the next open starts
  // fresh.
  const [query, setQuery] = useState('');
  const [collapsed, setCollapsed] = useState<ReadonlySet<string>>(new Set());
  const catalog = useMemo(() => catalogOf(defs, groups), [defs, groups]);
  const locked = useMemo(() => lockedMap(selected, defs), [selected, defs]);

  const names = (codes: string[]) =>
    new Intl.ListFormat(i18n.language).format(
      codes.map((c) => defs.find((d) => d.code === c)?.name || c),
    );
  // Expand everything while searching: matches should not hide inside collapsed groups.
  const isOpen = (key: string) => query.trim() !== '' || !collapsed.has(key);
  const setOpen = (key: string, open: boolean) =>
    setCollapsed((prev) => {
      const next = new Set(prev);
      if (open) next.delete(key);
      else next.add(key);
      return next;
    });

  // Group ticks and counts cover the whole group, not the search matches: the checkbox stands for the group.
  const shown = catalog
    .map((dom) => ({
      ...dom,
      codes: dom.modules.flatMap((m) => m.defs.map((d) => d.code ?? '')),
      modules: dom.modules
        .map((m) => ({
          ...m,
          codes: m.defs.map((d) => d.code ?? ''),
          hits: m.defs.filter((d) => matchesQuery(d, query)),
        }))
        .filter((m) => m.hits.length > 0),
    }))
    .filter((dom) => dom.modules.length > 0);

  const header = (
    key: string,
    name: string,
    codes: string[],
    kernel = false,
  ) => {
    const picked = codes.filter((c) => selected.includes(c)).length;
    // Two cells: the collapse button takes the first grid column, everything else the second (see the grid below).
    return (
      <>
        <CollapsibleTrigger
          render={
            <Button
              type="button"
              variant="ghost"
              size="icon-xs"
              aria-label={t('roles.toggleGroup')}
            />
          }
        >
          {isOpen(key) ? <ChevronDownIcon /> : <ChevronRightIcon />}
        </CollapsibleTrigger>
        <div className="flex items-center gap-2">
          <Checkbox
            id={`perm-group-${key}`}
            checked={picked > 0 && picked === codes.length}
            indeterminate={picked > 0 && picked < codes.length}
            className={HALF_CHECKED}
            onCheckedChange={(c) =>
              onChange(toggleMany(selected, codes, Boolean(c), defs))
            }
          />
          <FieldLabel htmlFor={`perm-group-${key}`}>{name}</FieldLabel>
          {kernel ? (
            <Badge variant="secondary">{t('roles.kernel')}</Badge>
          ) : null}
          <span className="text-muted-foreground text-xs tabular-nums">
            {picked}/{codes.length}
          </span>
        </div>
      </>
    );
  };

  const leaf = (d: PermissionDef) => {
    const code = d.code ?? '';
    const lockedBy = locked.get(code);
    const needs = depsClosure(code, defs);
    return (
      <div key={code} className="flex items-center gap-2">
        <Checkbox
          id={`perm-${code}`}
          checked={selected.includes(code)}
          // Locked while another ticked code depends on it: the server expands it on grant anyway, so being able to
          // untick it in the UI would disagree with what actually takes effect.
          disabled={lockedBy !== undefined}
          onCheckedChange={(c) =>
            onChange(
              c
                ? selectWithDeps(selected, code, defs)
                : deselect(selected, code, defs),
            )
          }
        />
        <FieldLabel
          htmlFor={`perm-${code}`}
          className="flex-wrap gap-1.5 font-normal"
        >
          {/* Tooltips attach to each part, not the whole label: the name shows the full permission code, the badge and
              lock each say their own thing; inside the label's tooltip, hovering the badge would pop both at once. */}
          <Tooltip>
            <TooltipTrigger
              render={<span className="inline-flex flex-wrap gap-[inherit]" />}
            >
              {d.name || code}
              <span className="font-mono text-muted-foreground text-xs">
                {d.action || code}
              </span>
            </TooltipTrigger>
            <TooltipContent>{code}</TooltipContent>
          </Tooltip>
          {d.requireOtp ? (
            <Tooltip>
              <TooltipTrigger render={<Badge variant="outline" />}>
                {t('roles.permOtp')}
              </TooltipTrigger>
              <TooltipContent>{t('roles.permOtpHint')}</TooltipContent>
            </Tooltip>
          ) : null}
          {needs.length > 0 ? (
            <Tooltip>
              <TooltipTrigger render={<Badge variant="outline" />}>
                {t('roles.permNeeds', { n: needs.length })}
              </TooltipTrigger>
              <TooltipContent>
                {t('roles.permNeedsHint', { names: names(needs) })}
              </TooltipContent>
            </Tooltip>
          ) : null}
          {lockedBy ? (
            <Tooltip>
              <TooltipTrigger render={<span className="inline-flex" />}>
                <LockIcon
                  aria-hidden
                  className="size-3 text-muted-foreground"
                />
              </TooltipTrigger>
              <TooltipContent>
                {t('roles.lockedBy', { names: names(lockedBy) })}
              </TooltipContent>
            </Tooltip>
          ) : null}
        </FieldLabel>
      </div>
    );
  };

  return (
    <div className="flex flex-col gap-3">
      <Input
        value={query}
        placeholder={t('roles.permSearch')}
        onChange={(e) => setQuery(e.target.value)}
      />
      {shown.length === 0 ? (
        <p className="text-muted-foreground text-sm">{t('table.empty')}</p>
      ) : null}
      {/* Every level is a two-column grid: the first column's width comes from the collapse button, the next level goes
          into the second column, so the next level's collapse button aligns with this level's checkbox, and action
          checkboxes align with the module's. No indentation numbers, so it follows the button if its size changes. */}
      {shown.map((dom) => (
        <Collapsible
          key={dom.domain}
          open={isOpen(dom.domain)}
          onOpenChange={(o) => setOpen(dom.domain, o)}
          className="grid grid-cols-[auto_1fr] items-center gap-x-2 gap-y-2"
        >
          {/* sys is the business domain reserved by the kernel, a different kind of thing from business codes; marked
              at the domain level. */}
          {header(dom.domain, dom.name, dom.codes, dom.domain === 'sys')}
          <CollapsibleContent className="col-start-2 flex flex-col gap-2">
            {dom.modules.map((m) => {
              const key = `${dom.domain}:${m.module}`;
              return (
                <Collapsible
                  key={key}
                  open={isOpen(key)}
                  onOpenChange={(o) => setOpen(key, o)}
                  className="grid grid-cols-[auto_1fr] items-center gap-x-2 gap-y-1"
                >
                  {header(key, m.name, m.codes)}
                  <CollapsibleContent className="col-start-2 grid gap-x-4 gap-y-1 sm:grid-cols-2">
                    {m.hits.map(leaf)}
                  </CollapsibleContent>
                </Collapsible>
              );
            })}
          </CollapsibleContent>
        </Collapsible>
      ))}
    </div>
  );
}

/** Details drawer: permissions, the three boundary dimensions and data scope shown read-only, placeholders when
 * unset. */
export function RoleDetail({
  open,
  id,
  onClose,
}: {
  open: boolean;
  id?: string;
  onClose: () => void;
}) {
  const { t } = useTranslation();
  const detail = useQuery(roleDetailQueryOptions(id ?? ''));
  const data = detail.data;
  // Codes and resources are shown as names: "View departments" reads better than sys:dept:view; the code stays in the
  // tooltip. Not sent while the drawer is closed, as with the form.
  const permsQuery = useQuery({ ...permissionsQueryOptions, enabled: open });
  const resourcesQuery = useQuery({ ...resourcesQueryOptions, enabled: open });
  const perms = permsQuery.data;
  const resources = resourcesQuery.data?.items ?? [];
  const none = <span className="text-muted-foreground">{t('roles.none')}</span>;

  const granted = new Set(data?.perms ?? []);
  const catalog = catalogOf(perms?.items ?? [], perms?.groups ?? []);
  // Codes no longer in the registry (the derived repo removed a permission, the database still has it) are listed
  // anyway: details must not hide what has actually been granted.
  const known = new Set((perms?.items ?? []).map((d) => d.code));
  const unknown = [...granted].filter((c) => !known.has(c));

  return (
    <ResourceSheet
      open={open}
      onClose={onClose}
      size="wide"
      title={t('action.detail')}
    >
      <DataRegion query={detail} requires={[permsQuery, resourcesQuery]}>
        {(data) => (
          <div className="flex flex-col gap-4 text-sm">
            <Section title={t('roles.basic')}>
              <p>
                {t('format.withDetail', {
                  main: data.name ?? '',
                  detail: data.code ?? '',
                })}
              </p>
            </Section>
            <Section title={t('roles.perms')}>
              {granted.size === 0 ? (
                none
              ) : (
                <div className="flex flex-col gap-3">
                  {catalog.flatMap((dom) =>
                    dom.modules.map((m) => {
                      const hit = m.defs.filter((d) =>
                        granted.has(d.code ?? ''),
                      );
                      return hit.length === 0 ? null : (
                        <div
                          key={`${dom.domain}:${m.module}`}
                          className="flex flex-col gap-1.5"
                        >
                          <p className="text-muted-foreground text-xs">
                            {dom.name} · {m.name}
                          </p>
                          <div className="flex flex-wrap gap-1">
                            {hit.map((d) => (
                              <Tooltip key={d.code}>
                                <TooltipTrigger
                                  render={<Badge variant="secondary" />}
                                >
                                  {d.name || d.code}
                                </TooltipTrigger>
                                <TooltipContent>{d.code}</TooltipContent>
                              </Tooltip>
                            ))}
                          </div>
                        </div>
                      );
                    }),
                  )}
                  {unknown.length > 0 ? (
                    <div className="flex flex-wrap gap-1">
                      {unknown.map((c) => (
                        <Badge key={c} variant="outline">
                          {c}
                        </Badge>
                      ))}
                    </div>
                  ) : null}
                </div>
              )}
            </Section>
            <Section title={t('roles.assignable')}>
              {(data.assignableRoles ?? []).length === 0
                ? none
                : (data.assignableRoles ?? [])
                    .map((r) => r.name)
                    .join(t('format.listSeparator'))}
            </Section>
            <Section title={t('roles.manageDepts')}>
              {(data.manageDepts ?? []).length === 0
                ? none
                : (data.manageDepts ?? [])
                    .map((d) => d.name)
                    .join(t('format.listSeparator'))}
            </Section>
            <Section title={t('roles.scopes')}>
              {(data.scopes ?? []).length === 0
                ? none
                : (data.scopes ?? []).map((s) => {
                    const res = resources.find((r) => r.res === s.resourceType);
                    // Explicitly assigned instances look up their names in the resource's options; when not found
                    // (instance deleted) fall back to #id.
                    const label = (rid: string) =>
                      res?.grantOptions?.find((o) => o.id === rid)?.label ||
                      `#${rid}`;
                    return (
                      <p key={s.resourceType}>
                        {t('format.labeled', {
                          label: res?.name || s.resourceType || '',
                        })}
                        {s.resourceIds?.length
                          ? t('format.withDetail', {
                              main: tDynamic(
                                `roles.scope.${s.scopeType}`,
                                s.scopeType ?? '',
                              ),
                              detail: s.resourceIds
                                .map(label)
                                .join(t('format.listSeparator')),
                            })
                          : tDynamic(
                              `roles.scope.${s.scopeType}`,
                              s.scopeType ?? '',
                            )}
                      </p>
                    );
                  })}
            </Section>
          </div>
        )}
      </DataRegion>
    </ResourceSheet>
  );
}

/** Delete: dialog confirmation + code. Deleting a role revokes immediately, so a mistaken delete costs more than a
 * mistaken edit. */
export function RoleDeleteDialog({
  role,
  onClose,
}: {
  role?: RoleListItem;
  onClose: () => void;
}) {
  const { t } = useTranslation();

  return (
    <ActionDialog
      open={role !== undefined}
      onOpenChange={(next) => next || onClose()}
      title={t('roles.delete')}
      description={t('roles.deleteHint', { name: role?.name ?? '' })}
      needOtp
      destructive
      onConfirm={async (otp) => {
        if (!role) return;
        await deleteRole(role.id ?? '', otp);
      }}
    />
  );
}
