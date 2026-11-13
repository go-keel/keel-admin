import { describe, expect, it } from 'vitest';
import {
  catalogOf,
  dependentsOf,
  depsClosure,
  deselect,
  lockedMap,
  matchesQuery,
  selectWithDeps,
  toggleMany,
} from './perm-deps';

// view ← update ← delete: delete depends on update, update depends on view.
const DEFS = [
  { code: 'sys:account:view', deps: [] },
  { code: 'sys:account:update', deps: ['sys:account:view'] },
  { code: 'sys:account:delete', deps: ['sys:account:update'] },
  { code: 'ops:notice:view', deps: [] },
];

describe('selectWithDeps', () => {
  it('ticking a code ticks its whole dependency chain, the way the server expands it', () => {
    expect(selectWithDeps([], 'sys:account:delete', DEFS).sort()).toEqual([
      'sys:account:delete',
      'sys:account:update',
      'sys:account:view',
    ]);
  });

  it('does not add an already ticked code twice', () => {
    const once = selectWithDeps([], 'sys:account:update', DEFS);
    expect(selectWithDeps(once, 'sys:account:update', DEFS).sort()).toEqual(
      once.sort(),
    );
  });

  it('a code without dependencies adds only itself', () => {
    expect(selectWithDeps([], 'ops:notice:view', DEFS)).toEqual([
      'ops:notice:view',
    ]);
  });
});

describe('dependentsOf / deselect', () => {
  const all = ['sys:account:view', 'sys:account:update', 'sys:account:delete'];

  it('a code something depends on cannot be unticked', () => {
    expect(dependentsOf('sys:account:view', all, DEFS).sort()).toEqual([
      'sys:account:delete',
      'sys:account:update',
    ]);
    expect(deselect(all, 'sys:account:view', DEFS).sort()).toEqual(all.sort());
  });

  it('a code nothing depends on can be unticked', () => {
    expect(deselect(all, 'sys:account:delete', DEFS).sort()).toEqual([
      'sys:account:update',
      'sys:account:view',
    ]);
  });

  it('after unticking the top layer, the next layer is unlocked', () => {
    const step1 = deselect(all, 'sys:account:delete', DEFS);
    expect(deselect(step1, 'sys:account:update', DEFS)).toEqual([
      'sys:account:view',
    ]);
  });
});

describe('dependencies across modules and business domains', () => {
  const defs = [
    { code: 'ops:order:handle', deps: ['ops:account:view'] },
    { code: 'ops:account:view', deps: ['merchant:finance:view'] },
    { code: 'merchant:finance:view', deps: [] },
  ];
  const all = defs.map((d) => d.code);

  it('ticks the full closure and locks direct and indirect prerequisites', () => {
    const selected = selectWithDeps([], 'ops:order:handle', defs);
    expect(selected.sort()).toEqual([...all].sort());
    expect(depsClosure('ops:order:handle', defs).sort()).toEqual([
      'merchant:finance:view',
      'ops:account:view',
    ]);
    expect(
      dependentsOf('merchant:finance:view', selected, defs).sort(),
    ).toEqual(['ops:account:view', 'ops:order:handle']);
    expect(deselect(selected, 'ops:account:view', defs)).toEqual(selected);
    expect(deselect(selected, 'merchant:finance:view', defs)).toEqual(selected);
    const locks = lockedMap(selected, defs);
    expect(locks.has('ops:account:view')).toBe(true);
    expect(locks.has('merchant:finance:view')).toBe(true);
    expect(locks.has('ops:order:handle')).toBe(false);
  });

  it('unlocks layer by layer after unticking the source operation', () => {
    const step1 = deselect(all, 'ops:order:handle', defs);
    expect(step1.sort()).toEqual(['merchant:finance:view', 'ops:account:view']);
    expect(lockedMap(step1, defs).has('ops:account:view')).toBe(false);
    expect(deselect(step1, 'merchant:finance:view', defs)).toEqual(step1);
    const step2 = deselect(step1, 'ops:account:view', defs);
    expect(lockedMap(step2, defs).has('merchant:finance:view')).toBe(false);
    expect(deselect(step2, 'merchant:finance:view', defs)).toEqual([]);
  });
});

describe('catalogOf', () => {
  it('orders groups as in groups, not alphabetically by code', () => {
    const defs = [
      { code: 'sys:account:view', domain: 'sys', module: 'account' },
      { code: 'sys:audit:view', domain: 'sys', module: 'audit' },
      { code: 'sys:dept:view', domain: 'sys', module: 'dept' },
      { code: 'ops:notice:view', domain: 'ops', module: 'notice' },
    ];
    const groups = [
      { domain: 'ops', name: 'Operations' },
      { domain: 'ops', module: 'notice' },
      { domain: 'sys' },
      { domain: 'sys', module: 'account' },
      { domain: 'sys', module: 'dept' },
      { domain: 'sys', module: 'audit' },
    ];
    const cat = catalogOf(defs, groups);
    expect(cat.map((d) => d.domain)).toEqual(['ops', 'sys']);
    expect(cat[1]?.modules.map((m) => m.module)).toEqual([
      'account',
      'dept',
      'audit',
    ]);
    // without a group name it falls back to the slug
    expect(cat[0]?.name).toBe('Operations');
    expect(cat[0]?.modules[0]?.name).toBe('notice');
  });
});

describe('catalogOf display order within a module', () => {
  it('view → create/update → other → delete; within a tier fewer prerequisites first, then by code', () => {
    const d = (code: string, deps: string[] = []) => ({
      code,
      deps,
      domain: 'sys',
      module: 'account',
    });
    const defs = [
      d('sys:account:update', ['sys:account:view']),
      d('sys:account:delete', ['sys:account:view']),
      d('sys:account:kick', ['sys:account:view']),
      d('sys:account:disable', ['sys:account:view', 'sys:account:update']),
      d('sys:account:enable', ['sys:account:view']),
      d('sys:account:create', ['sys:account:view']),
      d('sys:account:view'),
    ];
    const cat = catalogOf(defs, [{ domain: 'sys', module: 'account' }]);
    expect(cat[0]?.modules[0]?.defs.map((x) => x.code)).toEqual([
      'sys:account:view',
      'sys:account:create',
      'sys:account:update',
      'sys:account:enable',
      'sys:account:kick',
      'sys:account:disable',
      'sys:account:delete',
    ]);
  });
});

describe('depsClosure', () => {
  it('follows the dependency chain recursively, excluding itself', () => {
    expect(depsClosure('sys:account:delete', DEFS).sort()).toEqual([
      'sys:account:update',
      'sys:account:view',
    ]);
    expect(depsClosure('sys:account:view', DEFS)).toEqual([]);
  });
});

describe('toggleMany', () => {
  const group = [
    'sys:account:view',
    'sys:account:update',
    'sys:account:delete',
  ];

  it('ticks and unticks a whole group', () => {
    expect(toggleMany(['ops:notice:view'], group, true, DEFS).sort()).toEqual(
      ['ops:notice:view', ...group].sort(),
    );
    expect(
      toggleMany(['ops:notice:view', ...group], group, false, DEFS),
    ).toEqual(['ops:notice:view']);
  });

  it('adds back what codes still ticked outside the group depend on', () => {
    // delete is outside this group and still ticked: view and update are its prerequisites and cannot be unticked.
    const partial = ['sys:account:view', 'sys:account:update'];
    expect(toggleMany(group, partial, false, DEFS).sort()).toEqual(
      [...group].sort(),
    );
  });
});

describe('lockedMap', () => {
  it('computes all locked codes and what locks them at once', () => {
    const map = lockedMap(['sys:account:view', 'sys:account:update'], DEFS);
    expect(map.get('sys:account:view')).toEqual(['sys:account:update']);
    expect(map.has('sys:account:update')).toBe(false);
  });

  it('unticked codes are not in the lock table', () => {
    expect(lockedMap([], DEFS).size).toBe(0);
  });
});

describe('matchesQuery', () => {
  it('finds by code or by name; an empty query lets everything through', () => {
    const def = { code: 'sys:account:view', name: 'Browse accounts' };
    expect(matchesQuery(def, '')).toBe(true);
    expect(matchesQuery(def, 'account')).toBe(true);
    expect(matchesQuery(def, 'browse')).toBe(true);
    expect(matchesQuery(def, 'ACCOUNT')).toBe(true);
    expect(matchesQuery(def, 'notice')).toBe(false);
  });
});
