/**
 * The single endpoint for the brand logo (logoEntry in the server's internal/server/spa.go). Its three states (embedded
 * / disk / external URL) and both forms, dev and embedded deployment, share this one address: the server serves the
 * content directly (a 302 for an external URL) with the Content-Type of the real file, so the frontend need not know
 * where the logo lives or whether it is svg or png. It goes through /v1 to stay apart from frontend routes, see
 * docs/adr/admin-0012-raw-handlers-under-v1.md.
 */
export const BRAND_LOGO = '/v1/sys/brand/logo';

const FALLBACK_TITLE = 'Keel Admin';

let title = window.__BRAND__?.title || FALLBACK_TITLE;

export const brandTitle = (): string => title;

/**
 * Single source for the deployment brand (sys.brand in app.yaml). The title comes in two forms:
 * - embedded deployment: spa.go has injected the title into window.__BRAND__, so no request is needed;
 * - dev (vite dev server + `go run` backend): fetched from the backend through the vite /v1 proxy at /v1/sys/brand (the
 *   brand needs no proxy of its own).
 * The entry point awaits this once before rendering; afterwards everything reads it synchronously.
 */
export async function resolveBrand(): Promise<void> {
  // Embedded mode (including production deployments without a brand) sends no request: with a brand, __BRAND__ is
  // already injected and spa.go has written the favicon into index.html; without one, the build default is the right
  // one.
  if (window.__BRAND__ || __BUILD_ID__ !== 'dev') return;
  try {
    const resp = await fetch('/v1/sys/brand');
    if (!resp.ok) return;
    const b = (await resp.json()) as { title?: string };
    if (b.title) title = b.title;
    document.title = title;
  } catch {
    // Backend not running (frontend-only styling work) → build default
  } finally {
    // The dev index.html has no server-injected icon link, so add it here; it uses the same endpoint as the in-page
    // logo, which in dev reaches the backend through the /v1 proxy (the embedded fallback is not bound by embed_ui, see
    // web/logo.go).
    setFavicon(BRAND_LOGO);
  }
}

// Set the favicon in dev; reuse an existing link so hot reloads do not append duplicates.
function setFavicon(href: string): void {
  const link =
    document.querySelector<HTMLLinkElement>('link[rel~="icon"]') ??
    document.createElement('link');
  link.rel = 'icon';
  link.href = href;
  if (!link.isConnected) document.head.append(link);
}
