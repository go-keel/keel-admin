import i18n from 'i18next';
import { initReactI18next } from 'react-i18next';
import { type LanguageEntry, languages } from '@/languages';
import type appEnUS from '@/locales/app/en-US';
import type sysEnUS from '@/locales/sys/en-US';
import { shell } from '@/shell';
import {
  resolveInitialLocale,
  type SupportedLocale,
  setCurrentLocale,
} from './locale';

/**
 * Two namespaces: sys is the kernel (shell, exception pages, System; overwritten file by file on upgrade), app is the
 * business area (owned by the derived repo). The default ns is sys; business pages write t('app:…').
 *
 * Key types are derived from the en-US resources (see the module augmentation at the end of the file): a misspelt key
 * or a key missing from zh-CN fails tsc instead of showing the key name at runtime. The resources are built from the
 * registry in `languages.ts`; keys a kernel supplement translation lacks fall back via `fallbackLng: 'en-US'`, so a
 * full translation is not required.
 */
// Read as LanguageEntry: the registry keeps each entry's literal type, so optional fields an entry omits cannot be read.
const entries: readonly LanguageEntry[] = languages;
const resources = Object.fromEntries(
  entries.map((l) => [l.code, { sys: l.sys ?? {}, app: l.app }]),
);

void i18n.use(initReactI18next).init({
  resources,
  lng: resolveInitialLocale(),
  fallbackLng: 'en-US',
  ns: ['sys', 'app'],
  defaultNS: 'sys',
  interpolation: { escapeValue: false },
});

// The lang in index.html is the build default; the detected language must land on <html>, since screen readers and
// the browser's word breaking and spell checking all read it.
document.documentElement.lang = i18n.language;

/** Switches the language. No reload: after i18next swaps resources React re-renders, and in-page state is kept. */
export const changeLocale = async (locale: SupportedLocale): Promise<void> => {
  if (shell.locale.state === 'fixed') return;
  setCurrentLocale(locale);
  await i18n.changeLanguage(locale);
  document.documentElement.lang = locale;
};

/**
 * Read entry for dynamic keys: keys such as a time zone's labelKey come from a registry rather than literals and cannot
 * pass the key type check of t(). A missing key falls back to the caller's fallback text: a registry entry may have been
 * added by a derived repo, and the kernel resource files never had it.
 */
export const tDynamic = (key: string | undefined, fallback: string): string =>
  key && i18n.exists(key) ? i18n.t(key as never) : fallback;

// When passing t to a function, type the parameter as `TFunction<'sys'>` / `TFunction<'app'>`, not
// `ReturnType<typeof useTranslation>['t']`: taking the return type of a generic function expands the type parameters
// by their constraints rather than their defaults, the resulting t covers every namespace and key prefix, and with a
// few hundred messages tsc reports excessively deep instantiation (TS2589).
declare module 'i18next' {
  interface CustomTypeOptions {
    defaultNS: 'sys';
    resources: {
      sys: typeof sysEnUS;
      app: typeof appEnUS;
    };
  }
}

export default i18n;
