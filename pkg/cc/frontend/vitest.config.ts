import { defineConfig } from 'vitest/config';
import { storybookTest } from '@storybook/addon-vitest/vitest-plugin';
import react from '@vitejs/plugin-react';
import path from 'path';

/**
 * Vitest config for Storybook tests.
 *
 * The storybookTest plugin MUST be at the root plugins level, and when used,
 * it affects the entire config. Therefore, we need separate configs for unit
 * and storybook tests in Vitest v4.
 *
 * Run via: pnpm test:storybook
 * For unit tests, see vitest.config.unit.ts
 */
export default defineConfig({
  plugins: [
    react(),
    storybookTest({
      configDir: path.join(__dirname, '.storybook'),
      tags: {
        exclude: ['test-exclude'],
      },
    }),
  ],
  test: {
    name: 'storybook',
    globals: true,
    environment: 'jsdom',
    browser: {
      enabled: true,
      headless: true,
    },
    setupFiles: ['./.storybook/vitest-setup.ts'],
    fakeTimers: {
      toFake: ['setTimeout', 'clearTimeout', 'setInterval', 'clearInterval', 'setImmediate', 'clearImmediate', 'Date'],
    },
  },
  resolve: {
    alias: {
      '@': path.resolve(__dirname, './src'),
      '@components': path.resolve(__dirname, './src/components'),
      '@hooks': path.resolve(__dirname, './src/hooks'),
      '@utils': path.resolve(__dirname, './src/utils'),
    },
  },
});
