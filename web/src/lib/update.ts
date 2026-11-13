/**
 * Frontend update detection (no server changes):
 * 1) On window refocus and every 5 minutes, fetch /version.json (no-store); when buildId differs from the
 *    build-injected __BUILD_ID__ → show "a new version is available" with a click to reload; also set updatePending,
 *    so reloadIfUpdatePending switches to the new version unnoticed on the next page change.
 * 2) When the hashed chunks the old index.html references are gone with the new release, dynamic imports fail; the
 *    chunk load error is caught and the whole page reloads once (sessionStorage guards against loops).
 */
const CHECK_INTERVAL_MS = 5 * 60 * 1000;
const RELOAD_GUARD_KEY = 'keel_chunk_reloaded';

let updatePending = false;
let lastPathname: string | undefined;

/**
 * Called on route changes: once a new version is detected, the next page change (a pathname change) becomes a full
 * page load; the URL already points to the target route, so reload opens it with the new assets. The user is leaving
 * the current page anyway, so it feels like an ordinary navigation, no in-page state is lost, and it no longer runs
 * into load failures from old chunks being gone. Changes of only query/hash (in-page tabs, filters and so on) do not
 * trigger it, so in-page work is not interrupted.
 */
export function reloadIfUpdatePending(pathname: string): void {
  const prev = lastPathname;
  lastPathname = pathname;
  if (!updatePending || prev === undefined || prev === pathname) return;
  window.location.reload();
}

/**
 * Whether a dynamic import failed because an old chunk is gone. Both the name and the message are checked; browsers
 * differ.
 */
export function isChunkLoadError(reason: unknown): boolean {
  const name = (reason as { name?: string })?.name ?? '';
  const msg = String((reason as { message?: string })?.message ?? '');
  return (
    name === 'ChunkLoadError' ||
    /Loading( CSS)? chunk .* failed|Failed to fetch dynamically imported module|error loading dynamically imported module/i.test(
      msg,
    )
  );
}

/**
 * Installs two listeners and returns the uninstall function. onUpdate is called only once: a prompt that keeps popping
 * up is pointless. No detection in dev: __BUILD_ID__ is always 'dev', and version.json does not exist.
 */
export function installUpdateWatch(onUpdate: () => void): () => void {
  const onRejection = (e: PromiseRejectionEvent) => {
    if (!isChunkLoadError(e.reason)) return;
    if (sessionStorage.getItem(RELOAD_GUARD_KEY)) return;
    sessionStorage.setItem(RELOAD_GUARD_KEY, '1');
    window.location.reload();
  };
  // A successful load clears the loop guard
  sessionStorage.removeItem(RELOAD_GUARD_KEY);
  window.addEventListener('unhandledrejection', onRejection);

  if (__BUILD_ID__ === 'dev') {
    return () => window.removeEventListener('unhandledrejection', onRejection);
  }

  let notified = false;
  const check = async () => {
    if (notified || document.hidden) return;
    try {
      const resp = await fetch(`/version.json?t=${Date.now()}`, {
        cache: 'no-store',
      });
      if (!resp.ok) return;
      const { buildId } = (await resp.json()) as { buildId?: string };
      if (!buildId || buildId === __BUILD_ID__) return;
      updatePending = true;
      notified = true;
      onUpdate();
    } catch {
      // Ignore network blips; check again next round
    }
  };
  const onVisible = () => {
    if (!document.hidden) void check();
  };
  const timer = window.setInterval(check, CHECK_INTERVAL_MS);
  document.addEventListener('visibilitychange', onVisible);
  void check();

  return () => {
    window.clearInterval(timer);
    document.removeEventListener('visibilitychange', onVisible);
    window.removeEventListener('unhandledrejection', onRejection);
  };
}
