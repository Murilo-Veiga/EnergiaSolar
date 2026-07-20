import { defineConfig } from 'vite'
import react from '@vitejs/plugin-react'

// https://vite.dev/config/
export default defineConfig({
  plugins: [react()],
  server: {
    // Mesma lógica do nginx em produção (ver nginx.conf): o frontend
    // sempre chama /api relativo à própria origem, nunca um host fixado
    // (ver src/lib/api.ts). Aqui o Vite faz esse proxy pro servidor
    // api-go local — porta configurável via VITE_DEV_API_TARGET (usado
    // por scripts/dev.sh quando DEV_API_PORT é sobrescrito).
    proxy: {
      '/api': {
        target: process.env.VITE_DEV_API_TARGET ?? 'http://localhost:8092',
        changeOrigin: true,
      },
    },
  },
})
