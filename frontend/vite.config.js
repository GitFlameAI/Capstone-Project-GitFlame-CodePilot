import { defineConfig } from 'vite'
import vue from '@vitejs/plugin-vue'
import { fileURLToPath, URL } from 'node:url'

// The frontend can run in two modes:
//   1. Mock mode (default): no backend needed. The app serves data from an
//      in-memory mock so it is fully demoable for screenshots and GIFs.
//   2. Real mode: set VITE_API_BASE in a .env file (e.g. VITE_API_BASE=/api)
//      and the app will call the real Go backend instead of the mock.
//
// The proxy below lets you use a relative VITE_API_BASE (like "/api") during
// development and forwards those requests to the Go backend on :8000,
// which avoids CORS configuration entirely.
export default defineConfig({
  plugins: [vue()],
  resolve: {
    alias: {
      '@': fileURLToPath(new URL('./src', import.meta.url)),
    },
  },
  server: {
    port: 5173,
    proxy: {
      '/api': {
        target: 'http://localhost:8000',
        changeOrigin: true,
        rewrite: (path) => path.replace(/^\/api/, ''),
      },
    },
  },
})
