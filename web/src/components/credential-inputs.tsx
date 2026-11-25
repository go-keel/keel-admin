import { Link } from '@tanstack/react-router';
import { EyeIcon, EyeOffIcon } from 'lucide-react';
import { useId, useState } from 'react';
import { useTranslation } from 'react-i18next';
import {
  Field,
  FieldDescription,
  FieldError,
  FieldLabel,
} from '@/components/ui/field';
import { Input } from '@/components/ui/input';
import {
  InputGroup,
  InputGroupAddon,
  InputGroupButton,
  InputGroupInput,
} from '@/components/ui/input-group';
import { useProfile } from '@/hooks/use-profile';

/**
 * Password input: the login page, change password, reset password and unbinding an authenticator all use it, and a
 * derived repo uses it for every password too (docs/admin/pages.md, "Controls and sizes"). It is the shadcn InputGroup
 * pattern: an eye button at the end of the input toggles plain text, and button and input share one focus ring. Props
 * other than `type` go to the input as-is; the caller sets `autoComplete` per case (`current-password` /
 * `new-password`), which password managers rely on.
 */
export function PasswordInput(
  props: Omit<React.ComponentProps<typeof InputGroupInput>, 'type'>,
) {
  const { t } = useTranslation();
  const [visible, setVisible] = useState(false);
  return (
    <InputGroup>
      <InputGroupInput {...props} type={visible ? 'text' : 'password'} />
      <InputGroupAddon align="inline-end">
        {/* Not in the Tab order: Tab after the password goes straight to the next field; the toggle is mouse-only. */}
        <InputGroupButton
          size="icon-xs"
          tabIndex={-1}
          aria-label={t(visible ? 'form.hidePassword' : 'form.showPassword')}
          onClick={() => setVisible((v) => !v)}
        >
          {visible ? <EyeOffIcon /> : <EyeIcon />}
        </InputGroupButton>
      </InputGroupAddon>
    </InputGroup>
  );
}

/** Number of digits in a one-time code: the server's TOTP is fixed at 6. */
export const OTP_LENGTH = 6;

/**
 * One-time code input. The login page, binding an authenticator and OtpField below all use it, and a derived repo uses
 * it for every code too (docs/admin/pages.md, "Controls and sizes"). It is a shadcn Input plus three things:
 * `autoComplete="one-time-code"` lets browsers and password managers such as 1Password recognise the code field and
 * fill it; `inputMode="numeric"` brings up the numeric keyboard on phones; input keeps digits only, at most 6, so a
 * pasted "123 456" with a space still works, which is why there is no maxLength (it would cut the 7th character
 * before filtering).
 */
export function OtpInput({
  value,
  onChange,
  ...props
}: Omit<React.ComponentProps<typeof Input>, 'value' | 'onChange' | 'type'> & {
  value: string;
  onChange: (value: string) => void;
}) {
  return (
    <Input
      type="text"
      inputMode="numeric"
      autoComplete="one-time-code"
      pattern="[0-9]*"
      {...props}
      value={value}
      onChange={(e) =>
        onChange(e.target.value.replace(/\D/g, '').slice(0, OTP_LENGTH))
      }
    />
  );
}

/**
 * One-time code input. **At most one per form**: the headroom of `sys.otp.max_uses` is bought with replay risk, not a
 * budget for form design (ADR admin-0007).
 *
 * Present means required (aria-required, the label gets a red asterisk automatically). It is not a form-library value:
 * the code does not count as "edited", so closing after typing only a code does not ask whether to discard.
 */
export function OtpField({
  value,
  onChange,
  id,
  invalid,
}: {
  value: string;
  onChange: (value: string) => void;
  /**
   * Give it only when the input must be targeted from outside; by default an id is generated, avoiding clashes on one
   * page.
   */
  id?: string;
  /** Not filled at submission: mark red and say it is required. */
  invalid?: boolean;
}) {
  const { t } = useTranslation();
  const { otpBound } = useProfile();
  // A page may hold a form dialog and an action dialog at once, each with a code input. A hard-coded id would duplicate
  // the id on the page: invalid HTML, and <label for> would point at whichever comes first.
  const autoId = useId();
  const fieldId = id ?? autoId;
  if (!otpBound) {
    // Guidance in place instead of a field the server is bound to refuse (OTP_BIND_REQUIRED).
    return (
      <Field>
        <FieldLabel>{t('action.otp')}</FieldLabel>
        <FieldDescription>
          {t('action.otpBindFirst')}{' '}
          <Link to="/sys/profile" className="underline">
            {t('action.otpBindLink')}
          </Link>
        </FieldDescription>
      </Field>
    );
  }
  return (
    <Field data-invalid={invalid || undefined}>
      <FieldLabel htmlFor={fieldId}>{t('action.otp')}</FieldLabel>
      <OtpInput
        id={fieldId}
        aria-required
        aria-invalid={invalid || undefined}
        value={value}
        onChange={onChange}
      />
      {invalid ? (
        <FieldError>{t('form.required')}</FieldError>
      ) : (
        <FieldDescription>{t('action.otpHint')}</FieldDescription>
      )}
    </Field>
  );
}
