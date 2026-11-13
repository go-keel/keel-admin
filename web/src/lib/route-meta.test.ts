import {
  createMemoryHistory,
  createRootRoute,
  createRoute,
  createRouter,
} from '@tanstack/react-router';
import { describe, expect, it } from 'vitest';
import type { PermCode } from '@/generated/perm-code';
import {
  currentPageOf,
  pageTitleKey,
  routeLayersOf,
  viewTitleKey,
} from './route-meta';

/**
 * Uses a real router rather than a fake table: what must be locked is the routing library's behaviour, namely that
 * when the default sub-view shares the page's URL, the URL yields both layers instead of one taking the other's place.
 */
const rootRoute = createRootRoute();
const shell = createRoute({ getParentRoute: () => rootRoute, id: '_app' });
const landing = createRoute({
  getParentRoute: () => shell,
  path: '/',
  staticData: { titleKey: 'dashboard' },
});
const page = createRoute({
  getParentRoute: () => shell,
  path: 'orders',
  staticData: { titleKey: 'orders', perm: 'ops:order:view' as PermCode }, // made-up business code
});
const defaultView = createRoute({
  getParentRoute: () => page,
  path: '/',
  staticData: { titleKey: 'orders.list' },
});
const returnsView = createRoute({
  getParentRoute: () => page,
  path: 'returns',
  staticData: {
    titleKey: 'orders.returns',
    perm: 'ops:order:return' as PermCode,
  },
});
const router = createRouter({
  routeTree: rootRoute.addChildren([
    shell.addChildren([landing, page.addChildren([defaultView, returnsView])]),
  ]),
  history: createMemoryHistory(),
});

describe('routeLayersOf', () => {
  it('default sub-view sharing the page URL: both layers present, outermost first', () => {
    const layers = routeLayersOf(router, '/orders') ?? [];
    expect(layers.map((m) => m.titleKey)).toEqual([
      undefined,
      undefined,
      'orders',
      'orders.list',
    ]);
    expect(pageTitleKey(layers)).toBe('orders');
    expect(viewTitleKey(layers)).toBe('orders.list');
  });

  it('a sub-view carries the permission code of the page layer', () => {
    const layers = routeLayersOf(router, '/orders/returns') ?? [];
    expect(layers.map((m) => m.perm).filter(Boolean)).toEqual([
      'ops:order:view',
      'ops:order:return',
    ]);
    expect(viewTitleKey(layers)).toBe('orders.returns');
  });

  it('a page without layers: page name equals sub-view name', () => {
    const layers = routeLayersOf(router, '/') ?? [];
    expect(pageTitleKey(layers)).toBe('dashboard');
    expect(viewTitleKey(layers)).toBe('dashboard');
  });

  it('undefined when there is no such route; matching only outer layers also counts as none', () => {
    expect(routeLayersOf(router, '/gone')).toBeUndefined();
    expect(routeLayersOf(router, '/orders/gone')).toBeUndefined();
  });
});

describe('currentPageOf', () => {
  it('on a sub-view gives the page it belongs to', () => {
    expect(currentPageOf(router.matchRoutes('/orders/returns'))).toEqual({
      path: '/orders',
      titleKey: 'orders',
    });
    expect(currentPageOf(router.matchRoutes('/orders'))?.path).toBe('/orders');
  });

  it('drops the trailing slash when a directory index is itself the page; the landing page stays /', () => {
    expect(
      currentPageOf([{ pathname: '/reports/', staticData: { titleKey: 'r' } }]),
    ).toEqual({ path: '/reports', titleKey: 'r' });
    expect(currentPageOf(router.matchRoutes('/'))?.path).toBe('/');
  });

  it('a page without a title gives undefined', () => {
    expect(
      currentPageOf([{ pathname: '/login', staticData: {} }]),
    ).toBeUndefined();
  });
});
