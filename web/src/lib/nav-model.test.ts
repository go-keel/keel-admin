import { MegaphoneIcon, StarIcon } from 'lucide-react';
import { describe, expect, it } from 'vitest';
import type { PermCode } from '@/generated/perm-code';
import type { NavGroup, NavLeaf } from '@/nav';
import {
  buildMenu,
  FAVORITES_KEY,
  type MenuContext,
  type MenuItem,
} from './nav-model';
import type { Profile } from './perm';
import type { RouteLayers } from './route-meta';

const root: Profile = { isRoot: true, perms: [] };
const user = (perms: string[]): Profile => ({ isRoot: false, perms });

/**
 * Business paths are fictional (`/orders`): the menu's path type comes from the generated route tree, and kernel tests
 * cannot depend on whether a derived repo keeps some business page, so they are asserted to the path type.
 */
const biz = (path: string) => path as NavLeaf['path'];

/**
 * Fake route table: only registered paths "exist"; unregistered → undefined = route deleted. Values are the layers,
 * outermost first.
 */
const routes: Record<string, RouteLayers> = {
  '/': [{ titleKey: 'app:dashboard.title' }],
  '/orders': [
    { titleKey: 'app:orders.title', perm: 'ops:order:view' as PermCode }, // made-up business code
    { titleKey: 'app:orders.list' },
  ],
  '/returns': [
    { titleKey: 'app:returns.title', perm: 'ops:return:view' as PermCode },
  ],
  '/sys/users': [{ titleKey: 'nav.users', perm: 'sys:account:view' }],
  '/sys/roles': [{ titleKey: 'nav.roles', root: true }],
  '/sys/depts': [{ titleKey: 'nav.depts', perm: 'sys:dept:view' }],
  '/sys/audit': [{ titleKey: 'nav.audit', perm: 'sys:audit:view' }],
  '/sys/login-logs': [{ titleKey: 'nav.loginLogs', perm: 'sys:audit:view' }],
};

const business: NavGroup[] = [
  {
    labelKey: 'app:nav.dashboards',
    collapsible: true,
    items: [{ path: '/' }],
  },
  {
    labelKey: 'app:nav.orders',
    items: [{ path: biz('/orders'), icon: MegaphoneIcon }],
  },
];

const ctx = (over: Partial<MenuContext> = {}): MenuContext => ({
  profile: root,
  layersOf: (path) => routes[path],
  currentPage: '/',
  favorites: [],
  ...over,
});

const keys = (groups: { key: string }[]) => groups.map((g) => g.key);
/** Top-level items listed by URL; a folder parent lists its display-name key. */
const paths = (items: MenuItem[]) =>
  items.map((i) => (i.kind === 'page' ? i.path : i.labelKey));

/** One folder parent with two pages under it: orders need the order view code, returns the return view code. */
const withFolder: NavGroup[] = [
  {
    items: [
      {
        labelKey: 'app:nav.trade',
        children: [{ path: biz('/orders') }, { path: biz('/returns') }],
      },
    ],
  },
];
const pageOf = (item: MenuItem | undefined) =>
  item?.kind === 'page' ? item : undefined;
const folderOf = (menu: { items: MenuItem[] }[]) => {
  const item = menu[0]?.items[0];
  return item?.kind === 'folder' ? item : undefined;
};

describe('groups and order', () => {
  it('favorites always first, System always last, declaration order in between', () => {
    const menu = buildMenu(business, ctx({ favorites: ['/orders'] }));
    expect(keys(menu)).toEqual([FAVORITES_KEY, 'nav-0', 'nav-1', 'nav-2']);
    expect(menu[0]?.labelKey).toBe('nav.favorites');
    expect(menu[1]?.labelKey).toBe('app:nav.dashboards');
    expect(menu[3]?.labelKey).toBe('nav.system');
    expect(paths(menu[3]?.items ?? [])).toEqual([
      '/sys/users',
      '/sys/roles',
      '/sys/depts',
      '/sys/audit',
      '/sys/login-logs',
    ]);
  });

  it('the whole favorites group is absent when there are no favorites', () => {
    expect(keys(buildMenu(business, ctx()))).toEqual([
      'nav-0',
      'nav-1',
      'nav-2',
    ]);
  });

  it('a group without a label cannot collapse; the label is still given', () => {
    const menu = buildMenu(business, ctx());
    expect(menu[0]?.collapsible).toBe(true);
    expect(menu[1]?.collapsible).toBe(false);
  });

  it('the kernel favorites and System groups do not collapse and ignore the collapsible of business groups', () => {
    const menu = buildMenu(business, ctx({ favorites: ['/orders'] }));
    expect(menu[0]?.key).toBe(FAVORITES_KEY);
    expect(menu[0]?.collapsible).toBe(false);
    expect(menu[3]?.labelKey).toBe('nav.system');
    expect(menu[3]?.collapsible).toBe(false);
  });
});

describe('filtering by permission', () => {
  it('the whole System group is absent when no system page is accessible', () => {
    const menu = buildMenu(business, ctx({ profile: user([]) }));
    expect(keys(menu)).toEqual(['nav-0']); // orders need a permission code, so they are gone too
    expect(paths(menu[0]?.items ?? [])).toEqual(['/']);
  });

  it('keeps only permitted items; root-only pages are absent for non-root', () => {
    const menu = buildMenu(
      business,
      ctx({ profile: user(['sys:account:view', 'sys:audit:view']) }),
    );
    expect(paths(menu.at(-1)?.items ?? [])).toEqual([
      '/sys/users',
      '/sys/audit',
      '/sys/login-logs',
    ]);
  });

  it('folder parent: lists only permitted children', () => {
    const menu = buildMenu(
      withFolder,
      ctx({ profile: user(['ops:order:view']) }),
    );
    expect(paths(folderOf(menu)?.children ?? [])).toEqual(['/orders']);
  });

  it('folder parent: absent when no child is permitted, and so is its whole group', () => {
    const menu = buildMenu(withFolder, ctx({ profile: user([]) }));
    expect(keys(menu)).toEqual([]);
  });
});

describe('favorites group', () => {
  it('drops deleted routes and unpermitted items, keeping the favorites order', () => {
    const menu = buildMenu(
      business,
      ctx({
        profile: user(['sys:account:view']),
        favorites: ['/sys/users', '/gone', '/sys/roles'],
      }),
    );
    expect(paths(menu[0]?.items ?? [])).toEqual(['/sys/users']);
  });

  it('reuses the menu icon of the item; items without one in the menu fall back to the star', () => {
    const menu = buildMenu(
      business,
      ctx({ favorites: ['/orders', '/sys/users'] }),
    );
    expect(menu[0]?.items[0]?.icon).toBe(MegaphoneIcon);
    // the System group brings its own icons; the favorite gets the same one, not the star
    expect(menu[0]?.items[1]?.icon).not.toBe(StarIcon);
    // the item with no icon in the menu ('/' under the dashboard) falls back to the star
    expect(
      buildMenu(business, ctx({ favorites: ['/'] }))[0]?.items[0]?.icon,
    ).toBe(StarIcon);
  });
});

describe('what each item carries', () => {
  it('the item of the current page is active, the others are not', () => {
    const menu = buildMenu(business, ctx({ currentPage: '/orders' }));
    expect(pageOf(menu[0]?.items[0])?.active).toBe(false); // '/'
    expect(pageOf(menu[1]?.items[0])?.active).toBe(true); // '/orders'
  });

  it('folder parent: counts as inside the group only while on one of its children', () => {
    const inside = (currentPage: string) =>
      folderOf(buildMenu(withFolder, ctx({ currentPage })))?.inside;
    expect(inside('/returns')).toBe(true);
    expect(inside('/sys/users')).toBe(false);
  });

  it('menu items always show the page name, children and favorites too', () => {
    const menu = buildMenu(withFolder, ctx({ favorites: ['/orders'] }));
    // '/orders' is both a page and its default sub-view; the page layer is taken
    expect(pageOf(menu[0]?.items[0])?.titleKey).toBe('app:orders.title');
    expect(folderOf(menu.slice(1))?.children[0]?.titleKey).toBe(
      'app:orders.title',
    );
  });

  it('an icon registered on a child is reused once favorited', () => {
    const menu = buildMenu(
      [
        {
          items: [
            {
              labelKey: 'app:nav.trade',
              children: [{ path: biz('/returns'), icon: MegaphoneIcon }],
            },
          ],
        },
      ],
      ctx({ favorites: ['/returns'] }),
    );
    expect(menu[0]?.items[0]?.icon).toBe(MegaphoneIcon);
  });

  it('the title is a key, translation is not done here; empty when the route has no title', () => {
    const menu = buildMenu(business, ctx());
    expect(pageOf(menu[0]?.items[0])?.titleKey).toBe('app:dashboard.title');
    const noMeta = buildMenu([{ items: [{ path: '/login' }] }], ctx());
    expect(pageOf(noMeta[0]?.items[0])).toBeDefined();
    expect(pageOf(noMeta[0]?.items[0])?.titleKey).toBeUndefined();
  });
});
