<script setup lang="ts">
interface BackendStatus {
  name: string
  reachable: boolean
  checked_at: string
}

interface StatusResponse {
  health: {
    mode: 'normal' | 'survival'
    backends: BackendStatus[]
    last_beacon_ok: string
    mode_since: string
  }
  backends: string[]
  cache_entries: number
  recordbook_records: number
}

interface StatsResponse {
  buckets: { unix: number, queries: number, errors: number, cache_hits: number, backend_errors: number }[]
  total_queries: number
  total_errors: number
  total_cache_hits: number
}

const { data: status, refresh: refreshStatus } = await useFetch<StatusResponse>('/api/status')
const { data: stats, refresh: refreshStats } = await useFetch<StatsResponse>('/api/stats?seconds=300')

let timer: ReturnType<typeof setInterval> | undefined
onMounted(() => {
  timer = setInterval(() => {
    refreshStatus()
    refreshStats()
  }, 2000)
})
onUnmounted(() => clearInterval(timer))

const qps = computed(() => {
  const buckets = stats.value?.buckets ?? []
  // Average over the last 10 completed seconds.
  const recent = buckets.slice(-11, -1)
  if (!recent.length) return 0
  return (recent.reduce((s, b) => s + b.queries, 0) / recent.length).toFixed(1)
})

const modeColor = computed(() => status.value?.health.mode === 'survival' ? 'error' : 'success')
</script>

<template>
  <div class="space-y-6">
    <div class="grid grid-cols-2 lg:grid-cols-5 gap-4">
      <UCard>
        <div class="text-sm text-muted">Mode</div>
        <UBadge :color="modeColor" variant="subtle" size="lg" class="mt-1 uppercase">
          {{ status?.health.mode ?? '…' }}
        </UBadge>
      </UCard>
      <UCard>
        <div class="text-sm text-muted">QPS (10s avg)</div>
        <div class="text-2xl font-semibold">{{ qps }}</div>
      </UCard>
      <UCard>
        <div class="text-sm text-muted">Total queries</div>
        <div class="text-2xl font-semibold">{{ stats?.total_queries ?? 0 }}</div>
      </UCard>
      <UCard>
        <div class="text-sm text-muted">Cache entries</div>
        <div class="text-2xl font-semibold">{{ status?.cache_entries ?? 0 }}</div>
      </UCard>
      <UCard>
        <div class="text-sm text-muted">Record book</div>
        <div class="text-2xl font-semibold">{{ status?.recordbook_records ?? 0 }}</div>
      </UCard>
    </div>

    <UCard>
      <template #header>
        <div class="font-medium">Traffic (last 5 minutes)</div>
      </template>
      <ClientOnly>
        <QpsChart :buckets="stats?.buckets ?? []" />
      </ClientOnly>
    </UCard>

    <UCard>
      <template #header>
        <div class="flex items-center justify-between">
          <div class="font-medium">Main servers (beacon reachability)</div>
          <div class="text-sm text-muted" v-if="status">
            last beacon OK: {{ new Date(status.health.last_beacon_ok).toLocaleString() }}
          </div>
        </div>
      </template>
      <ul class="divide-y divide-default">
        <li
          v-for="up in status?.health.backends ?? []"
          :key="up.name"
          class="py-2 flex items-center justify-between"
        >
          <span class="font-mono text-sm">{{ up.name }}</span>
          <div class="flex items-center gap-3">
            <span class="text-xs text-muted">{{ new Date(up.checked_at).toLocaleTimeString() }}</span>
            <UBadge :color="up.reachable ? 'success' : 'error'" variant="subtle">
              {{ up.reachable ? 'reachable' : 'unreachable' }}
            </UBadge>
          </div>
        </li>
        <li v-if="!status?.health.backends?.length" class="py-2 text-sm text-muted">
          No probe results yet.
        </li>
      </ul>
    </UCard>
  </div>
</template>
