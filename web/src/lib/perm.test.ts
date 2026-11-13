import { describe, expect, it } from 'vitest';
import type { PermCode } from '@/generated/perm-code';
import { has, type Profile } from './perm';
import type { RouteLayers } from './route-meta';
import { canEnter } from './route-meta';

const user = (over: Partial<Profile> = {}): Profile => ({
  userId: '2',
  username: 'alice',
  isRoot: false,
  perms: ['sys:account:view'],
  ...over,
});

describe('has: permission check', () => {
  it('no code means public', () => {
    expect(has(undefined)).toBe(true);
    expect(has(user())).toBe(true);
  });

  it('passes only on a matching permission code', () => {
    expect(has(user(), 'sys:account:view')).toBe(true);
    expect(has(user(), 'sys:account:create')).toBe(false);
  });

  it('root bypasses all permission codes, even with empty perms', () => {
    const root = user({ isRoot: true, perms: [] });
    expect(has(root, 'anything:at:all' as PermCode)).toBe(true);
  });

  it('without a profile no code passes', () => {
    expect(has(undefined, 'sys:account:view')).toBe(false);
  });
});

describe('canEnter: route guard', () => {
  it('a layer with neither perm nor root is public', () => {
    expect(canEnter(user(), [])).toBe(true);
    expect(canEnter(user(), [{}, { titleKey: 'x' }])).toBe(true);
  });

  it('checks staticData.perm', () => {
    expect(canEnter(user(), [{ perm: 'sys:account:view' }])).toBe(true);
    expect(canEnter(user(), [{ perm: 'sys:dept:view' }])).toBe(false);
  });

  it('every layer must pass: the sub-view code without the page-layer code is not enough', () => {
    const layers: RouteLayers = [
      { perm: 'sys:dept:view' },
      { perm: 'sys:account:view' },
    ];
    expect(canEnter(user(), layers)).toBe(false);
    expect(
      canEnter(user({ perms: ['sys:dept:view', 'sys:account:view'] }), layers),
    ).toBe(true);
  });

  it('a root-only route admits only root; non-root cannot enter even with codes', () => {
    expect(canEnter(user({ perms: ['sys:role:view'] }), [{ root: true }])).toBe(
      false,
    );
    expect(canEnter(user({ isRoot: true }), [{ root: true }])).toBe(true);
  });
});
