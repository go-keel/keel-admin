import { languages } from '@/languages';
import { shell } from '@/shell';
import { readPreference, writePreference } from './preference';

export type SupportedLocale = (typeof languages)[number]['code'];

export const SUPPORTED_LOCALES: readonly SupportedLocale[] = languages.map(
  (l) => l.code,
);

/** Language names are written in their own language and stay out of the i18n resources: translating the name of a
 * language is meaningless. */
export const LOCALE_LABELS: Record<SupportedLocale, string> =
  Object.fromEntries(languages.map((l) => [l.code, l.label])) as Record<
    SupportedLocale,
    string
  >;

const STORAGE_KEY = 'keel_locale';
const FALLBACK_LOCALE: SupportedLocale = 'en-US';

/**
 * Find which candidate code a locale belongs to: exact match first, then the first candidate of the same language
 * family, matched on the part before `-` (`ja` matches `ja-JP`). The candidate table is passed in rather than always
 * read from `SUPPORTED_LOCALES`, so unit tests can check partial matching with a constructed registry instead of adding
 * a language to the real registry for the test.
 */
export const matchLanguage = <T extends string>(
  locale: string,
  candidates: readonly T[],
): T | undefined => {
  const normalized = locale.trim().toLowerCase();
  const exact = candidates.find((code) => code.toLowerCase() === normalized);
  if (exact) return exact;
  const language = normalized.split('-')[0];
  return candidates.find(
    (code) => code.toLowerCase().split('-')[0] === language,
  );
};

/**
 * Manual choice first; then the derived repo's default (when set it beats the browser, otherwise it is as if unset);
 * then the browser language matched against the registry; if nothing is supported, fall back to English.
 */
export const resolveSupportedLocale = (
  manualLocale: string | null,
  browserLocales: readonly string[],
  fallback?: SupportedLocale,
): SupportedLocale => {
  const manual = manualLocale && matchLanguage(manualLocale, SUPPORTED_LOCALES);
  if (manual) return manual;
  if (fallback) return fallback;
  for (const locale of browserLocales) {
    const matched = matchLanguage(locale, SUPPORTED_LOCALES);
    if (matched) return matched;
  }
  return FALLBACK_LOCALE;
};

const readStored = (): string | null => readPreference(STORAGE_KEY);

const browserLocales = (): readonly string[] =>
  typeof navigator === 'undefined'
    ? []
    : navigator.languages?.length
      ? navigator.languages
      : [navigator.language];

/**
 * Initial language for i18next. Fixed mode ignores the manual choice and the browser language; the Accept-Language
 * request header is derived from getLocale(), so the server-side i18n is locked as well, with no second change needed.
 */
export const resolveInitialLocale = (): SupportedLocale =>
  shell.locale.state === 'fixed'
    ? shell.locale.value
    : resolveSupportedLocale(
        readStored(),
        browserLocales(),
        shell.locale.default,
      );

export const persistLocale = (locale: SupportedLocale): void => {
  writePreference(STORAGE_KEY, locale);
};

/**
 * Synchronous read of the current language. The request layer injects Accept-Language on every request and cannot wait
 * for i18next's React context, so the language's current value lives here and i18n.ts writes it back on switch.
 */
let current: SupportedLocale = resolveInitialLocale();

export const getLocale = (): SupportedLocale => current;

/** Called only by lib/i18n.ts; everywhere else switches language through i18n's changeLanguage. */
export const setCurrentLocale = (locale: SupportedLocale): void => {
  if (shell.locale.state === 'fixed') return;
  current = locale;
  persistLocale(locale);
};
