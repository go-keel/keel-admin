import { useSyncExternalStore } from 'react';
import {
  getBusinessTimeZone,
  getTimeZone,
  subscribeTimeZone,
  type TimeZoneId,
} from '@/lib/timezone';

export const useTimeZone = (): TimeZoneId =>
  useSyncExternalStore(subscribeTimeZone, getTimeZone, getTimeZone);

/**
 * Business time zone: one value for the whole system, written into the time zone module when entering the auth shell
 * (see routes/_app.tsx). Read synchronously from the module, not by suspending on the query cache: components that show
 * times often appear only in tooltips and dialogs, so the cached copy is rarely read, and once evicted while idle, the
 * next read would suspend the whole page.
 */
export const useBusinessTimeZone = (): TimeZoneId =>
  useSyncExternalStore(
    subscribeTimeZone,
    getBusinessTimeZone,
    getBusinessTimeZone,
  );

/** Which time zone to read, write and display in: `display` is the header's display time zone (default), `business` is
 * the business time zone. */
export const useZone = (
  zone: 'display' | 'business' = 'display',
): TimeZoneId => {
  const display = useTimeZone();
  const business = useBusinessTimeZone();
  return zone === 'business' ? business : display;
};
