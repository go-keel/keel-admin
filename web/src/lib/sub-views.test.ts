import { describe, expect, it } from 'vitest';
import type { PermCode } from '@/generated/perm-code';
import type { Profile } from './perm';
import type { RouteLayers } from './route-meta';
import { subViewTabs } from './sub-views';

const user = (perms: string[]): Profile => ({ isRoot: false, perms });

// Made-up business codes: these tests exercise the routing logic, not the registry, so they cast to PermCode.
const page = { titleKey: 'orders', perm: 'ops:order:view' as PermCode };
const routes: Record<string, RouteLayers> = {
  '/orders': [page, { titleKey: 'orders.list' }],
  '/orders/returns': [
    page,
    { titleKey: 'orders.returns', perm: 'ops:order:return' as PermCode },
  ],
};
const layersOf = (path: string) => routes[path];
const tabs = ['/orders', '/orders/returns', '/orders/gone'];

describe('subViewTabs', () => {
  it('gives sub-view names in list order, leaving out missing routes', () => {
    expect(subViewTabs(tabs, { isRoot: true }, layersOf)).toEqual([
      { path: '/orders', titleKey: 'orders.list' },
      { path: '/orders/returns', titleKey: 'orders.returns' },
    ]);
  });

  it('a sub-view whose own code is missing does not appear', () => {
    expect(
      subViewTabs(tabs, user(['ops:order:view']), layersOf).map((t) => t.path),
    ).toEqual(['/orders']);
  });

  it('a missing code at the page level hides the sub-view even if it has its own code', () => {
    expect(subViewTabs(tabs, user(['ops:order:return']), layersOf)).toEqual([]);
  });
});
