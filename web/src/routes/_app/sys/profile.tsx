import { useQuery } from '@tanstack/react-query';
import { createFileRoute } from '@tanstack/react-router';
import { PencilIcon } from 'lucide-react';
import { type ReactNode, useState } from 'react';
import { Controller } from 'react-hook-form';
import { useTranslation } from 'react-i18next';
import { z } from 'zod';
import {
  ActionDialog,
  FormDialog,
  KeepLastOpen,
  PasswordInput,
} from '@/components/containers';
import { DataRegion } from '@/components/data-region';
import {
  AvatarField,
  DateTimeText,
  LoginAttempts,
  StatusBadge,
} from '@/components/fields';
import { OtpBindPanel } from '@/components/otp-bind';
import { Avatar, AvatarFallback, AvatarImage } from '@/components/ui/avatar';
import { Badge } from '@/components/ui/badge';
import { Button } from '@/components/ui/button';
import {
  Card,
  CardContent,
  CardDescription,
  CardTitle,
} from '@/components/ui/card';
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
 * The profile uses the **settings-page layout**: a small vertical nav on the left + content on the right (`< md`
 * collapses to a horizontal row on top). A new setting is one more nav item, not a structural change. Which section is
 * shown is local page state, not in the URL: nothing needs to link to a section directly (rule in docs/admin/pages.md,
 * "Sub-views": only sub-views that must be navigable directly get their own URL).
 *
 * Each section is a card: it shows the current value, and a button opens a dialog to change it; no edit form sits on
 * the page (container split in docs/admin/forms.md). Values on the cards are the profile snapshot, for display only;
 * form initial values are fetched when the dialog opens.
 *
 * Not in the menu: it is reached only from the avatar menu in the header bar, the single entry to the account.
 */
const SECTIONS = ['base', 'security'] as const;

export const Route = createFileRoute('/_app/sys/profile')({
  staticData: { titleKey: 'account.profile' },
  loader: ({ context }) => {
    void context.queryClient.prefetchQuery(myAccountQueryOptions);
  },
  component: ProfilePage,
});

function ProfilePage() {
  const { t } = useTranslation();
  const [section, setSection] = useState<(typeof SECTIONS)[number]>('base');
  const account = useMyAccount();

  return (
    <div className="flex flex-col gap-6 md:flex-row">
      <nav className="flex gap-1 overflow-x-auto md:w-40 md:flex-col">
        {SECTIONS.map((s) => (
          <Button
            key={s}
            // The current item uses secondary, the rest ghost: the selected state is left to button variants, no
            // hand-written colours.
            variant={section === s ? 'secondary' : 'ghost'}
            className="justify-start"
            aria-current={section === s ? 'page' : undefined}
            onClick={() => setSection(s)}
          >
            {t(`profile.${s}`)}
          </Button>
        ))}
      </nav>
      <div className="flex min-w-0 flex-1 flex-col gap-4">
        {/* The nav is frame and touches no data; account details live only inside the data region, so a failed fetch
            fails only that block. */}
        <DataRegion query={account}>
          {(a) =>
            section === 'base' ? (
              <BaseSection a={a} />
            ) : (
              <SecuritySection a={a} />
            )
          }
        </DataRegion>
      </div>
    </div>
  );
}

type MyAccount = NonNullable<ReturnType<typeof useMyAccount>['data']>;
const useMyAccount = () => useQuery(myAccountQueryOptions);

function BaseSection({ a }: { a: MyAccount }) {
  const { t } = useTranslation();
  const [editing, setEditing] = useState(false);

  // Identity header (modelled on admin profile pages): large avatar + nickname, username on a grey line below; under
  // the divider the remaining info, small label above, value below. The left nav already says "Basic info", so the card
  // repeats no title.
  return (
    <Card>
      <CardContent className="flex flex-col gap-(--card-spacing)">
        <div className="flex flex-wrap items-center gap-(--card-spacing)">
          <Avatar className="size-16">
            {a.avatar ? <AvatarImage src={a.avatar} alt="" /> : null}
            <AvatarFallback />
          </Avatar>
          <div className="flex min-w-0 flex-1 flex-col gap-1">
            <p className="truncate font-heading font-semibold text-xl">
              {a.nickname || a.username}
            </p>
            <p className="truncate text-muted-foreground text-sm">
              {a.username}
            </p>
          </div>
          <Button variant="outline" size="sm" onClick={() => setEditing(true)}>
            <PencilIcon data-icon="inline-start" />
            {t('profile.editBase')}
          </Button>
        </div>
        <Separator />
        <dl className="grid gap-(--card-spacing) sm:grid-cols-2 lg:grid-cols-3">
          <InfoItem label={t('field.email')} value={a.email} />
          <InfoItem label={t('field.phone')} value={a.phone} />
          <InfoItem label={t('users.dept')} value={a.deptName} />
          <InfoItem
            label={t('users.roles')}
            value={
              (a.roles ?? []).length > 0 ? (
                <div className="flex flex-wrap gap-1">
                  {(a.roles ?? []).map((r) => (
                    <Badge key={r.id} variant="secondary">
                      {r.name}
                    </Badge>
                  ))}
                </div>
              ) : null
            }
          />
          <InfoItem
            label={t('table.createdAt')}
            value={<DateTimeText value={a.createdAt} />}
          />
        </dl>
      </CardContent>
      <BaseForm open={editing} onClose={() => setEditing(false)} />
    </Card>
  );
}

/** One read-only item: small label above, value below; shows "—" when there is no value. */
function InfoItem({ label, value }: { label: string; value: ReactNode }) {
  return (
    <div className="flex min-w-0 flex-col gap-1">
      <dt className="text-muted-foreground text-xs">{label}</dt>
      <dd className="break-words">{value || '-'}</dd>
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

/** Change nickname, avatar, email, phone. The username is the account's identity; even its owner cannot change it, so
 * it is not in the form. */
function BaseForm({ open, onClose }: { open: boolean; onClose: () => void }) {
  const { t } = useTranslation();
  const { form, detail: detailQuery } = useDetailForm(baseSchema, {
    open,
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
    <FormDialog
      open={open}
      onClose={onClose}
      title={t('profile.editBase')}
      form={form}
      queries={[detailQuery]}
      onSubmit={(v) => updateMyProfile(v)}
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
          <Field data-invalid={fieldState.invalid}>
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
          <Field data-invalid={fieldState.invalid}>
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
          <Field data-invalid={fieldState.invalid}>
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
    </FormDialog>
  );
}

type SecurityDialog = 'password' | 'bind' | 'unbind';

function SecuritySection({ a }: { a: MyAccount }) {
  const { t } = useTranslation();
  const me = useProfile();
  const bound = Boolean(a.otpBound);
  const [dialog, setDialog] = useState<SecurityDialog>();
  const close = () => setDialog(undefined);

  // One card, one row per item, rows separated by dividers: name and description on the left, status and button on the
  // right.
  return (
    <Card>
      <CardContent className="flex flex-col gap-(--card-spacing)">
        <SettingRow
          title={t('profile.password')}
          description={t('profile.passwordHint')}
        >
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
                {t('profile.otpHint')} {t('profile.otpBoundAt')}{' '}
                <DateTimeText value={a.otpBoundAt} />
              </>
            ) : (
              t('profile.otpHint')
            )
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
        <SettingRow
          title={t('field.ipWhitelist')}
          description={t('profile.ipWhitelistHint')}
        >
          {a.ipWhitelist ? (
            <span className="break-all">{a.ipWhitelist}</span>
          ) : (
            <StatusBadge tone="muted">{t('profile.ipUnlimited')}</StatusBadge>
          )}
        </SettingRow>
        <Separator />
        <SettingRow
          title={t('users.recentLogins')}
          description={t('profile.recentLoginsHint')}
        />
        <LoginAttempts items={a.recentLogins ?? []} />
        <Separator />
        <SettingRow
          title={t('profile.perms')}
          description={t('profile.permsHint')}
        />
        {me.isRoot ? (
          <div>
            <Badge>{t('profile.allPerms')}</Badge>
          </div>
        ) : (me.perms ?? []).length === 0 ? (
          <p className="text-muted-foreground">{t('roles.none')}</p>
        ) : (
          <div className="flex flex-wrap gap-1">
            {(me.perms ?? []).map((p) => (
              <Badge key={p} variant="secondary">
                {p}
              </Badge>
            ))}
          </div>
        )}
      </CardContent>
      <PasswordForm open={dialog === 'password'} onClose={close} />
      <BindDialog open={dialog === 'bind'} onClose={close} />
      <UnbindDialog open={dialog === 'unbind'} onClose={close} />
    </Card>
  );
}

/** One settings row: name and description on the left (font sizes follow the card title and description), status and
 * button on the right. */
function SettingRow({
  title,
  description,
  children,
}: {
  title: string;
  description: ReactNode;
  children?: ReactNode;
}) {
  return (
    <div className="flex flex-wrap items-center justify-between gap-(--card-spacing)">
      <div className="flex min-w-0 flex-col gap-1">
        <CardTitle>{title}</CardTitle>
        <CardDescription>{description}</CardDescription>
      </div>
      {children ? (
        <div className="flex items-center gap-2">{children}</div>
      ) : null}
    </div>
  );
}

/** Matches the constraints of the server's ChangeMyPasswordRequest. */
const passwordSchema = z.object({
  oldPassword: z.string().min(1).max(128),
  newPassword: z.string().min(8).max(128),
});

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
    defaultValues: { oldPassword: '', newPassword: '' },
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
          <Field data-invalid={fieldState.invalid}>
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
        render={({ field, fieldState }) => (
          <Field data-invalid={fieldState.invalid}>
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
