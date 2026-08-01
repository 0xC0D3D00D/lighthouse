<script setup lang="ts">
interface RecordView {
  name: string
  qtype: number
  ttl: number
  queried_at: string
  rcode: number
  answer_count: number
  answers: string[] | null
  expired: boolean
}

const QTYPES: Record<number, string> = {
  1: 'A', 2: 'NS', 5: 'CNAME', 6: 'SOA', 12: 'PTR', 15: 'MX',
  16: 'TXT', 28: 'AAAA', 33: 'SRV', 43: 'DS', 46: 'RRSIG',
  48: 'DNSKEY', 64: 'SVCB', 65: 'HTTPS', 257: 'CAA',
}
const qtypeName = (n: number) => QTYPES[n] ?? String(n)

const search = ref('')
const qtype = ref('any')
const page = ref(1)
const pageSize = 25
const busy = ref<string | null>(null)
const toast = useToast()

const qtypeOptions = ['any', ...Object.values(QTYPES)]

const query = computed(() => ({
  search: search.value,
  qtype: qtype.value === 'any' ? '' : qtype.value,
  offset: (page.value - 1) * pageSize,
  limit: pageSize,
}))

const { data, refresh, status } = await useFetch<{ records: RecordView[], total: number }>(
  '/api/records',
  { query, watch: [query] },
)

watch([search, qtype], () => { page.value = 1 })

async function del(rec: RecordView) {
  busy.value = `${rec.name}|${rec.qtype}`
  try {
    await $fetch('/api/records', {
      method: 'DELETE',
      query: { name: rec.name, qtype: qtypeName(rec.qtype) },
    })
    toast.add({ title: `Deleted ${rec.name} ${qtypeName(rec.qtype)}`, color: 'success' })
    await refresh()
  } catch (e: any) {
    toast.add({ title: 'Delete failed', description: String(e?.data?.error ?? e), color: 'error' })
  } finally {
    busy.value = null
  }
}

async function requery(rec: RecordView) {
  busy.value = `${rec.name}|${rec.qtype}`
  try {
    await $fetch('/api/records/requery', {
      method: 'POST',
      query: { name: rec.name, qtype: qtypeName(rec.qtype) },
    })
    toast.add({ title: `Re-queried ${rec.name} ${qtypeName(rec.qtype)}`, color: 'success' })
    await refresh()
  } catch (e: any) {
    toast.add({ title: 'Re-query failed', description: String(e?.data?.error ?? e), color: 'error' })
  } finally {
    busy.value = null
  }
}

const totalPages = computed(() => Math.max(1, Math.ceil((data.value?.total ?? 0) / pageSize)))
</script>

<template>
  <div class="space-y-4">
    <div class="flex flex-wrap items-center gap-3">
      <UInput
        v-model="search"
        icon="i-lucide-search"
        placeholder="Search by name prefix…"
        class="w-72"
      />
      <USelect v-model="qtype" :items="qtypeOptions" class="w-32" />
      <UButton icon="i-lucide-refresh-cw" variant="ghost" :loading="status === 'pending'" @click="() => refresh()">
        Refresh
      </UButton>
      <div class="ms-auto text-sm text-muted">{{ data?.total ?? 0 }} records</div>
    </div>

    <UCard :ui="{ body: 'p-0 sm:p-0' }">
      <table class="w-full text-sm">
        <thead>
          <tr class="border-b border-default text-left text-muted">
            <th class="px-4 py-2 font-medium">Name</th>
            <th class="px-4 py-2 font-medium">Type</th>
            <th class="px-4 py-2 font-medium">Answers</th>
            <th class="px-4 py-2 font-medium">TTL</th>
            <th class="px-4 py-2 font-medium">Queried at</th>
            <th class="px-4 py-2 font-medium text-right">Actions</th>
          </tr>
        </thead>
        <tbody>
          <tr
            v-for="rec in data?.records ?? []"
            :key="`${rec.name}|${rec.qtype}`"
            class="border-b border-default last:border-0 align-top"
          >
            <td class="px-4 py-2 font-mono">{{ rec.name }}</td>
            <td class="px-4 py-2"><UBadge variant="subtle">{{ qtypeName(rec.qtype) }}</UBadge></td>
            <td class="px-4 py-2 font-mono text-xs whitespace-pre-line max-w-md truncate">
              {{ rec.answers?.length ? rec.answers.join('\n') : '—' }}
            </td>
            <td class="px-4 py-2">
              <span :class="rec.expired ? 'text-error' : ''">{{ rec.ttl }}s</span>
              <UBadge v-if="rec.expired" color="warning" variant="subtle" class="ms-1">expired</UBadge>
            </td>
            <td class="px-4 py-2 text-muted">{{ new Date(rec.queried_at).toLocaleString() }}</td>
            <td class="px-4 py-2 text-right whitespace-nowrap">
              <UButton
                icon="i-lucide-rotate-cw"
                size="xs"
                variant="ghost"
                :loading="busy === `${rec.name}|${rec.qtype}`"
                @click="requery(rec)"
              />
              <UButton
                icon="i-lucide-trash-2"
                size="xs"
                color="error"
                variant="ghost"
                :disabled="busy !== null"
                @click="del(rec)"
              />
            </td>
          </tr>
          <tr v-if="!data?.records?.length">
            <td colspan="6" class="px-4 py-8 text-center text-muted">No records found.</td>
          </tr>
        </tbody>
      </table>
    </UCard>

    <div class="flex items-center justify-center gap-2" v-if="totalPages > 1">
      <UButton icon="i-lucide-chevron-left" variant="ghost" :disabled="page <= 1" @click="page--" />
      <span class="text-sm text-muted">page {{ page }} / {{ totalPages }}</span>
      <UButton icon="i-lucide-chevron-right" variant="ghost" :disabled="page >= totalPages" @click="page++" />
    </div>
  </div>
</template>
