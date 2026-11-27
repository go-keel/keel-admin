import { describe, expect, it } from 'vitest';
import { addIPEntries, firstInvalidIPEntry, ipEntries } from './ip-whitelist';

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

describe('allowlist tags', () => {
  it('adds a pasted list split on commas, semicolons, spaces and line breaks, skipping duplicates', () => {
    expect(
      addIPEntries(
        ['9.9.9.9'],
        '1.1.1.1, 2.2.2.2;3.3.3.3\uFF1B4.4.4.4\n9.9.9.9 \uFF0C10.0.0.0/8',
      ),
    ).toEqual({
      entries: [
        '9.9.9.9',
        '1.1.1.1',
        '2.2.2.2',
        '3.3.3.3',
        '4.4.4.4',
        '10.0.0.0/8',
      ],
      rest: '',
      bad: undefined,
    });
  });

  it('keeps bad entries out of the tags and returns them for fixing', () => {
    expect(addIPEntries([], '1.1.1.1 1.1.1.300, x')).toEqual({
      entries: ['1.1.1.1'],
      rest: '1.1.1.300 x',
      bad: '1.1.1.300',
    });
  });
});

describe('ipEntries', () => {
  it('reads a stored allowlist, whatever its spacing, without empty entries', () => {
    expect(ipEntries('1.1.1.1,2.2.2.2 , ,10.0.0.0/8,')).toEqual([
      '1.1.1.1',
      '2.2.2.2',
      '10.0.0.0/8',
    ]);
    expect(ipEntries('')).toEqual([]);
  });
});
