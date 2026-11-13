import { zodResolver } from '@hookform/resolvers/zod';
import { useQuery, useQueryClient } from '@tanstack/react-query';
import { useNavigate, useSearch } from '@tanstack/react-router';
import { type ReactNode, useState } from 'react';
import { Controller, useForm } from 'react-hook-form';
import { useTranslation } from 'react-i18next';
import { z } from 'zod';
import {
  CaptchaField,
  type CaptchaValue,
  EMPTY_CAPTCHA,
} from '@/components/captcha-field';
import { OtpInput, PasswordInput } from '@/components/containers';
import { LoadFailed } from '@/components/data-region';
import { OtpBindPanel } from '@/components/otp-bind';
import { Button } from '@/components/ui/button';
import { Checkbox } from '@/components/ui/checkbox';
import {
  Field,
  FieldDescription,
  FieldError,
  FieldGroup,
  FieldLabel,
} from '@/components/ui/field';
import { Input } from '@/components/ui/input';
import { api, unwrap } from '@/lib/api';
import { loadState, retryFailed } from '@/lib/load-state';
import { loginMetaQueryOptions } from '@/lib/query';

const formSchema = z.object({
  username: z.string().min(1),
  password: z.string().min(1),
  // Always shown as optional: whether an account has an authenticator bound must not reach the frontend before login,
  // or it becomes an account enumeration channel.
  otpCode: z.string(),
  remember: z.boolean(),
});

type FormValues = z.infer<typeof formSchema>;

/**
 * Login form (kernel): username, password, CAPTCHA, one-time code (OTP), remember me, and the in-place binding guide
 * when a role forces OTP but none is bound yet. All security logic of the login flow lives here and upgrades with the
 * kernel.
 *
 * Everything outside the form (background, columns, hero image, brand, language switch, the heading above the form) is
 * decided by the assembly seam `web/src/login-layout.tsx`, which places this component wherever it wants. `heading` is
 * the heading above the form in the login step; the heading of the authenticator-binding step describes the flow and
 * comes from the kernel.
 */
export function LoginForm({ heading }: { heading?: ReactNode }) {
  const { t } = useTranslation();
  const { redirect } = useSearch({ from: '/login' });
  const navigate = useNavigate();
  const queryClient = useQueryClient();
  const metaQuery = useQuery(loginMetaQueryOptions);
  const meta = metaQuery.data;
  // Without the startup info we do not know whether a CAPTCHA is needed: that cell shows "failed to load" and retry,
  // and the login button is disabled, so no request bound to be rejected is sent.
  const metaFailed = loadState([metaQuery]) === 'failed';

  const [captcha, setCaptcha] = useState<CaptchaValue>(EMPTY_CAPTCHA);
  // +1 on every failed login: the image code refreshes and the Turnstile widget remounts on it (see CaptchaField).
  const [attempt, setAttempt] = useState(0);
  // "CAPTCHA not done" is reported only after the first login click, then validated on every change (form rules:
  // validate everything on first submit, validate on change after that).
  const [submitted, setSubmitted] = useState(false);
  const captchaMissing = Boolean(meta?.captchaEnabled) && !captcha.answer;
  // Non-empty = a role forces OTP on this account but none is bound, so switch to the binding guide in place (not a
  // second login).
  const [bindToken, setBindToken] = useState('');

  const form = useForm<FormValues>({
    resolver: zodResolver(formSchema),
    defaultValues: { username: '', password: '', otpCode: '', remember: false },
  });

  const enter = async () => {
    // A sign-in starts a new identity: drop everything cached for the previous session (its profile, lists, options),
    // or a session that expired without an explicit logout would gate pages and show data with the old identity.
    queryClient.clear();
    await navigate({ to: redirect ?? '/' });
  };

  const onSubmit = form.handleSubmit(
    async (values) => {
      setSubmitted(true);
      // No request without the CAPTCHA: the image code is empty, or the Turnstile widget has not produced a token (not
      // passed, expired or errored all clear it). This only saves a round trip bound to fail; the gate stays on the
      // server.
      if (captchaMissing) {
        document.getElementById('captcha')?.focus();
        return;
      }
      let reply: Awaited<ReturnType<typeof login>>;
      try {
        reply = await login(values, captcha);
      } catch {
        // A failed login (wrong password, wrong code, locked) has already been toasted by the request middleware; the
        // page just stays in place. Uncaught, handleSubmit would turn it into an unhandled rejection: a red line in the
        // console and noise in global error reporting, for an expected branch. The used token / challenge is single-use
        // and void, so clear it too: a remounted Turnstile does not clear itself, and keeping the old token would both
        // slip past the check above and be rejected by the server.
        setCaptcha(EMPTY_CAPTCHA);
        setAttempt((n) => n + 1);
        return;
      }
      if (reply.bindToken) {
        setBindToken(reply.bindToken);
        return;
      }
      await enter();
    },
    () => setSubmitted(true),
  );

  if (bindToken) {
    return (
      <div className="flex flex-col gap-6">
        <LoginHeading title={t('otp.bindTitle')} desc={t('otp.bindRequired')} />
        <OtpBindPanel
          bearer={bindToken}
          remember={form.getValues('remember')}
          onConfirmed={enter}
        />
      </div>
    );
  }

  return (
    <form onSubmit={onSubmit} className="flex flex-col gap-6">
      <FieldGroup>
        {heading}

        <Field data-invalid={form.formState.errors.username !== undefined}>
          <FieldLabel htmlFor="username">{t('login.username')}</FieldLabel>
          <Input
            id="username"
            autoComplete="username"
            {...form.register('username')}
          />
          {form.formState.errors.username ? (
            <FieldError>{t('login.usernameRequired')}</FieldError>
          ) : null}
        </Field>

        <Field data-invalid={form.formState.errors.password !== undefined}>
          <FieldLabel htmlFor="password">{t('login.password')}</FieldLabel>
          <PasswordInput
            id="password"
            autoComplete="current-password"
            aria-invalid={form.formState.errors.password !== undefined}
            {...form.register('password')}
          />
          {form.formState.errors.password ? (
            <FieldError>{t('login.passwordRequired')}</FieldError>
          ) : null}
        </Field>

        {metaFailed ? (
          <LoadFailed onRetry={() => retryFailed([metaQuery])} />
        ) : meta ? (
          <CaptchaField
            meta={meta}
            value={captcha}
            onChange={setCaptcha}
            attempt={attempt}
            error={
              submitted && captchaMissing
                ? t('login.captchaRequired')
                : undefined
            }
          />
        ) : null}

        <Controller
          name="otpCode"
          control={form.control}
          render={({ field }) => (
            <Field>
              <FieldLabel htmlFor="otpCode">{t('login.otp')}</FieldLabel>
              <OtpInput
                id="otpCode"
                value={field.value}
                onChange={field.onChange}
              />
              <FieldDescription>{t('login.otpHint')}</FieldDescription>
            </Field>
          )}
        />

        <Controller
          name="remember"
          control={form.control}
          render={({ field }) => (
            <Field orientation="horizontal">
              <Checkbox
                id="remember"
                checked={field.value}
                onCheckedChange={(c) => field.onChange(Boolean(c))}
              />
              <FieldLabel htmlFor="remember" className="font-normal">
                {t('login.remember')}
              </FieldLabel>
            </Field>
          )}
        />

        <Field>
          <Button
            type="submit"
            disabled={metaFailed || form.formState.isSubmitting}
          >
            {form.formState.isSubmitting
              ? t('login.submitting')
              : t('login.submit')}
          </Button>
        </Field>
      </FieldGroup>
    </form>
  );
}

/** Heading plus description above the form. The login layout in the assembly seam can use it directly, styled the same
 * as the binding step. */
export function LoginHeading({
  title,
  desc,
}: {
  title: string;
  desc?: string;
}) {
  return (
    <div className="flex flex-col items-center gap-1 text-center">
      <h1 className="font-medium text-3xl">{title}</h1>
      {desc ? (
        <p className="text-balance text-muted-foreground text-sm">{desc}</p>
      ) : null}
    </div>
  );
}

const login = async (values: FormValues, captcha: CaptchaValue) =>
  unwrap(
    await api.POST('/v1/sys/auth/login', {
      body: {
        username: values.username,
        password: values.password,
        otpCode: values.otpCode || undefined,
        captchaId: captcha.id || undefined,
        captchaAnswer: captcha.answer || undefined,
        remember: values.remember,
      },
    }),
  );
