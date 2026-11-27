import { useQuery } from '@tanstack/react-query';
import { type ReactNode, useState } from 'react';
import { Controller } from 'react-hook-form';
import { useTranslation } from 'react-i18next';
import { z } from 'zod';
import {
  ActionDialog,
  FormDialog,
  KeepLastOpen,
  SectionDialog,
  SectionForm,
} from '@/components/containers';
import { PasswordInput } from '@/components/credential-inputs';
import { DataRegion } from '@/components/data-region';
import {
  AvatarField,
  DateTimeText,
  IpTags,
  LoginAttempts,
  StatusBadge,
} from '@/components/fields';
import { OtpBindPanel } from '@/components/otp-bind';
import { Avatar, AvatarFallback, AvatarImage } from '@/components/ui/avatar';
import { Badge } from '@/components/ui/badge';
import { Button } from '@/components/ui/button';
import {
  Dialog,
  DialogContent,
  DialogHeader,
  DialogTitle,
} from '@/components/ui/dialog';
import { Field, FieldError, FieldLabel } from '@/components/ui/field';
import { Input } from '@/components/ui/input';
import { Separator } from '@/components/ui/separator';
import { useProfile } from '@/hooks/use-profile';
import {
  optionalEmail,
  optionalPhone,
  samePassword,
  useDetailForm,
  useZodForm,
} from '@/lib/form';
import {
  changeMyPassword,
  myAccountFreshQueryOptions,
  myAccountQueryOptions,
  refreshProfile,
  unbindOtp,
  updateMyProfile,
} from '@/lib/me';

/**
 * The profile: a section dialog opened from the avatar menu in the header, the single entry to one's own account
 * (docs/admin/forms.md, "Container roles"). Two independent sections: "Basic info" is a form edited in place, saved on
 * its own; "Security" is rows of actions (password and authenticator each open their own dialog: they affect signing
 * in and need a code or a password) and read-only items. A new setting is one more section or row. Not in the URL:
 * nothing links to it, and a refresh closes it like any other dialog.
 */
export function ProfileDialog({
  open,
  onClose,
}: {
  open: boolean;
  onClose: () => void;
}) {
  const { t } = useTranslation();
  return (
    <SectionDialog
      open={open}
      onClose={onClose}
      title={t('account.profile')}
      sections={[
        { id: 'base', title: t('profile.base'), content: <BaseSection /> },
        {
          id: 'security',
          title: t('profile.security'),
          content: <SecuritySection />,
        },
      ]}
    />
  );
}

type MyAccount = NonNullable<ReturnType<typeof useMyAccount>['data']>;
const useMyAccount = () => useQuery(myAccountQueryOptions);

/** Each section reads the account in its own data region, so a failed fetch fails only that section. */
function WithAccount({ children }: { children: (a: MyAccount) => ReactNode }) {
  const account = useMyAccount();
  return <DataRegion query={account}>{children}</DataRegion>;
}

function BaseSection() {
  const { t } = useTranslation();
  const me = useProfile();
  // Identity on top (large avatar + nickname, username in grey), then the form, then what only an administrator can
  // change, read-only.
  return (
    <WithAccount>
      {(a) => (
        <div className="flex flex-col gap-4">
          <div className="flex items-center gap-4">
            <Avatar className="size-14">
              {a.avatar ? <AvatarImage src={a.avatar} alt="" /> : null}
              <AvatarFallback />
            </Avatar>
            <div className="flex min-w-0 flex-col gap-1">
              <p className="truncate font-heading font-semibold text-lg">
                {a.nickname || a.username}
              </p>
              <p className="truncate text-muted-foreground text-sm">
                {a.username}
              </p>
            </div>
          </div>
          <Separator />
          <BaseForm />
          <Separator />
          <dl className="grid gap-4 sm:grid-cols-2">
            <InfoItem label={t('users.dept')} value={a.deptName} />
            <InfoItem
              label={t('table.createdAt')}
              value={<DateTimeText value={a.createdAt} />}
            />
            <InfoItem
              label={t('users.roles')}
              value={
                // Root holds no roles and needs none: say so instead of an empty "-", which reads as "no rights".
                me.isRoot || (a.roles ?? []).length > 0 ? (
                  <div className="flex flex-wrap gap-1">
                    {me.isRoot ? <Badge>{t('users.root')}</Badge> : null}
                    {(a.roles ?? []).map((r) => (
                      <Badge key={r.id} variant="secondary">
                        {r.name}
                      </Badge>
                    ))}
                  </div>
                ) : null
              }
            />
          </dl>
        </div>
      )}
    </WithAccount>
  );
}

/** One read-only item: small label above, value below; shows "—" when there is no value. */
function InfoItem({ label, value }: { label: string; value: ReactNode }) {
  return (
    <div className="flex min-w-0 flex-col gap-1">
      <dt className="text-muted-foreground text-xs">{label}</dt>
      <dd className="break-words text-sm">{value || '-'}</dd>
    </div>
  );
}

/** Matches the constraints of the server's UpdateMyProfileRequest. */
const baseSchema = z.object({
  nickname: z.string().trim().min(1).max(64),
  avatar: z.string(),
  email: optionalEmail(),
  phone: optionalPhone(),
});

/** Change nickname, avatar, email, phone, in place. The username is the account's identity; even its owner cannot
 * change it, so it is not in the form. Initial values are fetched fresh each time the section is shown. */
function BaseForm() {
  const { t } = useTranslation();
  const { form, detail: detailQuery } = useDetailForm(baseSchema, {
    open: true,
    defaultValues: { nickname: '', avatar: '', email: '', phone: '' },
    detail: {
      query: myAccountFreshQueryOptions,
      toValues: (d) => ({
        nickname: d.nickname ?? '',
        avatar: d.avatar ?? '',
        email: d.email ?? '',
        phone: d.phone ?? '',
      }),
    },
  });

  return (
    <SectionForm
      form={form}
      queries={[detailQuery]}
      onSubmit={(v) => updateMyProfile(v)}
      // The nickname and avatar show the change at once, email and phone do not: after Save the form looks the same as
      // before, so say it (criterion in docs/admin/forms.md, "Success messages").
      success={t('profile.saved')}
    >
      <Controller
        name="avatar"
        control={form.control}
        render={({ field }) => (
          <AvatarField value={field.value} onChange={field.onChange} />
        )}
      />
      <Controller
        name="nickname"
        control={form.control}
        render={({ field, fieldState }) => (
          <Field orientation="responsive" data-invalid={fieldState.invalid}>
            <FieldLabel htmlFor="me-nickname">{t('users.nickname')}</FieldLabel>
            <Input
              {...field}
              id="me-nickname"
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
        name="email"
        control={form.control}
        render={({ field, fieldState }) => (
          <Field orientation="responsive" data-invalid={fieldState.invalid}>
            <FieldLabel htmlFor="me-email">{t('field.email')}</FieldLabel>
            <Input
              {...field}
              id="me-email"
              type="email"
              autoComplete="email"
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
          <Field orientation="responsive" data-invalid={fieldState.invalid}>
            <FieldLabel htmlFor="me-phone">{t('field.phone')}</FieldLabel>
            <Input
              {...field}
              id="me-phone"
              type="tel"
              autoComplete="tel"
              aria-invalid={fieldState.invalid}
            />
            {fieldState.invalid ? (
              <FieldError errors={[fieldState.error]} />
            ) : null}
          </Field>
        )}
      />
    </SectionForm>
  );
}

type SecurityDialog = 'password' | 'bind' | 'unbind';

function SecuritySection() {
  const { t } = useTranslation();
  const me = useProfile();
  const [dialog, setDialog] = useState<SecurityDialog>();
  const close = () => setDialog(undefined);

  // One row per item, rows separated by dividers: name and description on the left, status and button on the right.
  return (
    <WithAccount>
      {(a) => {
        const bound = Boolean(a.otpBound);
        return (
          <div className="flex flex-col gap-4">
            <SettingRow title={t('profile.password')}>
              <Button
                variant="outline"
                size="sm"
                onClick={() => setDialog('password')}
              >
                {t('profile.changePassword')}
              </Button>
            </SettingRow>
            <Separator />
            <SettingRow
              title={t('profile.otp')}
              description={
                bound && a.otpBoundAt ? (
                  <>
                    {t('profile.otpBoundAt')}{' '}
                    <DateTimeText value={a.otpBoundAt} />
                  </>
                ) : undefined
              }
            >
              <StatusBadge tone={bound ? 'ok' : 'warn'}>
                {bound ? t('users.otpBound') : t('users.otpUnbound')}
              </StatusBadge>
              <Button
                variant="outline"
                size="sm"
                onClick={() => setDialog(bound ? 'unbind' : 'bind')}
              >
                {bound ? t('profile.unbind') : t('profile.bind')}
              </Button>
            </SettingRow>
            <Separator />
            <SettingRow title={t('field.ipWhitelist')}>
              {a.ipWhitelist ? (
                <IpTags value={a.ipWhitelist} />
              ) : (
                <StatusBadge tone="muted">
                  {t('profile.ipUnlimited')}
                </StatusBadge>
              )}
            </SettingRow>
            <Separator />
            <SettingRow title={t('users.recentLogins')} />
            <LoginAttempts items={a.recentLogins ?? []} />
            <Separator />
            <SettingRow title={t('profile.perms')} />
            {me.isRoot ? (
              <div>
                <Badge>{t('profile.allPerms')}</Badge>
              </div>
            ) : (me.perms ?? []).length === 0 ? (
              <p className="text-muted-foreground text-sm">{t('roles.none')}</p>
            ) : (
              <div className="flex flex-wrap gap-1">
                {(me.perms ?? []).map((p) => (
                  <Badge key={p} variant="secondary">
                    {p}
                  </Badge>
                ))}
              </div>
            )}
            <PasswordForm open={dialog === 'password'} onClose={close} />
            <BindDialog open={dialog === 'bind'} onClose={close} />
            <UnbindDialog open={dialog === 'unbind'} onClose={close} />
          </div>
        );
      }}
    </WithAccount>
  );
}

/** One settings row: name and description on the left, status and button on the right. */
function SettingRow({
  title,
  description,
  children,
}: {
  title: string;
  description?: ReactNode;
  children?: ReactNode;
}) {
  return (
    <div className="flex flex-wrap items-center justify-between gap-4">
      <div className="flex min-w-0 flex-col gap-1">
        <p className="font-medium text-sm">{title}</p>
        {description ? (
          <p className="text-muted-foreground text-sm">{description}</p>
        ) : null}
      </div>
      {children ? (
        <div className="flex items-center gap-2">{children}</div>
      ) : null}
    </div>
  );
}

/** Matches the constraints of the server's ChangeMyPasswordRequest. */
const passwordSchema = z
  .object({
    oldPassword: z.string().min(1).max(128),
    newPassword: z.string().min(8).max(128),
    confirmPassword: z.string(),
  })
  .refine(...samePassword('newPassword', 'confirmPassword'));

/** Change password. Nothing to prefill; one form, one step-up point, the code collected by the dialog's own input. */
function PasswordForm({
  open,
  onClose,
}: {
  open: boolean;
  onClose: () => void;
}) {
  const { t } = useTranslation();
  const form = useZodForm(passwordSchema, {
    open,
    defaultValues: { oldPassword: '', newPassword: '', confirmPassword: '' },
  });

  return (
    <FormDialog
      open={open}
      onClose={onClose}
      title={t('profile.changePassword')}
      description={t('profile.passwordHint')}
      form={form}
      needOtp
      // A successful password change has no other outlet to speak up: the session-invalid rule only sends the user back
      // to login without explanation. Without a word here, the user sees "the dialog closed and I was kicked out".
      success={t('profile.passwordChanged')}
      onSubmit={async (v, otp) => {
        await changeMyPassword(v.oldPassword, v.newPassword, otp);
        // Changing the password bumps token_version: the next request is sent back to login by the session-invalid
        // rule; no redirect here, that rule handles it uniformly.
        await refreshProfile();
      }}
    >
      <Controller
        name="oldPassword"
        control={form.control}
        render={({ field, fieldState }) => (
          <Field orientation="responsive" data-invalid={fieldState.invalid}>
            <FieldLabel htmlFor="old-pwd">
              {t('profile.oldPassword')}
            </FieldLabel>
            <PasswordInput
              {...field}
              id="old-pwd"
              autoComplete="current-password"
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
        name="newPassword"
        control={form.control}
        // Editing the password rechecks the confirmation, so a fixed mismatch clears at once.
        rules={{ deps: 'confirmPassword' }}
        render={({ field, fieldState }) => (
          <Field orientation="responsive" data-invalid={fieldState.invalid}>
            <FieldLabel htmlFor="new-pwd">{t('users.newPassword')}</FieldLabel>
            <PasswordInput
              {...field}
              id="new-pwd"
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
      <Controller
        name="confirmPassword"
        control={form.control}
        render={({ field, fieldState }) => (
          <Field orientation="responsive" data-invalid={fieldState.invalid}>
            <FieldLabel htmlFor="confirm-pwd">
              {t('field.confirmPassword')}
            </FieldLabel>
            <PasswordInput
              {...field}
              id="confirm-pwd"
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
    </FormDialog>
  );
}

/**
 * Bind an authenticator: a flow started from a page button goes into a dialog (container split in docs/admin/forms.md).
 * The three-step flow itself is `OtpBindPanel`, shared with the forced binding on the login page; on success it
 * refreshes the profile and closes the dialog. Clicking outside does not close it, the same rule as form dialogs: a
 * stray click mid-scan would mean starting over.
 */
function BindDialog({ open, onClose }: { open: boolean; onClose: () => void }) {
  const { t } = useTranslation();
  return (
    <Dialog
      open={open}
      onOpenChange={(next) => next || onClose()}
      disablePointerDismissal
    >
      <DialogContent>
        <DialogHeader>
          <DialogTitle>{t('otp.bindTitle')}</DialogTitle>
        </DialogHeader>
        {/* When closed it stays on the last frame through the exit animation; it unmounts with the popup, so the next
            open starts at step one with a new secret. */}
        <KeepLastOpen open={open}>
          <OtpBindPanel
            onConfirmed={async () => {
              await refreshProfile();
              onClose();
            }}
          />
        </KeepLastOpen>
      </DialogContent>
    </Dialog>
  );
}

/** Unbinding verifies the current password rather than step-up: someone who lost the authenticator must be able to
 * recover. */
function UnbindDialog({
  open,
  onClose,
}: {
  open: boolean;
  onClose: () => void;
}) {
  const { t } = useTranslation();
  const [password, setPassword] = useState('');

  return (
    <ActionDialog
      open={open}
      onOpenChange={(next) => {
        if (next) return;
        setPassword('');
        onClose();
      }}
      title={t('profile.unbind')}
      description={t('profile.unbindHint')}
      destructive
      confirmDisabled={password === ''}
      onConfirm={() => unbindOtp(password)}
    >
      <Field>
        <FieldLabel htmlFor="unbind-pwd">
          {t('profile.currentPassword')}
        </FieldLabel>
        <PasswordInput
          id="unbind-pwd"
          autoComplete="current-password"
          value={password}
          onChange={(e) => setPassword(e.target.value)}
        />
      </Field>
    </ActionDialog>
  );
}
