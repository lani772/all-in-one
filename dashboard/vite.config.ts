import { fileURLToPath } from 'node:url';
import { defineConfig } from 'vite';
import react from '@vitejs/plugin-react';

const appDeployCITestStub = fileURLToPath(new URL('./tests/appdeploy-client.stub.ts', import.meta.url));

export default defineConfig({
  resolve: process.env.APPDEPLOY_CI_STUB === 'true' ? { alias: { '@appdeploy/client': appDeployCITestStub } } : undefined,
  plugins: [react()],
  base: './',
  build: {
    outDir: process.env.APPDEPLOY_VITE_OUT_DIR || 'dist',
    sourcemap:
      process.env.APPDEPLOY_VITE_SOURCEMAP === 'hidden' ? 'hidden' : false,
    rollupOptions: {
      maxParallelFileOps: 128,
    },
  },
});
