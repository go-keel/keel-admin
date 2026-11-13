/**
 * Flat list → tree. The server returns flat rows with `parentId` (departments, menus and the like), and the tree shape
 * is assembled on the frontend; the assembly rule is pure logic, so it lives apart from the UI and unit tests cover
 * only this half.
 */
export type FlatNode = { id?: string; parentId?: string };

export type TreeNode<T> = T & { children: TreeNode<T>[] };

/**
 * Builds the tree by `parentId`; roots are the nodes whose `parentId` is `rootId` (default '0') or points to a missing
 * parent. **Orphans must appear in the result too**: when a parent is deleted or not visible without permission,
 * silently dropping the whole subtree makes data vanish; attached to the root it can at least be seen and edited.
 *
 * A cycle (corrupted data) does not make it loop forever: each node is attached at most once.
 */
export function buildTree<T extends FlatNode>(
  nodes: readonly T[],
  rootId = '0',
): TreeNode<T>[] {
  const wrapped = new Map<string, TreeNode<T>>(
    nodes.map((n) => [n.id ?? '', { ...n, children: [] } as TreeNode<T>]),
  );
  const roots: TreeNode<T>[] = [];
  const attached = new Set<string>();

  for (const node of wrapped.values()) {
    const id = node.id ?? '';
    const parentId = node.parentId ?? rootId;
    const parent = parentId === rootId ? undefined : wrapped.get(parentId);
    if (!parent || parent === node) {
      roots.push(node);
      continue;
    }
    // Cycle: the parent is inside its own subtree; attaching would loop forever, so fall back to the root.
    if (isDescendant(parent, id, wrapped, rootId)) {
      roots.push(node);
      continue;
    }
    parent.children.push(node);
    attached.add(id);
  }
  return roots.filter((n) => !attached.has(n.id ?? ''));
}

function isDescendant<T extends FlatNode>(
  node: TreeNode<T>,
  ancestorId: string,
  all: Map<string, TreeNode<T>>,
  rootId: string,
): boolean {
  let cur: TreeNode<T> | undefined = node;
  const seen = new Set<string>();
  while (cur) {
    const id = cur.id ?? '';
    if (id === ancestorId) return true;
    if (seen.has(id)) return false;
    seen.add(id);
    const parent: string = cur.parentId ?? rootId;
    cur = parent === rootId ? undefined : all.get(parent);
  }
  return false;
}
