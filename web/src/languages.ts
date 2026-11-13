import type { Locale } from 'react-day-picker';
import { ja, zhCN } from 'react-day-picker/locale';
import appEnUS from '@/locales/app/en-US';
import appJaJP from '@/locales/app/ja-JP';
import appZhCN from '@/locales/app/zh-CN';
import sysEnUS from '@/locales/sys/en-US';
import sysJaJP from '@/locales/sys/ja-JP';
import sysZhCN from '@/locales/sys/zh-CN';
import type { DeepPartial } from '@/locales/types';

export type LanguageEntry = {
  /** Language code, used by i18next, the filter bar calendar and the Turnstile widget's language parameter */
  code: string;
  /** Name shown in the dropdown, written in the language itself: translating the word "简体中文" makes no sense, so not an
   * i18n resource */
  label: string;
  /** Business copy; missing keys fall back to English. The kernel does not check business copy for completeness; a
   * derived repo that wants to guarantee none is missing adds the `Translations<typeof appEnUS>` type constraint to its
   * copy file (`locales/app/zh-CN.ts` shows how). */
  app: DeepPartial<typeof appEnUS>;
  /** Kernel supplementary translation; may cover only part; missing keys fall back to English, no full translation
   * needed. */
  sys?: DeepPartial<typeof sysEnUS>;
  /** Locale pack of the filter bar calendar (react-day-picker, i.e. a date-fns Locale); English calendar when
   * absent. */
  calendarLocale?: Locale;
};

/**
 * Language registry (assembly seam): a derived repo appends an entry here, and the dropdown, copy fallback, calendar
 * and browser language detection (`lib/locale.ts`) all read it, with no kernel code change.
 *
 * English (`en-US`) cannot be removed: it is the fallback language; `lib/i18n.ts`'s `fallbackLng` and missing keys in
 * kernel supplementary translations rely on it. Chinese (`zh-CN`) can be removed.
 */
export const languages = [
  { code: 'en-US', label: 'English', app: appEnUS, sys: sysEnUS },
  {
    code: 'zh-CN',
    label: '简体中文',
    app: appZhCN,
    sys: sysZhCN,
    calendarLocale: zhCN,
  },
  // Example: delete after forking. Japanese is the demo language shipped upstream and shows three things: business copy
  // fully translated and self-guarded (`locales/app/ja-JP.ts` also has the `Translations<typeof appEnUS>` type
  // constraint), the kernel translated only in part with the rest falling back to English (`locales/sys/ja-JP.ts`), and
  // sidebar / actions column widths given a separate value per language (`widthFor` in `styles/` and `notices/`).
  {
    code: 'ja-JP',
    label: '日本語',
    app: appJaJP,
    sys: sysJaJP,
    calendarLocale: ja,
  },
] as const satisfies readonly LanguageEntry[];

/** Calendar locale pack for the current language; undefined when not registered (English included: it is the calendar
 * default). */
export const calendarLocaleFor = (code: string): Locale | undefined =>
  (languages as readonly LanguageEntry[]).find((l) => l.code === code)
    ?.calendarLocale;
