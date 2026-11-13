import { Turnstile } from '@marsidev/react-turnstile';
import { RefreshCwIcon } from 'lucide-react';
import { useCallback, useEffect, useState } from 'react';
import { useTranslation } from 'react-i18next';
import { Button } from '@/components/ui/button';
import { Field, FieldError, FieldLabel } from '@/components/ui/field';
import { Input } from '@/components/ui/input';
import {
  Tooltip,
  TooltipContent,
  TooltipTrigger,
} from '@/components/ui/tooltip';
import { useColorMode } from '@/hooks/use-theme';
import { api, unwrap } from '@/lib/api';
import { getLocale } from '@/lib/locale';
import type { LoginMeta } from '@/lib/query';

/** The two CAPTCHA fields, matching the two parameters of LoginRequest one to one. */
export type CaptchaValue = { id: string; answer: string };

export const EMPTY_CAPTCHA: CaptchaValue = { id: '', answer: '' };

/**
 * The CAPTCHA row of the login page, branching on deployment config: disabled renders nothing, `base64` shows an image
 * code, `turnstile` mounts the Cloudflare widget. A new provider adds a branch here.
 *
 * attempt is "which attempt this is": the parent increments it on every failed login, the image code refreshes on it (a
 * challenge is void once verified, so without a new one the second try always fails), and the widget remounts on it
 * (the token is single-use; without a reset it would keep failing on an expired token).
 */
export function CaptchaField({
  meta,
  value,
  onChange,
  attempt,
  error,
}: {
  meta: LoginMeta;
  value: CaptchaValue;
  onChange: (value: CaptchaValue) => void;
  attempt: number;
  /** Field error while the CAPTCHA has not been passed (decided by the login page; this only displays it). */
  error?: string;
}) {
  if (!meta.captchaEnabled) return null;
  // Both branches use attempt as the key: remounting resets, so the image code fetches a new image and the widget drops
  // its internal expiry timer, sparing two separate "how to reset" mechanisms.
  if (meta.captchaProvider === 'turnstile') {
    return (
      <TurnstileField
        key={attempt}
        siteKey={meta.captchaSiteKey ?? ''}
        onChange={onChange}
        error={error}
      />
    );
  }
  return (
    <ImageCaptchaField
      key={attempt}
      value={value}
      onChange={onChange}
      error={error}
    />
  );
}

function TurnstileField({
  siteKey,
  onChange,
  error,
}: {
  siteKey: string;
  onChange: (value: CaptchaValue) => void;
  error?: string;
}) {
  const mode = useColorMode();
  return (
    <Field data-invalid={Boolean(error)}>
      <Turnstile
        siteKey={siteKey}
        options={{
          theme: mode === 'system' ? 'auto' : mode,
          language: getLocale().toLowerCase(),
        }}
        // The token the widget produces is the answer; it needs no server-issued challenge id. Not passed, expired or
        // errored all clear it, which the login page reads as "CAPTCHA not done yet".
        onSuccess={(token) => onChange({ id: '', answer: token })}
        onExpire={() => onChange(EMPTY_CAPTCHA)}
        onError={() => onChange(EMPTY_CAPTCHA)}
      />
      {error ? <FieldError>{error}</FieldError> : null}
    </Field>
  );
}

function ImageCaptchaField({
  value,
  onChange,
  error,
}: {
  value: CaptchaValue;
  onChange: (value: CaptchaValue) => void;
  error?: string;
}) {
  const { t } = useTranslation();
  const [image, setImage] = useState('');

  const issue = useCallback(async () => {
    try {
      const c = unwrap(await api.POST('/v1/sys/auth/captcha', { body: {} }));
      setImage(c.imageBase64 ?? '');
      // A new image must clear the answer at the same time: keeping the previous input means submitting unchanged,
      // which always fails.
      onChange({ id: c.captchaId ?? '', answer: '' });
    } catch {
      // The request middleware has already toasted; staying in place is enough.
    }
  }, [onChange]);

  // Fetch one on mount; on a failed login the parent changes the key to remount, which fetches another.
  useEffect(() => {
    void issue();
  }, [issue]);

  return (
    <Field data-invalid={Boolean(error)}>
      <FieldLabel htmlFor="captcha">{t('login.captcha')}</FieldLabel>
      <div className="flex items-center gap-2">
        <Input
          id="captcha"
          className="flex-1"
          inputMode="numeric"
          autoComplete="off"
          aria-invalid={Boolean(error)}
          value={value.answer}
          onChange={(e) => onChange({ ...value, answer: e.target.value })}
        />
        <Tooltip>
          <TooltipTrigger
            render={
              <Button
                type="button"
                variant="outline"
                // Height uses the default button size, matching the input on the left; width follows the 3:1 aspect
                // ratio of the image code, and the image fills the whole button. Click to get a new one.
                className="w-24 shrink-0 overflow-hidden p-0"
                onClick={() => void issue()}
                aria-label={t('login.captchaRefresh')}
              />
            }
          >
            {image ? (
              <img src={image} alt="" className="h-full w-full object-cover" />
            ) : (
              <RefreshCwIcon />
            )}
          </TooltipTrigger>
          <TooltipContent>{t('login.captchaRefresh')}</TooltipContent>
        </Tooltip>
      </div>
      {error ? <FieldError>{error}</FieldError> : null}
    </Field>
  );
}
