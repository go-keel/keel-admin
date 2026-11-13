/**
 * Frontend validation of the raw IP whitelist text (comma-separated IPs / CIDRs).
 *
 * This is **input experience**, not a security boundary: the real check is on the server (biz.ValidateIPWhitelist). The
 * frontend copy points out "one dot too many" before submitting, instead of making people submit and then read a vague
 * "invalid parameter". The rules deliberately match the server; when they differ, the server wins.
 */

/** Matches VARCHAR(512) of sys_user.ip_whitelist: anything longer would be silently truncated by the database. */
export const MAX_IP_WHITELIST_LEN = 512;

const IPV4 = /^(\d{1,3})\.(\d{1,3})\.(\d{1,3})\.(\d{1,3})$/;

const isIPv4 = (s: string): boolean => {
  const m = IPV4.exec(s);
  return !!m && m.slice(1).every((p) => p.length <= 3 && Number(p) <= 255);
};

// IPv6 has many forms (compressed, embedded IPv4, zone suffix), so the frontend does not replicate a parser: it rejects
// what is clearly not IPv6 and leaves the rest to the server. Better to let one through than to reject a valid one: a
// false rejection leaves people unable to enter a value the server would accept.
const isIPv6 = (s: string): boolean =>
  s.includes(':') && /^[0-9a-fA-F:.]+$/.test(s);

const isAddr = (s: string): boolean => isIPv4(s) || isIPv6(s);

const isPrefix = (s: string): boolean => {
  const [addr, bits, ...rest] = s.split('/');
  if (rest.length > 0 || !addr || !bits) return false;
  if (!/^\d{1,3}$/.test(bits)) return false;
  const max = addr.includes(':') ? 128 : 32;
  return isAddr(addr) && Number(bits) <= max;
};

/**
 * Validates the raw whitelist text and returns the first invalid entry; returns undefined when all are valid. An empty
 * string is valid = no restriction (the state of most accounts).
 *
 * Any invalid entry makes the whole invalid: a whitelist is a security boundary, and tolerating bad entries lets one
 * typo quietly allow the whole world (the same ruling as the server's parseIPWhitelist).
 */
export function firstInvalidIPEntry(raw: string): string | undefined {
  for (const item of raw.split(',')) {
    const entry = item.trim();
    if (entry === '') continue;
    if (entry.includes('/') ? !isPrefix(entry) : !isAddr(entry)) return entry;
  }
  return undefined;
}
