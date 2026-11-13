import { QRCodeSVG } from 'qrcode.react';
import { useEffect, useRef, useState } from 'react';
import { useTranslation } from 'react-i18next';
import { OTP_LENGTH, OtpInput } from '@/components/containers';
import { Button } from '@/components/ui/button';
import { Field, FieldLabel } from '@/components/ui/field';
import { Input } from '@/components/ui/input';
import { api, unwrap } from '@/lib/api';

/**
 * Three steps to bind an authenticator: scan the QR code → copy the secret as a fallback → enter the first code to
 * confirm.
 *
 * It is an ordered flow, not a form to submit and not "confirm one action", so it renders only the flow itself, with no
 * container: the forced binding on the login page expands in place, and the profile page opens it from a button in a
 * plain dialog (container roles in docs/admin/forms.md). The two differ only in what happens after confirmation
 * (onConfirmed).
 */
// The bootstrap flow's restricted token is sent explicitly in Authorization (the server prefers Authorization over the
// cookie); self-service binding while signed in omits it, since the identity is in the session cookie.
const authHeader = (token?: string) =>
  token ? { Authorization: `Bearer ${token}` } : undefined;

export function OtpBindPanel({
  bearer,
  remember,
  onConfirmed,
}: {
  /**
   * The bootstrap flow's restricted token (the bindToken returned by login). It is not a session and cannot go into a
   * cookie, so it can only be sent explicitly in Authorization (the server prefers Authorization over the cookie).
   * Omitted for self-service binding while signed in: the identity is in the session cookie.
   */
  bearer?: string;
  /** The bootstrap flow reuses "remember me" from the login form: a successful confirmation issues the session
   * cookie. */
  remember?: boolean;
  /** What to do once the first code is verified: the login page goes where it should, the profile page refreshes the
   * profile. */
  onConfirmed: () => Promise<void> | void;
}) {
  const { t } = useTranslation();
  const [uri, setUri] = useState('');
  const [code, setCode] = useState('');
  const [busy, setBusy] = useState(false);

  // One binding flow calls BindOTP exactly once. **Each call makes the server generate a new seed**, so calling twice
  // leaves "the page shows the first, the server stores the second": the scanned code is rejected on the spot.
  // StrictMode in dev runs the effect twice and hits exactly this race; in production a bearer change does too. A ref
  // guards it, because across StrictMode's repeated call it is the same component instance.
  const issued = useRef(false);

  useEffect(() => {
    if (issued.current) return;
    issued.current = true;
    void (async () => {
      try {
        const reply = unwrap(
          await api.POST('/v1/sys/auth/otp/bind', {
            body: {},
            headers: authHeader(bearer),
          }),
        );
        // Deliberately no "discard the result on unmount" cancellation: it fights the one-shot guard above: StrictMode
        // discards the first result on unmount, the guard blocks a second call on remount, and the secret is never
        // filled in. setState after unmount is a harmless no-op in React 18+.
        setUri(reply.provisioningUri ?? '');
      } catch {
        // The request middleware has already toasted.
      }
    })();
  }, [bearer]);

  // otpauth://totp/<issuer>:<account>?secret=XXX&... : manual entry needs only the secret.
  const secret = new URL(uri || 'otpauth://x').searchParams.get('secret') ?? '';

  const confirm = async () => {
    setBusy(true);
    try {
      unwrap(
        await api.POST('/v1/sys/auth/otp/confirm', {
          body: { code, remember },
          headers: authHeader(bearer),
        }),
      );
      await onConfirmed();
    } catch {
      // Wrong code: already toasted; stay here to re-enter.
    } finally {
      setBusy(false);
    }
  };

  return (
    <div className="flex flex-col gap-4">
      <p className="text-muted-foreground text-sm">{t('otp.bindHint')}</p>
      <div className="flex justify-center rounded-lg border bg-white p-3">
        {uri ? (
          <QRCodeSVG value={uri} size={160} />
        ) : (
          <div className="size-40" />
        )}
      </div>
      <Field>
        <FieldLabel htmlFor="otp-secret">{t('otp.secret')}</FieldLabel>
        <div className="flex items-center gap-2">
          <Input
            id="otp-secret"
            readOnly
            value={secret}
            className="font-mono text-xs"
          />
          <Button
            type="button"
            variant="outline"
            onClick={() => void navigator.clipboard?.writeText(secret)}
          >
            {t('otp.copy')}
          </Button>
        </div>
      </Field>
      <Field>
        <FieldLabel htmlFor="firstCode">{t('otp.firstCode')}</FieldLabel>
        <OtpInput id="firstCode" value={code} onChange={setCode} />
      </Field>
      <Button
        type="button"
        className="w-full"
        // Not clickable until all six digits are in: the code has a fixed length, one digit short is always wrong.
        disabled={busy || code.length < OTP_LENGTH}
        onClick={() => void confirm()}
      >
        {t('otp.confirm')}
      </Button>
    </div>
  );
}
