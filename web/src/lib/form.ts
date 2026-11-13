import { zodResolver } from '@hookform/resolvers/zod';
import {
  type QueryKey,
  skipToken,
  type UseQueryOptions,
  type UseQueryResult,
  useQuery,
} from '@tanstack/react-query';
import i18n from 'i18next';
import { useEffect, useRef } from 'react';
import {
  type DefaultValues,
  type FieldValues,
  type Path,
  type UseFormReturn,
  useForm,
} from 'react-hook-form';
import { z } from 'zod';
import { firstInvalidIPEntry, MAX_IP_WHITELIST_LEN } from './ip-whitelist';

/**
 * The form base (kernel). It handles only the three things that drift apart page by page unless centralized: error
 * copy, validation timing, and returning to the initial values on every open. Fields themselves are not wrapped:
 * write them with `Controller` + `Field` as shadcn's docs do, see docs/admin/forms.md.
 */

type Translate = (key: string, options?: Record<string, unknown>) => string;

const translate: Translate = (key, options) =>
  String(i18n.t(key as never, options as never));

/**
 * One zod issue → our copy.
 *
 * zod's built-in messages are technical ("Too small: expected string to have >=1 characters"), so the common kinds
 * map to our own entries; anything not covered gets "invalid format", so a page never mixes our wording with zod's.
 * Rules that set their own `error` do not come here (zod's precedence: the rule's own > global).
 */
export const issueMessage = (
  iss: z.core.$ZodRawIssue,
  t: Translate,
): string => {
  switch (iss.code) {
    case 'too_small': {
      const min = Number(iss.minimum);
      if (iss.origin === 'string') {
        // min(1) means required: an empty text box is an empty string, not undefined.
        return min <= 1 ? t('form.required') : t('form.tooShort', { min });
      }
      if (iss.origin === 'array' || iss.origin === 'set') {
        return t('form.pickAtLeast', { min });
      }
      return t('form.tooSmall', { min });
    }
    case 'too_big': {
      const max = Number(iss.maximum);
      return iss.origin === 'string'
        ? t('form.tooLong', { max })
        : t('form.tooBig', { max });
    }
    case 'invalid_type':
      // undefined / null means not filled in; only a filled value of the wrong type is an invalid format.
      return iss.input === undefined || iss.input === null
        ? t('form.required')
        : t('form.invalid');
    default:
      return t('form.invalid');
  }
};

z.config({ customError: (iss) => issueMessage(iss, translate) });

/**
 * Use it when a rule needs its own copy: `z.string().refine(ok, { error: tr('users.xxx') })`.
 * It returns a function, so the entry is looked up **at validation time**; a `t('…')` string would freeze the copy at
 * page load, and revalidation after a language switch would still show the old language.
 */
export const tr =
  (key: string, options?: Record<string, unknown>) => (): string =>
    translate(key, options);

/**
 * Rule for the login IP allowlist: comma-separated IPs / CIDRs, empty = unrestricted. A bad entry is reported **by
 * which entry**: a vague "invalid format" cannot be located in a comma-separated string. This is input convenience;
 * the server's verdict is authoritative (see lib/ip-whitelist.ts).
 */
export const ipWhitelist = () =>
  z
    .string()
    .max(MAX_IP_WHITELIST_LEN)
    .superRefine((raw, ctx) => {
      const entry = firstInvalidIPEntry(raw);
      if (entry !== undefined) {
        ctx.addIssue({
          code: 'custom',
          message: translate('field.ipInvalid', { entry }),
        });
      }
    });

/** Loose phone format: digits, a leading +, spaces and hyphens in between, digits at both ends. */
const PHONE = /^\+?[0-9][0-9 -]*[0-9]$/;

/**
 * Email: optional, empty string = not filled in; when filled, validated as an email, at most 254 characters. Matches
 * the server contract (the email rule in api/keel_admin/v1/user.proto); the server's verdict is authoritative.
 */
export const optionalEmail = () =>
  z
    .string()
    .max(254)
    .refine((v) => v === '' || z.email().safeParse(v).success, {
      error: tr('field.emailInvalid'),
    });

/**
 * Phone: optional; when filled, only loosely checked (see PHONE), 5–20 characters, any country, landline or mobile:
 * the template serves derived repos everywhere. Matches the server contract.
 */
export const optionalPhone = () =>
  z
    .string()
    .max(20)
    .refine((v) => v === '' || (v.length >= 5 && PHONE.test(v)), {
      error: tr('field.phoneInvalid'),
    });

/**
 * Form instance: wires up zod and fixes the validation timing, then handles two lifecycle concerns.
 *
 * - **Return to the initial values on every open**: creation uses `defaultValues`; editing passes the detail endpoint's
 *   result as `values` (undefined until it arrives). The detail is **seeded only once**: a refetch on window focus
 *   while editing must not wipe half-filled input. Why initial values must come from the detail endpoint: ADR
 *   admin-0006.
 * - **Revalidate fields with errors after a language switch**: error text is produced at validation time and would
 *   otherwise stay in the old language.
 */
export function useZodForm<T extends FieldValues>(
  schema: z.ZodType<T, T>,
  {
    defaultValues,
    values,
    open = true,
  }: { defaultValues: T; values?: T; open?: boolean },
): UseFormReturn<T> {
  const form = useForm<T>({
    resolver: zodResolver(schema),
    defaultValues: defaultValues as DefaultValues<T>,
    // Validate everything on the first submit, then revalidate each field as it changes (form conventions). This is
    // RHF's default, written out so readers need not look it up.
    mode: 'onSubmit',
    reValidateMode: 'onChange',
  });

  const seeded = useRef<'defaults' | 'values' | null>(null);
  // No dependency array: look on every render; seeded ensures each open seeds only once. values and defaultValues are
  // usually objects computed in the page, so depending on them by reference would reset on every render.
  useEffect(() => {
    if (!open) {
      seeded.current = null;
      return;
    }
    if (values !== undefined) {
      if (seeded.current !== 'values') {
        form.reset(values);
        seeded.current = 'values';
      }
    } else if (seeded.current === null) {
      form.reset(defaultValues);
      seeded.current = 'defaults';
    }
  });

  useEffect(() => {
    const recheck = () => {
      const names = Object.keys(form.formState.errors) as Path<T>[];
      if (names.length > 0) void form.trigger(names);
    };
    i18n.on('languageChanged', recheck);
    return () => i18n.off('languageChanged', recheck);
  }, [form]);

  return form;
}

/**
 * Forces the detail query of an edit form to: refetch on every open (`staleTime: 0`), drop on close (`gcTime: 0`),
 * and run only while open. Without `gcTime: 0`, on the next open the previous cached result arrives first and is
 * seeded as the initial values, and the refetched value cannot get in because of "seed only once": saving writes the
 * old values back (ADR admin-0006). Whatever the page's query options say about these is ignored. A pure function,
 * covered by unit tests.
 */
export function freshDetailQuery<O extends object>(
  query: O,
  open: boolean,
): Omit<O, 'enabled'> & {
  staleTime: 0;
  gcTime: 0;
  enabled: boolean | ((...args: unknown[]) => boolean);
} {
  const base = (query as { enabled?: unknown }).enabled;
  const enabled =
    typeof base === 'function'
      ? (...args: unknown[]) => open && Boolean(base(...args))
      : open && base !== false;
  return { ...query, staleTime: 0, gcTime: 0, enabled };
}

const NO_DETAIL_KEY = ['form', 'no-detail'] as const;

/**
 * Which query the form uses right now: the fresh query when open and editing, otherwise an empty query.
 *
 * It must also switch away on close; `gcTime: 0` alone is not enough: the cache is dropped only when there are no
 * observers, and on a page whose form component stays mounted with a fixed query key (profile) the observer never
 * goes away. The previous result then stays until the next open, is seeded first, the refetched value cannot get in
 * because of "seed only once", and saving writes the old values back. Departments and users change the key with the
 * id and escape it only by luck.
 */
export function detailFormQuery<O extends object>(
  query: O | undefined,
  open: boolean,
) {
  return query && open
    ? freshDetailQuery(query, open)
    : { queryKey: NO_DETAIL_KEY, queryFn: skipToken };
}

/**
 * Forms whose initial values must be fetched fresh: editing a user, role or department, and editing the profile. The
 * four concerns of initial values live here: fetch the detail fresh (`freshDetailQuery`), use the detail only when
 * editing, convert it to form values, and seed only once (`useZodForm`).
 *
 * - `detail`: when editing, the detail query and "detail → form values"; left out when creating, so nothing is sent.
 * - Pass the returned `detail` to `FormDialog`'s `queries`: while the detail has not arrived or failed, the dialog
 *   shows a skeleton or "failed to load" and blocks saving, so an empty form cannot overwrite the original data. When
 *   creating it counts as ready and does not block saving.
 *
 * Forms that need no fresh initial values (create-only forms, change password) keep using `useZodForm`.
 */
export function useDetailForm<T extends FieldValues, D>(
  schema: z.ZodType<T, T>,
  {
    open,
    defaultValues,
    detail,
  }: {
    open: boolean;
    /** Initial values when creating. */
    defaultValues: T;
    detail?: {
      // biome-ignore lint/suspicious/noExplicitAny: query key shapes differ per domain and do not matter here
      query: UseQueryOptions<D, Error, D, any>;
      toValues: (data: D) => T;
    };
  },
): { form: UseFormReturn<T>; detail: UseQueryResult<D> } {
  const query = useQuery<D, Error, D, QueryKey>(
    detailFormQuery(detail?.query, open) as UseQueryOptions<
      D,
      Error,
      D,
      QueryKey
    >,
  );
  const form = useZodForm(schema, {
    open,
    defaultValues,
    values:
      detail && query.data !== undefined
        ? detail.toValues(query.data)
        : undefined,
  });
  return { form, detail: query };
}
