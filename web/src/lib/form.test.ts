import { skipToken } from '@tanstack/react-query';
import { describe, expect, it } from 'vitest';
import { z } from 'zod';
import {
  detailFormQuery,
  freshDetailQuery,
  ipWhitelist,
  issueMessage,
  optionalEmail,
  optionalPhone,
} from './form';

// The fake t echoes the entry and its parameters: the assertions check which error maps to which entry,
// with or without numbers.
const t = (key: string, options?: Record<string, unknown>) =>
  options ? `${key} ${JSON.stringify(options)}` : key;
z.config({ customError: (iss) => issueMessage(iss, t) });

const first = (schema: z.ZodType, input: unknown) =>
  schema.safeParse(input).error?.issues[0]?.message;

describe('issueMessage', () => {
  it('an empty string is not filled in: min(1) reports required, not "at least 1 character"', () => {
    expect(first(z.string().min(1), '')).toBe('form.required');
  });

  it('length bounds carry the number', () => {
    expect(first(z.string().min(8), 'abc')).toBe('form.tooShort {"min":8}');
    expect(first(z.string().max(3), 'abcd')).toBe('form.tooLong {"max":3}');
  });

  it('multi-select needs at least one item', () => {
    expect(first(z.array(z.string()).min(1), [])).toBe(
      'form.pickAtLeast {"min":1}',
    );
  });

  it('numeric bounds', () => {
    expect(first(z.number().min(0), -1)).toBe('form.tooSmall {"min":0}');
    expect(first(z.number().max(9), 10)).toBe('form.tooBig {"max":9}');
  });

  it('undefined is not filled in; only a filled value of the wrong type is an invalid format', () => {
    expect(first(z.string(), undefined)).toBe('form.required');
    expect(first(z.string(), 3)).toBe('form.invalid');
  });

  it('uncovered errors get the common fallback, never leaking zod built-in copy', () => {
    expect(first(z.email(), 'x')).toBe('form.invalid');
  });

  it('copy set on the rule takes precedence over the global mapping', () => {
    expect(
      first(
        z.string().refine(() => false, { error: () => 'own' }),
        'x',
      ),
    ).toBe('own');
  });
});

describe('ipWhitelist', () => {
  it('empty = unrestricted, valid', () => {
    expect(ipWhitelist().safeParse('').success).toBe(true);
  });

  it('one bad entry fails: an allowlist cannot tolerate bad values, one typo would let the whole world in', () => {
    expect(ipWhitelist().safeParse('10.0.0.0/8, 999.1.1.1').success).toBe(
      false,
    );
  });

  it('too long fails: the database would silently truncate it and quietly rewrite the rule', () => {
    const long = new Array(200).fill('10.0.0.1').join(',');
    expect(ipWhitelist().safeParse(long).success).toBe(false);
  });
});

describe('optionalEmail', () => {
  it('empty = not filled in, valid', () => {
    expect(optionalEmail().safeParse('').success).toBe(true);
  });

  it('a filled value is checked as an email', () => {
    expect(optionalEmail().safeParse('alice@corp.io').success).toBe(true);
    expect(optionalEmail().safeParse('alice.corp.io').success).toBe(false);
  });
});

describe('optionalPhone', () => {
  it('empty = not filled in, valid', () => {
    expect(optionalPhone().safeParse('').success).toBe(true);
  });

  it('loose format: accepts +, spaces and hyphens, any country, landlines too', () => {
    for (const ok of ['13800001111', '+86 138-0000-1111', '010-12345678']) {
      expect(optionalPhone().safeParse(ok).success).toBe(true);
    }
  });

  it('letters, too short, too long, or non-digits at either end all fail', () => {
    for (const bad of ['138abc0000', '1234', '1'.repeat(21), '138-']) {
      expect(optionalPhone().safeParse(bad).success).toBe(false);
    }
  });
});

describe('freshDetailQuery', () => {
  const base = { queryKey: ['x'], queryFn: () => 1 };

  it('fills in what is missing: refetch on every open, drop on close', () => {
    const q = freshDetailQuery(base, true);
    expect(q.staleTime).toBe(0);
    expect(q.gcTime).toBe(0);
    expect(q.enabled).toBe(true);
  });

  it('overrides the opposite settings: a kept cache would seed the previous old values as initial values', () => {
    const q = freshDetailQuery(
      { ...base, staleTime: 60_000, gcTime: 300_000 },
      true,
    );
    expect(q.staleTime).toBe(0);
    expect(q.gcTime).toBe(0);
  });

  it('not sent while the dialog is closed; stays off when the original options turned it off', () => {
    expect(freshDetailQuery(base, false).enabled).toBe(false);
    expect(freshDetailQuery({ ...base, enabled: false }, true).enabled).toBe(
      false,
    );
  });

  it('when the original enabled is a function, sends only when it and "open" both hold', () => {
    const on = freshDetailQuery({ ...base, enabled: () => true }, true);
    const off = freshDetailQuery({ ...base, enabled: () => true }, false);
    expect((on.enabled as () => boolean)()).toBe(true);
    expect((off.enabled as () => boolean)()).toBe(false);
  });
});

describe('detailFormQuery', () => {
  const base = { queryKey: ['account'], queryFn: () => 1 };

  it('uses the fresh query when open', () => {
    expect(detailFormQuery(base, true)).toEqual(freshDetailQuery(base, true));
  });

  it('switches away on close: a mounted form with a fixed key otherwise keeps the cache and reseeds old values', () => {
    // The profile is such a case: BaseForm stays mounted with a fixed key. Departments and users escape only
    // because the id changes.
    expect(detailFormQuery(base, false)).toEqual({
      queryKey: ['form', 'no-detail'],
      queryFn: skipToken,
    });
  });

  it('creating gives no detail: no request whether open or not', () => {
    for (const open of [true, false]) {
      expect(detailFormQuery(undefined, open)).toEqual({
        queryKey: ['form', 'no-detail'],
        queryFn: skipToken,
      });
    }
  });
});
