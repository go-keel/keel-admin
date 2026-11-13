import { writeFileSync } from 'node:fs';
import { fileURLToPath } from 'node:url';
import tailwindcss from '@tailwindcss/vite';
import { tanstackRouter } from '@tanstack/router-plugin/vite';
import react from '@vitejs/plugin-react';
import { defineConfig } from 'vite';

/**
 * Build ID: generated once per build, injected as __BUILD_ID__ and written to dist/version.json. At runtime the app
 * polls version.json and compares it with its own __BUILD_ID__ to learn that the server now runs a new version (see
 * src/lib/update.ts). In dev it is always 'dev' and detection is off.
 */
const buildId = `${Date.now().toString(36)}-${Math.random().toString(36).slice(2, 8)}`;

export default defineConfig(({ command }) => ({
  plugins: [
    // File routes: scan src/routes/ to generate src/routeTree.gen.ts (a committed generated file)
    tanstackRouter({ target: 'react', autoCodeSplitting: true }),
    react(),
    tailwindcss(),
    {
      name: 'keel-version-json',
      apply: 'build',
      writeBundle() {
        writeFileSync('dist/version.json', `${JSON.stringify({ buildId })}\n`);
      },
    },
  ],
  resolve: {
    alias: { '@': fileURLToPath(new URL('./src', import.meta.url)) },
  },
  define: {
    __BUILD_ID__: JSON.stringify(command === 'build' ? buildId : 'dev'),
  },
  server: {
    port: 8000,
    // Backend on 8081: matches the addr override in configs/config.dev.yaml (8080 is often taken locally). The two
    // brand paths are under /v1 too (see internal/server/spa.go), so this entry covers them.
    proxy: { '/v1': { target: 'http://localhost:8081', changeOrigin: true } },
  },
  test: {
    environment: 'jsdom',
    include: ['src/**/*.test.ts'],
  },
}));
