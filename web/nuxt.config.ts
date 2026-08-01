export default defineNuxtConfig({
  compatibilityDate: '2026-08-01',
  ssr: false,
  modules: ['@nuxt/ui'],
  css: ['~/assets/css/main.css'],
  app: {
    head: {
      title: 'Lighthouse DNS',
    },
  },
  devtools: { enabled: false },
  nitro: {
    devProxy: {
      '/api': { target: 'http://localhost:8080/api', changeOrigin: true },
    },
  },
})
