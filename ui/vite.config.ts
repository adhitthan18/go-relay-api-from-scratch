import react from '@vitejs/plugin-react'
import { defineConfig } from 'vite'

// Vite jalan di 5173, API Go di 8080. Dari sisi browser, port yang berbeda
// berarti origin yang berbeda — dan cookie session tidak ikut terkirim ke
// origin lain. Proxy ini membuat keduanya tampak satu origin.
const jalurAPI = ['/login', '/logout', '/me', '/devices', '/api', '/health', '/version']

export default defineConfig({
  plugins: [react()],
  server: {
    proxy: Object.fromEntries(
      jalurAPI.map((p) => [p, { target: 'http://localhost:8080' }]),
    ),
  },
})