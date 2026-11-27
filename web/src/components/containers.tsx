import {
  createContext,
  type ReactElement,
  type ReactNode,
  useContext,
  useEffect,
  useRef,
  useState,
} from 'react';
import { flushSync } from 'react-dom';
import type { FieldValues, UseFormReturn } from 'react-hook-form';
import { useTranslation } from 'react-i18next';
import { toast } from 'sonner';
import { OtpField } from '@/components/credential-inputs';
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
import { FieldGroup } from '@/components/ui/field';
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
import { isCodeRejected } from '@/lib/api';
import { loadState, type QueryLike, retryFailed } from '@/lib/load-state';
import { sectionAfterInvalidSubmit } from '@/lib/sections';
import { cn } from '@/lib/utils';

/**
 * The containers (full rules: docs/admin/forms.md, "Container roles"):
 *
 * - Dialog: a form to submit (create, edit) uses FormDialog; independent topics side by side (the profile) use
 *   SectionDialog with a SectionForm per form; confirming one action uses ActionDialog by default.
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
 * The flow for confirming one action, shared by the two confirm containers (ActionDialog, ConfirmPopover), not
 * exported: collects the one-time code in the confirmation itself, clears the previous round's code on every open (a
 * code is single-use, so submitting the old one always fails), is busy while running; a throw = failure, stay put (the
 * request layer has already toasted the error); returning `false` = the page blocked it itself, also stay put; anything
 * else is success: say the sentence (`success`), then the container closes (`onDone`). A missing code disables the
 * confirm button (like `confirmDisabled`). Forms ask for their code on Save instead (`useStepUp`).
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
      // Leave the busy state in a render of its own before closing: a closing popup freezes its last frame
      // (KeepLastOpen), which would otherwise show the busy button through the exit animation.
      flushSync(() => setBusy(false));
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

/** A section of a dialog with section navigation: one entry on the left, and the content shown for it. */
export type DialogSection = { id: string; title: string; content: ReactNode };

/** An element marking itself invalid the shadcn way (`aria-invalid` on a control, `data-invalid` on a Field/FieldSet). */
const INVALID = '[aria-invalid="true"], [data-invalid="true"]';

// While the "discard changes" confirmation sits above it, the dialog shrinks, moves down and dims (the Base UI nested
// dialog pattern): the upper layer draws no backdrop, and two same-colored layers without this separation do not show
// which one is on top.
const NESTED_DIALOG =
  'translate-y-[calc(-50%+1.25rem*var(--nested-dialogs))] scale-[calc(1-0.1*var(--nested-dialogs))] transition-[translate,scale] ease-out after:pointer-events-none after:absolute after:inset-0 after:rounded-[inherit] after:bg-black/5 after:opacity-0 after:transition-opacity data-nested-dialog-open:after:opacity-100';

// Fill the screen on narrow screens: a long form scrolling in a small padded box is harder to fill in.
const FULL_SCREEN_ON_MOBILE =
  'max-sm:top-0 max-sm:left-0 max-sm:h-dvh max-sm:max-h-none max-sm:max-w-none max-sm:translate-x-0 max-sm:translate-y-0 max-sm:rounded-none';

// The two sizes (docs/admin/forms.md, "Container roles"): the default for up to two groups of fields, and the large
// one with section navigation, whose fixed height keeps the dialog from jumping as sections switch.
const DIALOG_DEFAULT = 'max-h-[calc(100dvh-2rem)] sm:max-w-2xl';
const DIALOG_LARGE = 'sm:h-[min(42rem,calc(100dvh-2rem))] sm:max-w-4xl';

/**
 * The left navigation of a dialog with sections; on narrow screens a scrolling row on top. `flagged` sections carry
 * a red dot (a form dialog's sections with errors).
 */
function SectionNav({
  sections,
  active,
  flagged = [],
  onSelect,
}: {
  sections: readonly DialogSection[];
  active: string;
  flagged?: readonly string[];
  onSelect: (id: string) => void;
}) {
  const { t } = useTranslation();
  return (
    <nav className="flex shrink-0 gap-1 overflow-x-auto border-b p-2 sm:w-48 sm:flex-col sm:overflow-y-auto sm:border-r sm:border-b-0">
      {sections.map((s) => (
        <Button
          key={s.id}
          type="button"
          // The current item uses secondary, the rest ghost: the selected state is left to button variants.
          variant={active === s.id ? 'secondary' : 'ghost'}
          className="shrink-0 justify-start"
          aria-current={active === s.id ? 'true' : undefined}
          onClick={() => onSelect(s.id)}
        >
          <span className="truncate">{s.title}</span>
          {flagged.includes(s.id) ? (
            <span
              className="ml-auto size-1.5 shrink-0 rounded-full bg-destructive"
              role="img"
              aria-label={t('form.sectionInvalid')}
            />
          ) : null}
        </Button>
      ))}
    </nav>
  );
}

/** "Discard unsaved changes?", rendered inside the dialog it guards so focus and Esc close the upper layer first. */
function DiscardConfirm({
  open,
  onOpenChange,
  onDiscard,
}: {
  open: boolean;
  onOpenChange: (open: boolean) => void;
  onDiscard: () => void;
}) {
  const { t } = useTranslation();
  return (
    <AlertDialog open={open} onOpenChange={onOpenChange}>
      <AlertDialogContent>
        <AlertDialogHeader>
          <AlertDialogTitle>{t('form.discardTitle')}</AlertDialogTitle>
          {/* The title says it all; this is for screen readers only. */}
          <AlertDialogDescription className="sr-only">
            {t('form.discardDesc')}
          </AlertDialogDescription>
        </AlertDialogHeader>
        <AlertDialogFooter>
          <AlertDialogCancel>{t('form.keepEditing')}</AlertDialogCancel>
          <AlertDialogAction
            variant="destructive"
            onClick={() => {
              onOpenChange(false);
              onDiscard();
            }}
          >
            {t('form.discard')}
          </AlertDialogAction>
        </AlertDialogFooter>
      </AlertDialogContent>
    </AlertDialog>
  );
}

/**
 * The body of a form, shared by the form dialog and the section form: failed → "Failed to load" with a retry;
 * loading → a skeleton; otherwise the fields as rows (`data-field-rows`: fields with `orientation="responsive"` put
 * their first child, the name, in a left column; global CSS in styles/index.css), under an optional notice.
 */
function FormBody({
  state,
  queries,
  notice,
  children,
}: {
  state: 'loading' | 'failed' | 'ready';
  queries?: readonly QueryLike[];
  notice?: ReactNode;
  children: ReactNode;
}) {
  if (state === 'failed') {
    return <LoadFailed onRetry={() => retryFailed(queries ?? [])} />;
  }
  if (state === 'loading') {
    return (
      <div className="flex flex-col gap-3">
        <Skeleton className="h-8 w-full" />
        <Skeleton className="h-8 w-full" />
        <Skeleton className="h-8 w-2/3" />
      </div>
    );
  }
  return (
    <FieldGroup data-field-rows>
      {notice}
      {children}
    </FieldGroup>
  );
}

/**
 * The step-up of a form (`needOtp`): the code is not a field of the form but asked for when Save passes validation, in
 * a small dialog above the form holding only the code. One form, one submit, one code (ADR admin-0007), and the code
 * reads as "confirm this save", not as a field of whichever section is shown.
 *
 * - Enter in the code submits: Save was already a deliberate click (unlike ActionDialog, where Enter would be the only
 *   step).
 * - The server refused the code itself (wrong, already used): the prompt stays, emptied, for the next code. Any other
 *   failure closes it and goes back to the form, whose fields are what needs fixing. Either way the request layer has
 *   already said why.
 * - Without an authenticator bound, the form says so on top and Save is disabled, so nobody fills a whole form only to
 *   be stopped at the end.
 */
function useStepUp<R>({
  needOtp,
  open,
  success,
  onDone,
}: {
  needOtp?: boolean;
  open: boolean;
  success?: SuccessText<R>;
  onDone: () => void;
}) {
  const { otpBound } = useProfile();
  const [busy, setBusy] = useState(false);
  const [asking, setAsking] = useState(false);
  const [code, setCode] = useState('');
  const pending = useRef<(code: string) => Promise<R | false>>(null);

  useEffect(() => {
    if (open) setAsking(false);
  }, [open]);

  const exec = async (otp: string) => {
    const run = pending.current;
    if (!run) return;
    setBusy(true);
    try {
      const result = await run(otp);
      // As in useConfirmFlow: not busy any more before anything closes, so no frozen "Saving…" in the exit.
      flushSync(() => setBusy(false));
      setAsking(false);
      if (result !== false) {
        toastSuccess(success, result as R);
        onDone();
      }
    } catch (e) {
      // The request layer has already toasted the error.
      if (isCodeRejected(e)) setCode('');
      else setAsking(false);
    } finally {
      setBusy(false);
    }
  };

  /** Called with the validated values' submission; asks for the code first when the form needs one. */
  const submit = async (run: (code: string) => Promise<R | false>) => {
    pending.current = run;
    if (!needOtp) return exec('');
    setCode(''); // a code is single-use: never offer the previous round's
    setAsking(true);
  };

  const blocked = Boolean(needOtp) && !otpBound;
  return {
    busy,
    blocked,
    submit,
    notice: blocked ? <OtpField value="" onChange={() => {}} /> : null,
    prompt: needOtp ? (
      <OtpPrompt
        open={asking}
        busy={busy}
        code={code}
        onCode={setCode}
        onCancel={() => setAsking(false)}
        onConfirm={() => void exec(code)}
      />
    ) : null,
  };
}

/** The small dialog of `useStepUp`: the code and Confirm, nothing else. */
function OtpPrompt({
  open,
  busy,
  code,
  onCode,
  onCancel,
  onConfirm,
}: {
  open: boolean;
  busy: boolean;
  code: string;
  onCode: (code: string) => void;
  onCancel: () => void;
  onConfirm: () => void;
}) {
  const { t } = useTranslation();
  return (
    <Dialog
      open={open}
      onOpenChange={(next) => {
        if (!next && !busy) onCancel();
      }}
      disablePointerDismissal
    >
      <DialogContent className="sm:max-w-sm" showCloseButton={false}>
        <KeepLastOpen open={open}>
          <form
            noValidate
            className="flex flex-col gap-4"
            onSubmit={(e) => {
              e.preventDefault();
              // A form inside a portal still bubbles through React to the form dialog's own form: stop it there.
              e.stopPropagation();
              if (code !== '' && !busy) onConfirm();
            }}
          >
            <DialogHeader>
              <DialogTitle>{t('form.otpTitle')}</DialogTitle>
              {/* The title says it all; this is for screen readers only. */}
              <DialogDescription className="sr-only">
                {t('form.otpDesc')}
              </DialogDescription>
            </DialogHeader>
            <FieldGroup>
              <OtpField value={code} onChange={onCode} autoFocus />
            </FieldGroup>
            <DialogFooter>
              <Button
                type="button"
                variant="outline"
                disabled={busy}
                onClick={onCancel}
              >
                {t('action.cancel')}
              </Button>
              <Button type="submit" disabled={busy || code === ''}>
                {busy ? t('action.saving') : t('action.confirm')}
              </Button>
            </DialogFooter>
          </form>
        </KeepLastOpen>
      </DialogContent>
    </Dialog>
  );
}

function bodyState(
  queries: readonly QueryLike[] | undefined,
  busy: boolean | undefined,
): 'loading' | 'failed' | 'ready' {
  const state = queries ? loadState(queries) : 'ready';
  if (state === 'failed') return 'failed';
  return state === 'loading' || busy ? 'loading' : 'ready';
}

/**
 * Form dialog: anything that submits a form, such as create and edit, uses it. It owns only the rules of the shell;
 * the page writes the fields inside the shadcn way, each with `orientation="responsive"` so it is a row:
 *
 * - **A click outside does not close it**: a stray click on the list behind it, or the mouse sliding out and
 *   clicking, is the most common way to lose a whole form. Only Esc, Cancel and the × at the top right close it.
 * - **Confirm when edited**: if the values differ from the initial values at open, ask "Discard unsaved changes?"
 *   before closing; close directly when unchanged. The form library decides "edited", the page reports nothing.
 * - **The button is disabled while submitting to prevent double clicks**; failure stays put (the request layer has
 *   already toasted the error), only success closes it.
 * - With `needOtp`, Save asks for the one-time code in a small dialog once the fields pass validation (`useStepUp`);
 *   the code goes to onSubmit with the submission. A form has at most one step-up point (ADR admin-0007).
 *
 * Fields go either in `children` (up to two groups, one after the other) or, for three or more groups, in `sections`:
 * the large dialog with section navigation on the left, still one form and one Save. All sections stay mounted (only
 * the current one is shown), so their values and errors survive switching; a submit that fails validation switches to
 * the section with the first error and marks every section with an error in the navigation. No width option, so
 * forms of a kind look alike. Fills the screen on narrow screens.
 */
export function FormDialog<T extends FieldValues, R>({
  open,
  onClose,
  title,
  description,
  form,
  onSubmit,
  needOtp,
  queries,
  loading: busy,
  success,
  ...body
}: {
  open: boolean;
  onClose: () => void;
  title: string;
  description?: string;
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
} & (
  | { children: ReactNode; sections?: never }
  | { sections: readonly DialogSection[]; children?: never }
)) {
  const { t } = useTranslation();
  const formRef = useRef<HTMLFormElement>(null);
  const [tried, setTried] = useState(false);
  const [confirming, setConfirming] = useState(false);
  const sections = body.sections;
  const [active, setActive] = useState(sections?.[0]?.id ?? '');
  const [flagged, setFlagged] = useState<string[]>([]);
  const step = useStepUp({ needOtp, open, success, onDone: onClose });

  // Clear the previous round's submission traces on every open.
  // biome-ignore lint/correctness/useExhaustiveDependencies: reset only on open; sections is a fresh array every render
  useEffect(() => {
    if (open) {
      setTried(false);
      setConfirming(false);
      setFlagged([]);
      setActive(sections?.[0]?.id ?? '');
    }
  }, [open]);

  const state = bodyState(queries, busy);
  const { isDirty, errors } = form.formState;
  const submitting = step.busy;

  // Sections in error, read from the rendered fields (they mark themselves the shadcn way), so a page declares no
  // field-to-section map that could miss one. No dependency array: errors arrive with the fields' own re-render.
  useEffect(() => {
    if (!sections || !tried) return;
    const next = invalidSections(formRef.current, sections);
    setFlagged((prev) => (prev.join() === next.join() ? prev : next));
  });
  void errors; // subscribe to error changes so the effect above re-runs

  const requestClose = () => {
    if (submitting) return;
    if (isDirty) setConfirming(true);
    else onClose();
  };

  const submit = form.handleSubmit(
    (values) => step.submit((otp) => onSubmit(values, otp)),
    () => {
      if (!sections) return;
      // The library's own focus lands on a hidden field when the first error is in another section: switch to that
      // section after the fields have rendered their errors, then focus the first invalid control in it.
      requestAnimationFrame(() => {
        const invalid = invalidSections(formRef.current, sections);
        const target = sectionAfterInvalidSubmit(active, invalid);
        setActive(target);
        requestAnimationFrame(() => {
          formRef.current
            ?.querySelector<HTMLElement>(
              `[data-section-id="${target}"] [aria-invalid="true"]`,
            )
            ?.focus();
        });
      });
    },
  );

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
          'flex flex-col gap-0 p-0',
          sections ? DIALOG_LARGE : DIALOG_DEFAULT,
          NESTED_DIALOG,
          FULL_SCREEN_ON_MOBILE,
        )}
      >
        {/* The form layer is what freezes; the discard confirmation sits outside and plays its own exit on close. */}
        <KeepLastOpen open={open}>
          <form
            ref={formRef}
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
            {sections ? (
              <div className="flex min-h-0 flex-1 flex-col sm:flex-row">
                <SectionNav
                  sections={sections}
                  active={active}
                  flagged={flagged}
                  onSelect={setActive}
                />
                <div className="min-h-0 flex-1 overflow-y-auto p-4">
                  <FormBody
                    state={state}
                    queries={queries}
                    notice={step.notice}
                  >
                    {sections.map((s) => (
                      <div
                        key={s.id}
                        data-section-id={s.id}
                        hidden={s.id !== active}
                      >
                        <FieldGroup data-field-rows>{s.content}</FieldGroup>
                      </div>
                    ))}
                  </FormBody>
                </div>
              </div>
            ) : (
              <div className="min-h-0 flex-1 overflow-y-auto p-4">
                <FormBody state={state} queries={queries} notice={step.notice}>
                  {body.children}
                </FormBody>
              </div>
            )}
            <DialogFooter className="m-0">
              <Button type="button" variant="outline" onClick={requestClose}>
                {t('action.cancel')}
              </Button>
              <Button
                type="submit"
                disabled={state !== 'ready' || submitting || step.blocked}
              >
                {submitting ? t('action.saving') : t('action.save')}
              </Button>
            </DialogFooter>
          </form>
        </KeepLastOpen>

        {step.prompt}
        <DiscardConfirm
          open={confirming}
          onOpenChange={setConfirming}
          onDiscard={onClose}
        />
      </DialogContent>
    </Dialog>
  );
}

function invalidSections(
  root: HTMLElement | null,
  sections: readonly DialogSection[],
): string[] {
  if (!root) return [];
  return sections
    .filter((s) =>
      root.querySelector(`[data-section-id="${s.id}"] :is(${INVALID})`),
    )
    .map((s) => s.id);
}

/** Lets the form in the current section of a section dialog report whether it has unsaved changes. */
const SectionDirtyContext = createContext<(dirty: boolean) => void>(() => {});

/**
 * Section dialog: the large dialog with section navigation, for sections that are **independent topics**, each with
 * its own content (docs/admin/forms.md, "Container roles"): a `SectionForm` with its own Save, rows of actions, or
 * read-only information. One form split into sections is a `FormDialog` with `sections` instead.
 *
 * Only the current section is mounted: switching away drops its unsaved edits and the next visit starts from fresh
 * values. So leaving a section whose `SectionForm` has unsaved changes, by switching or by closing, first asks whether
 * to discard them. A click outside does not close it, as with the form dialog. Open/closed and the current section
 * are page state, not in the URL; every open starts on the first section.
 */
export function SectionDialog({
  open,
  onClose,
  title,
  description,
  sections,
}: {
  open: boolean;
  onClose: () => void;
  title: string;
  description?: string;
  sections: readonly DialogSection[];
}) {
  const [active, setActive] = useState(sections[0]?.id ?? '');
  const [dirty, setDirty] = useState(false);
  // The leave waiting for "discard?": a section to switch to, or null to close.
  const [leaving, setLeaving] = useState<{ to: string | null } | null>(null);

  // biome-ignore lint/correctness/useExhaustiveDependencies: reset only on open; sections is a fresh array every render
  useEffect(() => {
    if (open) {
      setActive(sections[0]?.id ?? '');
      setLeaving(null);
    }
  }, [open]);

  const leave = (to: string | null) => {
    if (to === active) return;
    if (dirty) setLeaving({ to });
    else if (to === null) onClose();
    else setActive(to);
  };

  const current = sections.find((s) => s.id === active) ?? sections[0];

  return (
    <Dialog
      open={open}
      onOpenChange={(next) => {
        if (!next) leave(null);
      }}
      disablePointerDismissal
    >
      <DialogContent
        className={cn(
          'flex flex-col gap-0 p-0',
          DIALOG_LARGE,
          NESTED_DIALOG,
          FULL_SCREEN_ON_MOBILE,
        )}
      >
        <KeepLastOpen open={open}>
          <DialogHeader className="border-b p-4 pr-12">
            <DialogTitle>{title}</DialogTitle>
            <DialogDescription className={cn(!description && 'sr-only')}>
              {description ?? title}
            </DialogDescription>
          </DialogHeader>
          <div className="flex min-h-0 flex-1 flex-col sm:flex-row">
            <SectionNav sections={sections} active={active} onSelect={leave} />
            <div
              key={current?.id}
              className="flex min-h-0 flex-1 flex-col overflow-y-auto p-4"
            >
              <SectionDirtyContext.Provider value={setDirty}>
                {current?.content}
              </SectionDirtyContext.Provider>
            </div>
          </div>
        </KeepLastOpen>

        <DiscardConfirm
          open={leaving !== null}
          onOpenChange={(next) => next || setLeaving(null)}
          onDiscard={() => {
            const to = leaving?.to ?? null;
            setDirty(false);
            if (to === null) onClose();
            else setActive(to);
          }}
        />
      </DialogContent>
    </Dialog>
  );
}

/**
 * The form of one section in a `SectionDialog`, with its own Save at the bottom. The same conventions as the form
 * dialog (`queries`, `loading`, `needOtp`, `success`, onSubmit's throw / `false` / success), except that success does
 * not close anything: the saved values become the new unchanged state, so leaving no longer asks. A change that
 * affects signing in or needs a one-time code (password, authenticator) is not a field here but a row whose button
 * opens its own dialog (docs/admin/forms.md, "Container roles").
 */
export function SectionForm<T extends FieldValues, R>({
  form,
  onSubmit,
  needOtp,
  queries,
  loading: busy,
  success,
  children,
}: {
  form: UseFormReturn<T>;
  onSubmit: (values: T, otpCode: string) => Promise<R | false>;
  needOtp?: boolean;
  queries?: readonly QueryLike[];
  loading?: boolean;
  success?: SuccessText<R>;
  children: ReactNode;
}) {
  const { t } = useTranslation();
  const setDirty = useContext(SectionDirtyContext);
  const step = useStepUp({
    needOtp,
    open: true,
    success,
    // Saved: what is on screen is now the unchanged state, so leaving no longer asks.
    onDone: () => form.reset(form.getValues()),
  });

  const { isDirty } = form.formState;
  useEffect(() => setDirty(isDirty), [isDirty, setDirty]);
  useEffect(() => () => setDirty(false), [setDirty]);

  const state = bodyState(queries, busy);
  const submit = form.handleSubmit((values) =>
    step.submit((otp) => onSubmit(values, otp)),
  );

  return (
    <>
      <form
        noValidate
        className="flex flex-col gap-4"
        onSubmit={(e) => void submit(e)}
      >
        <FormBody state={state} queries={queries} notice={step.notice}>
          {children}
        </FormBody>
        <div className="flex justify-end">
          <Button
            type="submit"
            disabled={state !== 'ready' || step.busy || step.blocked}
          >
            {step.busy ? t('action.saving') : t('action.save')}
          </Button>
        </div>
      </form>
      {step.prompt}
    </>
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
