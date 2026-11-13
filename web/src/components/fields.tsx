import { XIcon } from 'lucide-react';
import { type ReactNode, useId, useRef } from 'react';
import { useTranslation } from 'react-i18next';
import { toast } from 'sonner';
import type { DateZone } from '@/components/date-picker';
import { Avatar, AvatarFallback, AvatarImage } from '@/components/ui/avatar';
import { Badge } from '@/components/ui/badge';
import { Button } from '@/components/ui/button';
import { Field, FieldLabel } from '@/components/ui/field';
import { useZone } from '@/hooks/use-timezone';
import {
  EMPTY_TIME,
  formatDateTime,
  formatTimeZoneShortName,
} from '@/lib/timezone';

/** Section heading inside drawers and form dialogs. A long form needs sections with a readable boundary between
 * them. */
export function Section({
  title,
  hint,
  children,
}: {
  title: string;
  hint?: string;
  children: ReactNode;
}) {
  return (
    <section className="flex flex-col gap-3 py-2">
      <div className="flex flex-col gap-0.5">
        <h3 className="font-medium text-sm">{title}</h3>
        {hint ? <p className="text-muted-foreground text-xs">{hint}</p> : null}
      </div>
      {children}
    </section>
  );
}

/**
 * Login attempt list: success / failure badge, time, IP. Shared by the user detail drawer and "Recent logins" on the
 * profile page.
 */
export function LoginAttempts({
  items,
}: {
  items: readonly {
    id?: string;
    success?: boolean;
    createdAt?: string;
    ip?: string;
  }[];
}) {
  const { t } = useTranslation();
  if (items.length === 0) {
    return <p className="text-muted-foreground text-sm">{t('table.empty')}</p>;
  }
  return (
    <ul className="flex flex-col gap-2 text-sm">
      {items.map((l) => (
        <li key={l.id} className="flex flex-wrap items-center gap-2">
          {l.success ? (
            <StatusBadge tone="ok">{t('audit.success')}</StatusBadge>
          ) : (
            <StatusBadge tone="danger">{t('audit.failure')}</StatusBadge>
          )}
          <DateTimeText value={l.createdAt} />
          <span className="text-muted-foreground">{l.ip}</span>
        </li>
      ))}
    </ul>
  );
}

/** One row of read-only information: label on the left, value on the right, listed row by row in a `<dl>`. A missing
 * value shows "—". */
export function DetailRow({
  label,
  value,
}: {
  label: string;
  value: ReactNode;
}) {
  return (
    <div className="grid grid-cols-[8rem_1fr] gap-2">
      <dt className="text-muted-foreground">{label}</dt>
      <dd>{value || '-'}</dd>
    </div>
  );
}

/**
 * Status badge: enum and yes/no values are shown as a small colored badge rather than a line of black text.
 *
 * Four levels are enough; more and people have to guess what each color means:
 * - `ok`     holds, normal, done: green
 * - `warn`   to do, a step missing, needs attention but is not an error: yellow
 * - `danger` disabled, failed, something is wrong: red
 * - `muted`  a neutral fact, neither good nor bad (not bound, but not required either): gray
 *
 * Colors come from three semantic tokens, `--success` / `--warning` / `--destructive`. They **deliberately do not
 * follow the brand preset**: green = holds, yellow = to do, red = wrong reads the same under any preset, whereas
 * coloring "normal" with the brand color would make it an orange "normal" under a preset with an orange primary.
 */
const TONE = {
  ok: 'border-success/30 bg-success/10 text-success',
  warn: 'border-warning/30 bg-warning/10 text-warning',
  danger: 'border-destructive/30 bg-destructive/10 text-destructive',
  muted: 'border-border bg-muted text-muted-foreground',
} as const;

export function StatusBadge({
  tone,
  children,
}: {
  tone: keyof typeof TONE;
  children: ReactNode;
}) {
  return (
    <Badge variant="outline" className={TONE[tone]}>
      {children}
    </Badge>
  );
}

/**
 * By default the time follows the display time zone chosen in the header; an instant that only makes sense in the
 * business time zone (such as a send time scheduled in the business time zone) gets `zone="business"`, always shown in
 * the business time zone regardless of the header. The backend sends only UTC RFC3339 and conversion happens only here,
 * so every place that shows a time uses this component instead of calling formatDateTime itself.
 *
 * **With a time zone label** (`2026-07-25 22:01:59 (UTC)`): the same digits are different instants in different time
 * zones, and the header dropdown can change at any time. Repeating it on every row of a column is redundant, but
 * without it a screenshot sent to a colleague, or your own look the next day, cannot tell whose midnight it is.
 */
export function DateTimeText({
  value,
  zone,
}: {
  value?: string | null;
  zone?: DateZone;
}) {
  const timeZone = useZone(zone);
  const text = formatDateTime(value, timeZone);
  // An empty value gets no time zone: "-(UTC)" means nothing.
  if (text === EMPTY_TIME) {
    return <span className="text-muted-foreground">{text}</span>;
  }
  return (
    <span className="whitespace-nowrap tabular-nums">
      {text}
      <span className="ml-1 text-muted-foreground text-xs">
        ({formatTimeZoneShortName(timeZone)})
      </span>
    </span>
  );
}

/**
 * Avatar field: a locally picked image becomes a data URI. The size limit is **decided by the server**
 * (biz.MaxAvatarBytes); the limit here only rejects early to save a round trip that is bound to fail; when the two
 * differ, the server wins.
 */
export function AvatarField({
  value,
  onChange,
  maxBytes = 512 * 1024,
}: {
  value: string;
  onChange: (dataUri: string) => void;
  maxBytes?: number;
}) {
  const { t } = useTranslation();
  const inputRef = useRef<HTMLInputElement>(null);
  const id = useId();

  return (
    <Field>
      <FieldLabel htmlFor={id}>{t('field.avatar')}</FieldLabel>
      <div className="flex items-center gap-3">
        <Avatar className="size-16">
          {value ? <AvatarImage src={value} alt="" /> : null}
          <AvatarFallback />
        </Avatar>
        {/* The native file input is hidden and triggered by the "choose image" button next to it: shadcn has no file
            picker component, so this is an exception to "prefer shadcn controls". */}
        <input
          id={id}
          ref={inputRef}
          type="file"
          accept="image/*"
          className="hidden"
          onChange={(e) => {
            const file = e.target.files?.[0];
            // Picking the same file twice must still fire change: clear the input value.
            e.target.value = '';
            if (!file) return;
            if (file.size > maxBytes) {
              onChange(value);
              // The over-limit message goes through toast (the single global voice), not a permanent error line in the
              // field.
              toast.error(t('field.avatarTooLarge'));
              return;
            }
            const reader = new FileReader();
            reader.onload = () => onChange(String(reader.result ?? ''));
            reader.readAsDataURL(file);
          }}
        />
        <Button
          type="button"
          variant="outline"
          size="sm"
          onClick={() => inputRef.current?.click()}
        >
          {t('field.pickImage')}
        </Button>
        {value ? (
          <Button
            type="button"
            variant="ghost"
            size="icon-sm"
            aria-label={t('field.clearAvatar')}
            onClick={() => onChange('')}
          >
            <XIcon />
          </Button>
        ) : null}
      </div>
    </Field>
  );
}
