import { registerTimeZones } from '@/lib/timezone';

/**
 * Time zone registry (assembly seam): what the header bar time zone dropdown offers besides UTC; a derived repo appends
 * here. id must be an IANA time zone supported by the browser's Intl; the display name entry goes in business copy
 * (`locales/app/`); without labelKey fallbackLabel is shown as is; write shortName where Intl gives no common
 * abbreviation (such as CST, IST).
 *
 * The server's business time zone need not be registered here: the kernel adds it to the dropdown once received;
 * registering it here only gives it a translated display name and abbreviation.
 */
registerTimeZones(
  // Example: delete after forking, or replace with your own
  {
    id: 'Asia/Shanghai',
    labelKey: 'app:timezone.beijing',
    fallbackLabel: 'Beijing Time',
    shortName: 'CST',
  },
);
