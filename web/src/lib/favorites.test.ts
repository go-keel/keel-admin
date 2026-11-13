import { describe, expect, it } from 'vitest';
import { favoritePaths, visibleFavorites } from './favorites';
import type { Profile } from './perm';
import type { RouteLayers } from './route-meta';

const user = (perms: string[]): Profile => ({ perms, isRoot: false });
const root: Profile = { isRoot: true, perms: [] };

/** Route table: only registered paths "exist"; unregistered → undefined = route deleted. */
const routes: Record<string, RouteLayers> = {
  '/': [{}],
  '/sys/users': [{ perm: 'sys:account:view' }],
  '/sys/roles': [{ root: true }],
};
const layersOf = (path: string): RouteLayers | undefined => routes[path];

describe('favoritePaths', () => {
  it('gives an empty array when the setting is absent entirely', () => {
    expect(favoritePaths(undefined)).toEqual([]);
    expect(favoritePaths({ isRoot: false })).toEqual([]);
    expect(favoritePaths({ settings: {} })).toEqual([]);
  });

  it('keeps the order as is: the order is the favorites order', () => {
    const profile = { settings: { favoriteMenus: ['/b', '/a'] } };
    expect(favoritePaths(profile)).toEqual(['/b', '/a']);
  });
});

describe('visibleFavorites', () => {
  it('filters out deleted routes', () => {
    const paths = ['/', '/gone'];
    expect(visibleFavorites(paths, root, layersOf)).toEqual(['/']);
  });

  it('filters out items without permission and keeps those with it', () => {
    const paths = ['/', '/sys/users', '/sys/roles'];
    expect(
      visibleFavorites(paths, user(['sys:account:view']), layersOf),
    ).toEqual(['/', '/sys/users']);
  });

  it('root sees everything', () => {
    const paths = ['/', '/sys/users', '/sys/roles'];
    expect(visibleFavorites(paths, root, layersOf)).toEqual(paths);
  });

  it('keeps the favorites order rather than the route table order', () => {
    const paths = ['/sys/users', '/'];
    expect(visibleFavorites(paths, root, layersOf)).toEqual([
      '/sys/users',
      '/',
    ]);
  });
});
