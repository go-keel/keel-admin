import { describe, expect, it } from 'vitest';
import { buildTree } from './tree';

const ids = (nodes: { id?: string; children: { id?: string }[] }[]) =>
  nodes.map((n) => ({ id: n.id, children: n.children.map((c) => c.id) }));

describe('buildTree', () => {
  it('builds the tree by parentId, parentId=0 is a root', () => {
    const tree = buildTree([
      { id: '1', parentId: '0' },
      { id: '2', parentId: '1' },
      { id: '3', parentId: '1' },
    ]);
    expect(ids(tree)).toEqual([{ id: '1', children: ['2', '3'] }]);
  });

  it('builds three levels too', () => {
    const tree = buildTree([
      { id: '1', parentId: '0' },
      { id: '2', parentId: '1' },
      { id: '3', parentId: '2' },
    ]);
    expect(tree[0]?.children[0]?.children[0]?.id).toBe('3');
  });

  it('attaches orphans to the root instead of silently dropping them', () => {
    // Parent 9 is not in the set (deleted / not visible without permission)
    const tree = buildTree([
      { id: '1', parentId: '0' },
      { id: '5', parentId: '9' },
    ]);
    expect(tree.map((n) => n.id).sort()).toEqual(['1', '5']);
  });

  it('a cycle does not make it loop forever', () => {
    const tree = buildTree([
      { id: '1', parentId: '2' },
      { id: '2', parentId: '1' },
    ]);
    expect(tree.length).toBeGreaterThan(0);
  });

  it('a node that is its own parent is treated as a root', () => {
    expect(buildTree([{ id: '1', parentId: '1' }]).map((n) => n.id)).toEqual([
      '1',
    ]);
  });

  it('empty input gives an empty tree', () => {
    expect(buildTree([])).toEqual([]);
  });
});
