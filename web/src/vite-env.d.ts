/// <reference types="vite/client" />

declare global {
  /** One-off build identifier injected by vite define; always 'dev' in dev (see vite.config.ts). */
  const __BUILD_ID__: string;

  interface Window {
    /** The deployment brand injected by spa.go in embedded deployment (see internal/server/spa.go). */
    __BRAND__?: { title?: string };
  }
}

export {};
