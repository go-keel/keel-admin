/**
 * Pure logic of a role's feature-permission settings: dependency closure, grouped catalogue, search.
 *
 * The server expands `deps` when it grants (perm.Effective), so the settings UI must expand and lock them too;
 * otherwise "what is ticked" and "what actually takes effect" diverge, and the person configuring cannot see it.
 */
export type PermDef = {
  code?: string;
  deps?: string[];
};

const depsOf = (defs: readonly PermDef[]): Map<string, string[]> =>
  new Map(defs.map((d) => [d.code ?? '', d.deps ?? []]));

const closureIn = (map: Map<string, string[]>, code: string): Set<string> => {
  const seen = new Set<string>();
  const walk = (c: string) => {
    for (const dep of map.get(c) ?? []) {
      if (seen.has(dep)) continue;
      seen.add(dep);
      walk(dep);
    }
  };
  walk(code);
  return seen;
};

/** The prerequisite closure of a code (excluding itself): ticking it, these are what the server actually grants too. */
export const depsClosure = (
  code: string,
  defs: readonly PermDef[],
): string[] => [...closureIn(depsOf(defs), code)];

/** Tick a code: its deps closure is ticked along with it. */
export function selectWithDeps(
  selected: readonly string[],
  code: string,
  defs: readonly PermDef[],
): string[] {
  const map = depsOf(defs);
  const out = new Set(selected);
  const walk = (c: string) => {
    if (out.has(c)) return;
    out.add(c);
    for (const dep of map.get(c) ?? []) walk(dep);
  };
  walk(code);
  return [...out];
}

/**
 * Untick a code: possible only when nothing depends on it. Returns the set after unticking; when it cannot be unticked
 * the set is returned unchanged (the UI then renders it as locked instead of letting the click silently do nothing).
 */
export function deselect(
  selected: readonly string[],
  code: string,
  defs: readonly PermDef[],
): string[] {
  if (dependentsOf(code, selected, defs).length > 0) return [...selected];
  return selected.filter((c) => c !== code);
}

/** Among the ticked codes, those that depend on code directly or indirectly. Non-empty = code is locked. */
export function dependentsOf(
  code: string,
  selected: readonly string[],
  defs: readonly PermDef[],
): string[] {
  const map = depsOf(defs);
  return selected.filter((s) => s !== code && closureIn(map, s).has(code));
}

/**
 * Tick or untick a group of codes together (a group's select-all box). After unticking together, whatever codes still
 * ticked outside the group depend on is added back: the server expands them when granting anyway, and the UI must not
 * show them as absent.
 */
export function toggleMany(
  selected: readonly string[],
  codes: readonly string[],
  checked: boolean,
  defs: readonly PermDef[],
): string[] {
  const base = checked
    ? [...selected, ...codes]
    : selected.filter((c) => !codes.includes(c));
  return base.reduce<string[]>((acc, c) => selectWithDeps(acc, c, defs), []);
}

/** All locks computed at once: code → ticked codes that depend on it. Rendering looks rows up instead of recomputing. */
export function lockedMap(
  selected: readonly string[],
  defs: readonly PermDef[],
): Map<string, string[]> {
  const out = new Map<string, string[]>();
  for (const def of defs) {
    const code = def.code ?? '';
    if (!selected.includes(code)) continue;
    const by = dependentsOf(code, selected, defs);
    if (by.length > 0) out.set(code, by);
  }
  return out;
}

export type PermCatalog<T> = {
  domain: string;
  name: string;
  modules: { module: string; name: string; defs: T[] }[];
}[];

/** Tiers of feature operations: view → create/update → other → delete. */
const actionTier = (code: string): number => {
  const action = code.split(':')[2] ?? '';
  if (action === 'view') return 0;
  if (action === 'create' || action === 'update') return 1;
  if (action === 'delete') return 3;
  return 2;
};

/**
 * Business domain → feature module → code. **Groups are ordered as in groups** (the server has already sorted them by
 * each group's Sort); codes are sent in alphabetical order, and ordering groups by that would put departments after
 * audit. Groups not declared in groups go last.
 *
 * **Codes within a module follow the framework's display order**: view first, create and update adjacent, other
 * operations in the middle, delete last; within a tier, a smaller prerequisite closure comes first, then by code. deps
 * is a DAG and cannot be drawn as parent/child nesting, so feature operations can only be flat; this order makes
 * "prerequisites first" visible at a glance, while the real grant relations are still expressed by auto-ticking and
 * locking.
 */
export function catalogOf<
  T extends PermDef & { domain?: string; module?: string },
>(
  defs: readonly T[],
  groups: readonly { domain?: string; module?: string; name?: string }[],
): PermCatalog<T> {
  const indexOf = (domain: string, module: string) =>
    groups.findIndex((g) => g.domain === domain && (g.module ?? '') === module);
  const rank = (domain: string, module: string) => {
    const i = indexOf(domain, module);
    return i < 0 ? groups.length : i;
  };
  const nameOf = (domain: string, module: string) =>
    groups[indexOf(domain, module)]?.name || module || domain;

  const out: PermCatalog<T> = [];
  for (const d of defs) {
    const domain = d.domain ?? '';
    const module = d.module ?? '';
    let dom = out.find((x) => x.domain === domain);
    if (!dom) {
      dom = { domain, name: nameOf(domain, ''), modules: [] };
      out.push(dom);
    }
    let mod = dom.modules.find((x) => x.module === module);
    if (!mod) {
      mod = { module, name: nameOf(domain, module), defs: [] };
      dom.modules.push(mod);
    }
    mod.defs.push(d);
  }
  out.sort((a, b) => rank(a.domain, '') - rank(b.domain, ''));
  const map = depsOf(defs);
  const closureSize = (code: string) => closureIn(map, code).size;
  for (const dom of out) {
    dom.modules.sort(
      (a, b) => rank(dom.domain, a.module) - rank(dom.domain, b.module),
    );
    for (const mod of dom.modules) {
      mod.defs.sort((a, b) => {
        const ac = a.code ?? '';
        const bc = b.code ?? '';
        return (
          actionTier(ac) - actionTier(bc) ||
          closureSize(ac) - closureSize(bc) ||
          ac.localeCompare(bc)
        );
      });
    }
  }
  return out;
}

/** Search by name or by code: with many codes, scrolling will not find one. */
export const matchesQuery = (
  def: { code?: string; name?: string },
  query: string,
): boolean => {
  const q = query.trim().toLowerCase();
  if (q === '') return true;
  return (
    (def.code ?? '').toLowerCase().includes(q) ||
    (def.name ?? '').toLowerCase().includes(q)
  );
};
