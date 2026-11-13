import { useQuery } from '@tanstack/react-query';
import { useRouter } from '@tanstack/react-router';
import { ExternalLinkIcon } from 'lucide-react';
import { useEffect, useState } from 'react';
import { Controller } from 'react-hook-form';
import { useTranslation } from 'react-i18next';
import { z } from 'zod';
import {
  ActionDialog,
  FormDialog,
  PasswordInput,
  ResourceSheet,
} from '@/components/containers';
import { DataRegion } from '@/components/data-region';
import {
  AvatarField,
  DateTimeText,
  DetailRow,
  LoginAttempts,
  Section,
} from '@/components/fields';
import { Badge } from '@/components/ui/badge';
import { Button } from '@/components/ui/button';
import { Checkbox } from '@/components/ui/checkbox';
import {
  Field,
  FieldDescription,
  FieldError,
  FieldLabel,
  FieldLegend,
  FieldSet,
} from '@/components/ui/field';
import { Input } from '@/components/ui/input';
import {
  Select,
  SelectContent,
  SelectGroup,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from '@/components/ui/select';
import { usePerm, useProfile } from '@/hooks/use-profile';
import { loginLogList } from '@/lib/audit';
import {
  ipWhitelist,
  optionalEmail,
  optionalPhone,
  useDetailForm,
} from '@/lib/form';
import {
  assignableRolesQueryOptions,
  deptOptionsQueryOptions,
  resetPassword,
  saveUser,
  setUserRoot,
  submittableRoleIds,
  type UserListItem,
  userAction,
  userDetailQueryOptions,
} from '@/lib/users';

/** Page-private code stays in place (the `-` prefix of TanStack file routes), not in kernel directories. */

/**
 * Matches the constraints of the server's CreateUserRequest / UpdateUserRequest; the server is still the last gate.
 * Username and initial password are filled only on create, hence one schema each for create and edit.
 */
const userFields = {
  nickname: z.string().trim().min(1).max(64),
  avatar: z.string(),
  email: optionalEmail(),
  phone: optionalPhone(),
  deptId: z.string(),
  roleIds: z.array(z.string()).min(1),
  ipWhitelist: ipWhitelist(),
};

const createUserSchema = z.object({
  username: z.string().trim().min(1).max(64),
  password: z.string().min(1).max(128),
  ...userFields,
});

const editUserSchema = z.object({
  username: z.string(),
  password: z.string(),
  ...userFields,
});

type UserValues = z.infer<typeof createUserSchema>;

const EMPTY: UserValues = {
  username: '',
  password: '',
  nickname: '',
  avatar: '',
  email: '',
  phone: '',
  deptId: '0',
  roleIds: [],
  ipWhitelist: '',
};

/**
 * Create and edit share **one form and one dialog**: what the same people with the same gate can change is merged into
 * one submission (see ADR "operation split and step-up placement"). One step-up for the whole form. Granting / revoking
 * super admin has a different gate (root only), so it is not in this form but a separate action in the row's "⋯" menu.
 */
export function UserForm({
  open,
  id,
  onClose,
}: {
  open: boolean;
  id?: string;
  onClose: () => void;
}) {
  const { t } = useTranslation();
  const can = usePerm();

  // Not sent while the dialog is closed: only the form uses these options.
  const deptsQuery = useQuery({ ...deptOptionsQueryOptions, enabled: open });
  const depts = deptsQuery.data?.items ?? [];
  // ListAssignableRoleOptions is bound to sys:account:update, while this page's entry gate is only sys:account:view.
  // Sending it unconditionally would give a view-only user a 403 on entering the page: "a request sent on page load
  // must not have a higher gate than the page's own entry gate" (admin-0008).
  const rolesQuery = useQuery({
    ...assignableRolesQueryOptions,
    enabled: open && can('sys:account:update'),
  });
  const roles = rolesQuery.data?.items ?? [];

  // Initial values come from the detail endpoint (useDetailForm), not the list row snapshot, which would silently write
  // back others' concurrent changes.
  const schema: z.ZodType<UserValues, UserValues> = id
    ? editUserSchema
    : createUserSchema;
  const { form, detail } = useDetailForm(schema, {
    open,
    defaultValues: EMPTY,
    detail: id
      ? {
          query: userDetailQueryOptions(id),
          toValues: (u) => ({
            username: u.username ?? '',
            password: '',
            nickname: u.nickname ?? '',
            avatar: u.avatar ?? '',
            email: u.email ?? '',
            phone: u.phone ?? '',
            deptId: u.deptId ?? '0',
            roleIds: (u.roles ?? []).map((r) => r.id ?? ''),
            ipWhitelist: u.ipWhitelist ?? '',
          }),
        }
      : undefined,
  });

  const submit = (v: UserValues, otp: string) =>
    saveUser(id, { ...v, roleIds: submittableRoleIds(v.roleIds, roles) }, otp);

  return (
    <FormDialog
      open={open}
      onClose={onClose}
      title={id ? t('users.edit') : t('users.create')}
      form={form}
      needOtp
      queries={[detail, deptsQuery, rolesQuery]}
      onSubmit={submit}
    >
      <Section title={t('users.basic')}>
        <Controller
          name="username"
          control={form.control}
          render={({ field, fieldState }) => (
            <Field data-invalid={fieldState.invalid}>
              <FieldLabel htmlFor="username">{t('users.username')}</FieldLabel>
              <Input
                {...field}
                id="username"
                // The username can be set on create but not changed on edit: it is the account's identity; changing it
                // means a different person.
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
        {id ? null : (
          <Controller
            name="password"
            control={form.control}
            render={({ field, fieldState }) => (
              <Field data-invalid={fieldState.invalid}>
                <FieldLabel htmlFor="password">
                  {t('users.initialPassword')}
                </FieldLabel>
                <PasswordInput
                  {...field}
                  id="password"
                  autoComplete="new-password"
                  aria-required
                  aria-invalid={fieldState.invalid}
                />
                {fieldState.invalid ? (
                  <FieldError errors={[fieldState.error]} />
                ) : null}
              </Field>
            )}
          />
        )}
        <Controller
          name="nickname"
          control={form.control}
          render={({ field, fieldState }) => (
            <Field data-invalid={fieldState.invalid}>
              <FieldLabel htmlFor="nickname">{t('users.nickname')}</FieldLabel>
              <Input
                {...field}
                id="nickname"
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
          name="avatar"
          control={form.control}
          render={({ field }) => (
            <AvatarField value={field.value} onChange={field.onChange} />
          )}
        />
        <Controller
          name="email"
          control={form.control}
          render={({ field, fieldState }) => (
            <Field data-invalid={fieldState.invalid}>
              <FieldLabel htmlFor="email">{t('field.email')}</FieldLabel>
              <Input
                {...field}
                id="email"
                type="email"
                autoComplete="off"
                aria-invalid={fieldState.invalid}
              />
              {fieldState.invalid ? (
                <FieldError errors={[fieldState.error]} />
              ) : null}
            </Field>
          )}
        />
        <Controller
          name="phone"
          control={form.control}
          render={({ field, fieldState }) => (
            <Field data-invalid={fieldState.invalid}>
              <FieldLabel htmlFor="phone">{t('field.phone')}</FieldLabel>
              <Input
                {...field}
                id="phone"
                type="tel"
                autoComplete="off"
                aria-invalid={fieldState.invalid}
              />
              {fieldState.invalid ? (
                <FieldError errors={[fieldState.error]} />
              ) : null}
            </Field>
          )}
        />
      </Section>

      <Section title={t('users.access')} hint={t('users.accessHint')}>
        <Controller
          name="deptId"
          control={form.control}
          render={({ field }) => (
            <Field>
              <FieldLabel htmlFor="deptId">{t('users.dept')}</FieldLabel>
              <Select
                value={field.value}
                onValueChange={(v) => field.onChange(v ?? '0')}
                items={{
                  '0': t('users.noDept'),
                  ...Object.fromEntries(depts.map((d) => [d.id ?? '', d.name])),
                }}
              >
                <SelectTrigger id="deptId" className="w-full">
                  <SelectValue />
                </SelectTrigger>
                <SelectContent>
                  <SelectGroup>
                    <SelectItem value="0">{t('users.noDept')}</SelectItem>
                    {depts.map((d) => (
                      <SelectItem
                        key={d.id}
                        value={d.id ?? ''}
                        disabled={d.disabled}
                      >
                        {d.name}
                      </SelectItem>
                    ))}
                  </SelectGroup>
                </SelectContent>
              </Select>
            </Field>
          )}
        />

        <Controller
          name="roleIds"
          control={form.control}
          render={({ field, fieldState }) => (
            <FieldSet data-required data-invalid={fieldState.invalid}>
              <FieldLegend variant="label">{t('users.roles')}</FieldLegend>
              {/* Only roles **I can assign** (ListAssignableRoleOptions); the filter uses the full option set, each
                  fits its own purpose. */}
              {roles.map((r) => (
                <div key={r.id} className="flex items-center gap-2 text-sm">
                  <Checkbox
                    id={`role-${r.id}`}
                    checked={field.value.includes(r.id ?? '')}
                    onCheckedChange={(checked) =>
                      field.onChange(
                        checked
                          ? [...field.value, r.id ?? '']
                          : field.value.filter((x) => x !== r.id),
                      )
                    }
                  />
                  <FieldLabel htmlFor={`role-${r.id}`}>{r.name}</FieldLabel>
                </div>
              ))}
              {fieldState.invalid ? (
                <FieldError errors={[fieldState.error]} />
              ) : null}
            </FieldSet>
          )}
        />

        <Controller
          name="ipWhitelist"
          control={form.control}
          render={({ field, fieldState }) => (
            <Field data-invalid={fieldState.invalid}>
              <FieldLabel htmlFor="ip-whitelist">
                {t('field.ipWhitelist')}
              </FieldLabel>
              <Input
                {...field}
                id="ip-whitelist"
                placeholder="10.0.0.0/8, 203.0.113.7"
                aria-invalid={fieldState.invalid}
              />
              {/* Empty = unrestricted (the state of nearly every account), so not marked required; only a bad entry is
                  reported, naming which one. */}
              {fieldState.invalid ? (
                <FieldError errors={[fieldState.error]} />
              ) : (
                <FieldDescription>{t('field.ipHint')}</FieldDescription>
              )}
            </Field>
          )}
        />
      </Section>
    </FormDialog>
  );
}

/** Details drawer: read-only, shows this account's boundaries without entering edit. */
export function UserDetail({
  open,
  id,
  onClose,
}: {
  open: boolean;
  id?: string;
  onClose: () => void;
}) {
  const { t } = useTranslation();
  const can = usePerm();
  const me = useProfile();
  const detail = useQuery(userDetailQueryOptions(id ?? ''));

  return (
    <ResourceSheet open={open} onClose={onClose} title={t('action.detail')}>
      <DataRegion query={detail}>
        {(data) => (
          <dl className="flex flex-col gap-3 text-sm">
            <DetailRow label={t('users.username')} value={data.username} />
            <DetailRow label={t('users.nickname')} value={data.nickname} />
            <DetailRow label={t('field.email')} value={data.email} />
            <DetailRow label={t('field.phone')} value={data.phone} />
            <DetailRow label={t('users.dept')} value={data.deptName} />
            <DetailRow
              label={t('users.roles')}
              value={
                <div className="flex flex-wrap gap-1">
                  {/* Same rule as the list: the super admin badge is shown only to root. */}
                  {me.isRoot && data.isRoot ? (
                    <Badge>{t('users.root')}</Badge>
                  ) : null}
                  {(data.roles ?? []).map((r) => (
                    <Badge
                      key={r.id}
                      variant={r.disabled ? 'outline' : 'secondary'}
                    >
                      {r.name}
                      {r.disabled ? ` · ${t('users.disabled')}` : null}
                    </Badge>
                  ))}
                </div>
              }
            />
            <DetailRow
              label={t('users.status')}
              value={
                data.status === 1 ? t('users.enabled') : t('users.disabled')
              }
            />
            <DetailRow
              label={t('users.otp')}
              value={
                data.otpBound ? t('users.otpBound') : t('users.otpUnbound')
              }
            />
            <DetailRow
              label={t('field.ipWhitelist')}
              value={data.ipWhitelist || '-'}
            />
            <DetailRow
              label={t('table.createdAt')}
              value={<DateTimeText value={data.createdAt} />}
            />
          </dl>
        )}
      </DataRegion>
      {/* Login logs have their own view code: without it this section is not rendered, so opening the drawer does not
          get a 403. */}
      {detail.data && id && can('sys:login_log:view') ? (
        <RecentLogins userId={id} />
      ) : null}
    </ResourceSheet>
  );
}

const RECENT_LOGINS = 5;

/**
 * This account's latest logins. The drawer holds only the first screenful; for paging go to the login log page, a real
 * link in a new browser tab, so neither the current page's filters nor this drawer get lost (container rules). Filtered
 * exactly by account ID: username matching is fuzzy, and "zhang" would also bring in "zhangsan".
 */
function RecentLogins({ userId }: { userId: string }) {
  const { t } = useTranslation();
  const router = useRouter();
  const logins = useQuery(
    loginLogList.queryOptions({ userId }, 1, RECENT_LOGINS),
  );

  return (
    <Section title={t('users.recentLogins')}>
      <DataRegion query={logins}>
        {(data) => <LoginAttempts items={data.items ?? []} />}
      </DataRegion>
      <Button
        variant="link"
        size="sm"
        className="self-start px-0"
        nativeButton={false}
        render={
          <a
            // Generated by the router from the login log page's definition; a wrong key fails to compile, so do not
            // hand-build it.
            href={
              router.buildLocation({
                to: '/sys/login-logs',
                search: { userId },
              }).href
            }
            target="_blank"
            rel="noopener"
          />
        }
      >
        {t('action.viewAll')}
        <ExternalLinkIcon />
      </Button>
    </Section>
  );
}

export type PendingAction = {
  kind: 'reset_pwd' | 'reset_otp' | 'disable' | 'enable' | 'kick' | 'root';
  user: UserListItem;
};

/**
 * Dialog for one-off actions. Each confirms and collects its code on its own: they are independent, and merging them
 * into one "batch action" would let one confirmation cover several different things. `root` is grant / revoke super
 * admin, the direction depending on whether the target currently is one.
 */
export function UserActionDialog({
  pending,
  onClose,
}: {
  pending?: PendingAction;
  onClose: () => void;
}) {
  const { t } = useTranslation();
  const [password, setPassword] = useState('');

  // Switching action clears the new password typed last time: keeping it would fill one account's password into
  // another.
  useEffect(() => {
    if (pending) setPassword('');
  }, [pending]);

  const run = async (otp: string) => {
    if (!pending) return;
    const id = pending.user.id ?? '';
    if (pending.kind === 'reset_pwd') {
      await resetPassword(id, password, otp);
    } else if (pending.kind === 'root') {
      await setUserRoot(id, !pending.user.isRoot, otp);
    } else {
      await userAction(id, pending.kind, otp);
    }
  };

  const name = pending?.user.username ?? '';
  const revoking = Boolean(pending?.user.isRoot);
  // Speak up only when the screen shows no change (criterion in docs/admin/forms.md, "Success messages"): resetting the
  // password, signing out, changing super admin status change no column; disable/enable has the status column,
  // resetting the authenticator its column, so no message.
  const success =
    pending?.kind === 'reset_pwd'
      ? t('users.pwdReset', { name })
      : pending?.kind === 'kick'
        ? t('users.kicked', { name })
        : pending?.kind === 'root'
          ? t(revoking ? 'users.rootRevoked' : 'users.rootGranted', { name })
          : undefined;
  return (
    <ActionDialog
      open={pending !== undefined}
      onOpenChange={(next) => next || onClose()}
      title={
        pending?.kind === 'root'
          ? t(revoking ? 'users.revokeRoot' : 'users.grantRoot')
          : t(`users.${pending?.kind ?? 'kick'}`)
      }
      description={
        pending?.kind === 'root'
          ? t(revoking ? 'users.revokeRootHint' : 'users.grantRootHint', {
              name,
            })
          : t('users.actionOn', { name })
      }
      needOtp
      // Granting super admin is the ultimate privilege escalation, so it uses the warning colour like disable and sign
      // out; revoking takes it back and need not.
      destructive={
        pending?.kind === 'disable' ||
        pending?.kind === 'kick' ||
        (pending?.kind === 'root' && !revoking)
      }
      confirmDisabled={pending?.kind === 'reset_pwd' && password.length < 8}
      success={success}
      onConfirm={run}
    >
      {pending?.kind === 'reset_pwd' ? (
        <Field>
          <FieldLabel htmlFor="new-password">
            {t('users.newPassword')}
          </FieldLabel>
          <PasswordInput
            id="new-password"
            autoComplete="new-password"
            value={password}
            onChange={(e) => setPassword(e.target.value)}
          />
        </Field>
      ) : null}
    </ActionDialog>
  );
}
