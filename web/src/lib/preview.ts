/**
 * Frontend state of permission preview (root only): localStorage holds the selected roles, and the request middleware
 * injects X-Preview-Roles from it; the backend honors it only for root (it only reduces, never raises). Names are only
 * for the button label. Entering and exiting both reload the page fully, so the profile is fetched again with the new
 * identity and the menu and permission checks follow.
 */
const KEY = 'keel:preview';

export type PreviewState = {
  /** CSV of role IDs, used directly as the value of the X-Preview-Roles header */
  ids: string;
  /** Role names, only for the button and dropdown labels */
  names: string[];
};

export function getPreview(): PreviewState | undefined {
  try {
    const raw = localStorage.getItem(KEY);
    if (!raw) return undefined;
    const v = JSON.parse(raw) as Partial<PreviewState> | null;
    return v?.ids ? { ids: v.ids, names: v.names ?? [] } : undefined;
  } catch {
    return undefined;
  }
}

export function setPreview(state: PreviewState): void {
  localStorage.setItem(KEY, JSON.stringify(state));
}

export function clearPreview(): void {
  localStorage.removeItem(KEY);
}
