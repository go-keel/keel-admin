import { Link } from '@tanstack/react-router';
import { EyeIcon, EyeOffIcon } from 'lucide-react';
import {
  type ReactElement,
  type ReactNode,
  useEffect,
  useId,
  useState,
} from 'react';
import type { FieldValues, UseFormReturn } from 'react-hook-form';
import { useTranslation } from 'react-i18next';
import { toast } from 'sonner';
import { LoadFailed } from '@/components/data-region';
import {
  AlertDialog,
  AlertDialogAction,
  AlertDialogCancel,
  AlertDialogContent,
  AlertDialogDescription,
  AlertDialogFooter,
  AlertDialogHeader,
  AlertDialogTitle,
} from '@/components/ui/alert-dialog';
import { Button } from '@/components/ui/button';
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogFooter,
  DialogHeader,
  DialogTitle,
} from '@/components/ui/dialog';
import {
  Field,
  FieldDescription,
  FieldError,
  FieldGroup,
  FieldLabel,
} from '@/components/ui/field';
import { Input } from '@/components/ui/input';
import {
  InputGroup,
  InputGroupAddon,
  InputGroupButton,
  InputGroupInput,
} from '@/components/ui/input-group';
import {
  Popover,
  PopoverContent,
  PopoverDescription,
  PopoverHeader,
  PopoverTitle,
  PopoverTrigger,
} from '@/components/ui/popover';
import {
  Sheet,
  SheetContent,
  SheetDescription,
  SheetHeader,
  SheetTitle,
} from '@/components/ui/sheet';
import { Skeleton } from '@/components/ui/skeleton';
import { useProfile } from '@/hooks/use-profile';
import { loadState, type QueryLike, retryFailed } from '@/lib/load-state';
import { cn } from '@/lib/utils';

/**
 * Three containers (full rules: docs/admin/forms.md, "Container roles"):
 *
 * - Dialog: a form to submit (create, edit) uses FormDialog; confirming one action uses ActionDialog by default.
 * - Popover (ConfirmPopover): a button shown directly on the page may also confirm its action in a popover anchored to
 *   the button; the page decides. An action inside a "⋯" menu can only use a dialog.
 * - Drawer (ResourceSheet): viewing one row's information; the page decides whether it holds buttons.
 *
 * The containers own only the rules of the shell (whether a click outside closes it, whether edits need confirming
 * before closing, how busy and failed states look); the page writes the fields inside the shadcn way.
 */

/**
 * The sentence to say after success; omitted means no toast. **Give it only for actions with no visible change on
 * screen**: a new row in the list or a changed status column already says it, and a toast on top is noise (the test is
 * in docs/admin/forms.md, "Success messages").
 *
 * When one action has more than one outcome (really executed / only submitted for approval), give a function: its
 * argument is the confirm function's return value, and returning undefined sends nothing. The argument type is
 * inferred from the confirm function; it cannot be inferred when the confirm function is an inline arrow without
 * parameter types placed after `success`, so annotate the parameter or move the confirm function before `success`.
 */
type SuccessText<R> = string | ((result: R) => string | undefined);

function toastSuccess<R>(success: SuccessText<R> | undefined, result: R) {
  const text = typeof success === 'function' ? success(result) : success;
  if (text) toast.success(text);
}

/**
 * The flow for confirming one action, shared by the three containers (FormDialog, ActionDialog, ConfirmPopover), not
 * exported: collects the one-time code, clears the previous round's code on every open (a code is single-use, so
 * submitting the old one always fails), is busy while running; a throw = failure, stay put (the request layer has
 * already toasted the error); returning `false` = the page blocked it itself, also stay put; anything else is success:
 * say the sentence (`success`), then the container closes (`onDone`).
 *
 * How a missing code looks belongs to the container: the form dialog follows the form rules and flags it only on
 * submit (focus and mark red); the two confirm containers disable the confirm button (like `confirmDisabled`). This
 * hook only reports `otpMissing`.
 */
function useConfirmFlow<R>({
  open,
  needOtp,
  success,
  onDone,
}: {
  open: boolean;
  needOtp?: boolean;
  success?: SuccessText<R>;
  onDone: () => void;
}) {
  const [otp, setOtp] = useState('');
  const [busy, setBusy] = useState(false);

  // Clear on open, not on close: after success the container closes itself, not necessarily through the close callback.
  useEffect(() => {
    if (open) setOtp('');
  }, [open]);

  const run = async (
    exec: (otpCode: string) => R | false | Promise<R | false>,
  ) => {
    setBusy(true);
    try {
      const result = await exec(otp);
      if (result !== false) {
        toastSuccess(success, result as R);
        onDone();
      }
    } catch {
      // The request layer has already toasted the error (the frontend has only two rules); stay put to retry.
    } finally {
      setBusy(false);
    }
  };

  return {
    otp,
    setOtp,
    otpMissing: Boolean(needOtp) && otp === '',
    busy,
    run,
  };
}

/**
 * After closing, the popup keeps drawing its last frame before closing until the exit animation ends and the popup is
 * removed (this unmounts with the popup, so the next open starts fresh). Clearing state on close is the right way for
 * a page (clear the id being edited, invalidate the fetched details), and the container's own busy state resets too,
 * but the popup still plays a 100-200 ms exit animation; without freezing, that window draws a jumping title, a
 * skeleton, an empty drawer, a reset button.
 */
export function KeepLastOpen({
  open,
  children,
}: {
  open: boolean;
  children: ReactNode;
}) {
  const [last, setLast] = useState(children);
  if (open && children !== last) setLast(children);
  return open ? children : last;
}

/**
 * Resource drawer: views one row's information, plus related summaries that fit on one screen (a related list that
 * needs paging goes to its own page). Open/closed is page state, not in the URL. Two widths: the default for ordinary
 * details, `wide` for details with many sections.
 */
export function ResourceSheet({
  open,
  onClose,
  title,
  description,
  size = 'default',
  children,
  footer,
}: {
  open: boolean;
  onClose: () => void;
  title: string;
  description?: string;
  size?: 'default' | 'wide';
  children: ReactNode;
  footer?: ReactNode;
}) {
  return (
    <Sheet open={open} onOpenChange={(next) => next || onClose()}>
      <SheetContent
        // The width must carry the same data-[side] variant prefix as the base class: the base class's
        // `data-[side=right]:sm:max-w-sm` is more specific than a bare `sm:max-w-3xl`, and tailwind-merge does not
        // see them as conflicting, so without the prefix no width takes effect.
        className={cn(
          'flex w-full flex-col gap-0 p-0',
          size === 'wide'
            ? 'data-[side=right]:sm:max-w-3xl'
            : 'data-[side=right]:sm:max-w-md',
        )}
      >
        <KeepLastOpen open={open}>
          <SheetHeader className="border-b">
            <SheetTitle>{title}</SheetTitle>
            {description ? (
              <SheetDescription>{description}</SheetDescription>
            ) : (
              // A Sheet must have a readable title and description; without a description give a hidden one, so a
              // screen reader does not stop on a dialog with only a title.
              <SheetDescription className="sr-only">{title}</SheetDescription>
            )}
          </SheetHeader>
          <div className="flex-1 overflow-y-auto p-4">{children}</div>
          {footer ? <div className="border-t p-4">{footer}</div> : null}
        </KeepLastOpen>
      </SheetContent>
    </Sheet>
  );
}

const FORM_DIALOG_WIDTH = {
  default: 'sm:max-w-lg',
  wide: 'sm:max-w-3xl',
  xl: 'sm:max-w-5xl',
};

/**
 * Form dialog: anything that submits a form, such as create and edit, uses it. It owns only the rules of the shell;
 * the page writes the fields inside the shadcn way:
 *
 * - **A click outside does not close it**: a stray click on the list behind it, or the mouse sliding out and
 *   clicking, is the most common way to lose a whole form. Only Esc, Cancel and the × at the top right close it.
 * - **Confirm when edited**: if the values differ from the initial values at open, ask "Discard unsaved changes?"
 *   before closing; close directly when unchanged. The form library decides "edited", the page reports nothing.
 * - **The button is disabled while submitting to prevent double clicks**; failure stays put (the request layer has
 *   already toasted the error), only success closes it.
 * - Optionally carries **one** one-time code input (`needOtp`); the code goes to onSubmit with the submission. A
 *   form has at most one step-up point (ADR admin-0007).
 *
 * Three widths: the default for ordinary forms with few fields, `wide` for large forms that fit in two columns, like
 * the role configuration, `xl` for settings forms that would be longer than a screen even in two columns and need
 * three. Pick a width only from these; no className, so forms of a kind have the same width. Fills the screen on
 * narrow screens.
 */
export function FormDialog<T extends FieldValues, R>({
  open,
  onClose,
  title,
  description,
  size = 'default',
  form,
  onSubmit,
  needOtp,
  queries,
  loading: busy,
  success,
  children,
}: {
  open: boolean;
  onClose: () => void;
  title: string;
  description?: string;
  size?: keyof typeof FORM_DIALOG_WIDTH;
  form: UseFormReturn<T>;
  /**
   * Submit. A throw = failure (the request layer has already toasted the error), the dialog stays put; returning
   * `false` = the page blocked it itself (for example, marked a field with form.setError), also stays put; anything
   * else is success.
   */
  onSubmit: (values: T, otpCode: string) => Promise<R | false>;
  needOtp?: boolean;
  /**
   * Data the form needs (details when editing, dropdown options): pass the `useQuery` result directly. Not yet
   * arrived: the body shows a skeleton and Save is disabled, so nobody starts filling an empty form; failed: the body
   * becomes "Failed to load" with a retry, Save is disabled too, so an empty form is never submitted as the current
   * values. A query that is not enabled (details when creating) does not count.
   */
  queries?: readonly QueryLike[];
  /**
   * Other things to wait for besides data (such as the form finishing its backfill): shows only a skeleton, never
   * fails.
   */
  loading?: boolean;
  /** The sentence to say after success; the function form gets onSubmit's return value (see `SuccessText`). */
  success?: SuccessText<R>;

  children: ReactNode;
}) {
  const { t } = useTranslation();
  const otpId = useId();
  const [tried, setTried] = useState(false);
  const [confirming, setConfirming] = useState(false);
  const flow = useConfirmFlow({ open, needOtp, success, onDone: onClose });

  // Clear the previous round's submission traces on every open (useConfirmFlow clears the code).
  useEffect(() => {
    if (open) {
      setTried(false);
      setConfirming(false);
    }
  }, [open]);

  const state = queries ? loadState(queries) : 'ready';
  const loading = state === 'loading' || Boolean(busy);
  const failed = state === 'failed';

  const { isDirty, isSubmitting } = form.formState;

  const requestClose = () => {
    if (isSubmitting) return;
    if (isDirty) setConfirming(true);
    else onClose();
  };

  // The code is one item of the form: following the form rules it is flagged only on submit, focused and marked red.
  const submit = form.handleSubmit(async (values) => {
    if (flow.otpMissing) {
      document.getElementById(otpId)?.focus();
      return;
    }
    await flow.run((otp) => onSubmit(values, otp));
  });

  return (
    <Dialog
      open={open}
      onOpenChange={(next) => {
        if (!next) requestClose();
      }}
      disablePointerDismissal
    >
      <DialogContent
        className={cn(
          'flex max-h-[calc(100dvh-2rem)] flex-col gap-0 p-0',
          FORM_DIALOG_WIDTH[size],
          // While the "discard changes" confirmation sits above it, this layer shrinks, moves down and dims (the Base
          // UI
          // nested dialog pattern): the upper layer draws no backdrop, and two same-colored layers without this
          // separation do not show which one is on top.
          'translate-y-[calc(-50%+1.25rem*var(--nested-dialogs))] scale-[calc(1-0.1*var(--nested-dialogs))] transition-[translate,scale] ease-out',
          'after:pointer-events-none after:absolute after:inset-0 after:rounded-[inherit] after:bg-black/5 after:opacity-0 after:transition-opacity data-nested-dialog-open:after:opacity-100',
          // Fill the screen on narrow screens: a long form scrolling in a small padded box is harder to fill in.
          'max-sm:top-0 max-sm:left-0 max-sm:h-dvh max-sm:max-h-none max-sm:max-w-none max-sm:translate-x-0 max-sm:translate-y-0 max-sm:rounded-none',
        )}
      >
        {/* The form layer is what freezes; the discard confirmation sits outside and plays its own exit on close. */}
        <KeepLastOpen open={open}>
          <form
            noValidate
            className="flex min-h-0 flex-1 flex-col"
            onSubmit={(e) => {
              setTried(true);
              void submit(e);
            }}
          >
            <DialogHeader className="border-b p-4 pr-12">
              <DialogTitle>{title}</DialogTitle>
              {/* A dialog must have a readable description; without one give a hidden one, so a screen reader does not
                  stop on a box with only a title. */}
              <DialogDescription className={cn(!description && 'sr-only')}>
                {description ?? title}
              </DialogDescription>
            </DialogHeader>
            <div className="min-h-0 flex-1 overflow-y-auto p-4">
              {failed ? (
                <LoadFailed onRetry={() => retryFailed(queries ?? [])} />
              ) : loading ? (
                <div className="flex flex-col gap-3">
                  <Skeleton className="h-8 w-full" />
                  <Skeleton className="h-8 w-full" />
                  <Skeleton className="h-8 w-2/3" />
                </div>
              ) : (
                <FieldGroup>
                  {children}
                  {needOtp ? (
                    <OtpField
                      id={otpId}
                      value={flow.otp}
                      onChange={flow.setOtp}
                      invalid={tried && flow.otpMissing}
                    />
                  ) : null}
                </FieldGroup>
              )}
            </div>
            <DialogFooter className="m-0">
              <Button type="button" variant="outline" onClick={requestClose}>
                {t('action.cancel')}
              </Button>
              <Button
                type="submit"
                disabled={loading || failed || isSubmitting}
              >
                {isSubmitting ? t('action.saving') : t('action.save')}
              </Button>
            </DialogFooter>
          </form>
        </KeepLastOpen>

        {/* Rendered inside the form dialog, so focus and Esc across the two layers close the upper one first. */}
        <AlertDialog open={confirming} onOpenChange={setConfirming}>
          <AlertDialogContent>
            <AlertDialogHeader>
              <AlertDialogTitle>{t('form.discardTitle')}</AlertDialogTitle>
              <AlertDialogDescription>
                {t('form.discardDesc')}
              </AlertDialogDescription>
            </AlertDialogHeader>
            <AlertDialogFooter>
              <AlertDialogCancel>{t('form.keepEditing')}</AlertDialogCancel>
              <AlertDialogAction
                variant="destructive"
                onClick={() => {
                  setConfirming(false);
                  onClose();
                }}
              >
                {t('form.discard')}
              </AlertDialogAction>
            </AlertDialogFooter>
          </AlertDialogContent>
        </AlertDialog>
      </DialogContent>
    </Dialog>
  );
}

/**
 * Action dialog: confirms one one-off action, optionally collecting an OTP code along the way.
 *
 * Actions with `needOtp` collect their code here: one action, one point, so it does not compete with other forms for
 * the headroom of `sys.otp.max_uses` (ADR admin-0007).
 *
 * Deliberately no `<form>` and no Enter: these actions often cannot be undone and need a deliberate click on Confirm;
 * Enter is too easy, and typing the code and pressing it would execute. That the form dialog submits on Enter is no
 * reason to match it here.
 */
export function ActionDialog<R>({
  open,
  onOpenChange,
  title,
  description,
  needOtp,
  confirmLabel,
  destructive,
  confirmDisabled,
  queries,
  success,
  onConfirm,
  children,
}: {
  open: boolean;
  onOpenChange: (open: boolean) => void;
  title: string;
  description?: string;
  needOtp?: boolean;
  confirmLabel?: string;
  destructive?: boolean;
  /**
   * The action's own parameters are not filled in yet (such as a new password too short for a reset): the confirm
   * button is disabled. **Do not use it to mean "submitting"**: the dialog disables itself while submitting.
   */
  confirmDisabled?: boolean;
  /**
   * Data the action's parameters need (such as dropdown options): pass the `useQuery` result directly, judged as in
   * `FormDialog`: not yet arrived, the parameter area shows a skeleton; failed, it becomes "Failed to load" with a
   * retry; neither allows confirming. A query that is not enabled does not count.
   */
  queries?: readonly QueryLike[];
  /** The sentence to say after success; the function form gets onConfirm's return value (see `SuccessText`). */
  success?: SuccessText<R>;
  /**
   * Confirm. The argument is the collected OTP code (an empty string without needOtp). A throw = failure, the dialog
   * stays put to retry (the request layer has already toasted the error); returning `false` = the page blocked it
   * itself, also stays put; anything else is success, and the dialog closes itself.
   */
  onConfirm: (otpCode: string) => R | false | Promise<R | false>;
  /** Other parameters the action collects (such as the new password for a reset). */
  children?: ReactNode;
}) {
  const { t } = useTranslation();
  const state = queries ? loadState(queries) : 'ready';
  // Disabling while submitting, staying put on failure and closing on success all belong to the container
  // (useConfirmFlow): the page gives only a confirm function. Making every page write this again means one copy misses
  // a spot, and there is one more place where you can click again before the request returns.
  const { otp, setOtp, otpMissing, busy, run } = useConfirmFlow({
    open,
    needOtp,
    success,
    onDone: () => onOpenChange(false),
  });

  return (
    <AlertDialog
      open={open}
      onOpenChange={(next) => {
        // Do not close while the request is running: Esc and backdrop clicks are stopped here, Cancel is disabled
        // separately.
        if (busy) return;
        onOpenChange(next);
      }}
    >
      <AlertDialogContent>
        <KeepLastOpen open={open}>
          <AlertDialogHeader>
            <AlertDialogTitle>{title}</AlertDialogTitle>
            {/* Error text is toasted by the global rules, the dialog does not repeat it; otherwise the same error is
                said twice. */}
            <AlertDialogDescription>
              {description ?? title}
            </AlertDialogDescription>
          </AlertDialogHeader>
          {state === 'failed' ? (
            <LoadFailed onRetry={() => retryFailed(queries ?? [])} />
          ) : state === 'loading' ? (
            <div className="flex flex-col gap-3">
              <Skeleton className="h-8 w-full" />
              <Skeleton className="h-8 w-2/3" />
            </div>
          ) : (
            <FieldGroup>
              {children}
              {needOtp ? <OtpField value={otp} onChange={setOtp} /> : null}
            </FieldGroup>
          )}
          <AlertDialogFooter>
            <AlertDialogCancel disabled={busy}>
              {t('action.cancel')}
            </AlertDialogCancel>
            <AlertDialogAction
              disabled={
                busy || state !== 'ready' || confirmDisabled || otpMissing
              }
              className={cn(
                destructive &&
                  'bg-destructive text-white hover:bg-destructive/90',
              )}
              onClick={(e) => {
                // Not the default "close on click": failure stays put to retry, useConfirmFlow closes on success.
                e.preventDefault();
                void run(onConfirm);
              }}
            >
              {confirmLabel ?? t('action.confirm')}
            </AlertDialogAction>
          </AlertDialogFooter>
        </KeepLastOpen>
      </AlertDialogContent>
    </AlertDialog>
  );
}

/**
 * Popover confirmation: a button shown directly on the page (on a row, in the page header, on the bulk bar) may use it
 * to confirm one action in a popover anchored to the button, instead of a dialog; the page decides by consistency
 * across the page and feel (the container rules). An action inside a "⋯" menu cannot use it: once the menu closes, the
 * popover has no button to anchor to; RowActions does this split for table row actions itself.
 *
 * Same conventions as ActionDialog: optionally collects a one-time code; failure stays put (the error has already been
 * toasted), only success closes it; deliberately no Enter either, for the reason given on ActionDialog.
 */
export function ConfirmPopover<R>({
  trigger,
  title,
  description,
  confirmLabel,
  destructive,
  needOtp,
  success,
  onConfirm,
}: {
  /** The trigger button itself (a `<Button>` element); the popover is anchored to it. */
  trigger: ReactElement;
  title: string;
  description?: string;
  confirmLabel?: string;
  destructive?: boolean;
  needOtp?: boolean;
  /** The sentence to say after success; the function form gets onConfirm's return value (see `SuccessText`). */
  success?: SuccessText<R>;
  /** As in ActionDialog: a throw or `false` stays put, anything else is success and closes. */
  onConfirm: (otpCode: string) => Promise<R | false>;
}) {
  const { t } = useTranslation();
  const [open, setOpen] = useState(false);
  const close = () => setOpen(false);
  const { otp, setOtp, otpMissing, busy, run } = useConfirmFlow({
    open,
    needOtp,
    success,
    onDone: close,
  });

  return (
    <Popover
      open={open}
      onOpenChange={(next) => {
        if (busy) return;
        if (next) setOpen(true);
        else close();
      }}
    >
      <PopoverTrigger render={trigger} />
      <PopoverContent align="end">
        <KeepLastOpen open={open}>
          <PopoverHeader>
            <PopoverTitle>{title}</PopoverTitle>
            {description ? (
              <PopoverDescription>{description}</PopoverDescription>
            ) : null}
          </PopoverHeader>
          {needOtp ? <OtpField value={otp} onChange={setOtp} /> : null}
          <div className="flex justify-end gap-2">
            <Button size="sm" variant="outline" disabled={busy} onClick={close}>
              {t('action.cancel')}
            </Button>
            <Button
              size="sm"
              variant={destructive ? 'destructive' : 'default'}
              disabled={busy || otpMissing}
              onClick={() => void run(onConfirm)}
            >
              {confirmLabel ?? t('action.confirm')}
            </Button>
          </div>
        </KeepLastOpen>
      </PopoverContent>
    </Popover>
  );
}

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
