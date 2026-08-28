import react from '@vitejs/plugin-react'
import { defineConfig, loadEnv } from 'vite'

// https://vite.dev/config/
export default defineConfig(({ mode }) => {
  // client/.env (see client/.env.example) lets the dev server's own port
  // and the /api proxy target be overridden without editing this file —
  // useful when DBTOOL_LISTEN_ADDR changes the Go server's port.
  const env = loadEnv(mode, process.cwd(), '')
  const port = Number(env.VITE_DEV_PORT) || 5173
  const apiProxyTarget = env.VITE_API_PROXY_TARGET || 'http://localhost:8080'

  return {
    plugins: [react()],
    server: {
      port,
      proxy: {
        '/api': apiProxyTarget,
      },
    },
  }
})
