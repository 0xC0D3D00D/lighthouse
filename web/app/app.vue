<script setup lang="ts">
const route = useRoute()

const links = computed(() => [
  { label: 'Status', icon: 'i-lucide-activity', to: '/', active: route.path === '/' },
  { label: 'Record Book', icon: 'i-lucide-book-open', to: '/records', active: route.path === '/records' },
])
</script>

<template>
  <UApp>
    <div class="min-h-screen bg-default">
      <header class="border-b border-default">
        <div class="max-w-6xl mx-auto px-4 py-3 flex items-center gap-6">
          <div class="flex items-center gap-2 font-semibold text-lg">
            <UIcon name="i-lucide-radio-tower" class="text-primary size-6" />
            Lighthouse
          </div>
          <UNavigationMenu :items="links" />
        </div>
      </header>
      <main class="max-w-6xl mx-auto px-4 py-6">
        <NuxtPage />
      </main>
    </div>
  </UApp>
</template>
