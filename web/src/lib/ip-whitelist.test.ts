import { describe, expect, it } from 'vitest';
import { firstInvalidIPEntry } from './ip-whitelist';

describe('firstInvalidIPEntry', () => {
  it('empty string = no restriction, valid', () => {
    expect(firstInvalidIPEntry('')).toBeUndefined();
    expect(firstInvalidIPEntry('  ,  ')).toBeUndefined();
  });

  it('accepts single IPs and CIDRs, both forms count', () => {
    expect(firstInvalidIPEntry('1.2.3.4')).toBeUndefined();
    expect(firstInvalidIPEntry('10.0.0.0/8, 192.168.1.1')).toBeUndefined();
    expect(firstInvalidIPEntry('::1, fd00::/8')).toBeUndefined();
  });

  it('points out the first bad entry: a whitelist cannot tolerate bad values, one typo would allow the whole world', () => {
    expect(firstInvalidIPEntry('1.2.3.4, 999.1.1.1')).toBe('999.1.1.1');
    expect(firstInvalidIPEntry('1.2.3.4.5')).toBe('1.2.3.4.5');
    expect(firstInvalidIPEntry('10.0.0.0/33')).toBe('10.0.0.0/33');
    expect(firstInvalidIPEntry('10.0.0.0/')).toBe('10.0.0.0/');
    expect(firstInvalidIPEntry('not-an-ip')).toBe('not-an-ip');
  });
});
