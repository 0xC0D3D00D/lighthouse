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
  manual: boolean
  source: string
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

const addOpen = ref(false)
const addBusy = ref(false)
const editing = ref(false)
const addForm = reactive({ name: '', qtype: 'A', ttl: 300, values: '' })

function openAdd() {
  editing.value = false
  addForm.name = ''
  addForm.qtype = 'A'
  addForm.ttl = 300
  addForm.values = ''
  addOpen.value = true
}

// rdataOf extracts the rdata from a zone-file RR string
// ("name ttl class type rdata..."), or null when the type doesn't match.
function rdataOf(rr: string, type: string): string | null {
  const parts = rr.trim().split(/\s+/)
  if (parts.length < 5 || parts[3] !== type) return null
  return parts.slice(4).join(' ')
}

function openEdit(rec: RecordView) {
  editing.value = true
  addForm.name = rec.name
  addForm.qtype = qtypeName(rec.qtype)
  addForm.ttl = rec.ttl || 300
  addForm.values = (rec.answers ?? [])
    .map(rr => rdataOf(rr, qtypeName(rec.qtype)))
    .filter((v): v is string => v !== null)
    .join('\n')
  addOpen.value = true
}

async function addRecord() {
  const values = addForm.values.split('\n').map(v => v.trim()).filter(Boolean)
  if (!addForm.name.trim() || !values.length) {
    toast.add({ title: 'Name and at least one value are required', color: 'error' })
    return
  }
  addBusy.value = true
  try {
    await $fetch('/api/records', {
      method: 'POST',
      body: {
        name: addForm.name.trim(),
        qtype: addForm.qtype,
        ttl: Number(addForm.ttl) || 300,
        values,
      },
    })
    toast.add({ title: `${editing.value ? 'Saved' : 'Added'} ${addForm.name.trim()} ${addForm.qtype}`, description: 'Manual record pinned as source of truth', color: 'success' })
    addOpen.value = false
    addForm.name = ''
    addForm.values = ''
    await refresh()
  } catch (e: any) {
    toast.add({ title: 'Add failed', description: String(e?.data?.error ?? e), color: 'error' })
  } finally {
    addBusy.value = false
  }
}
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
      <UButton icon="i-lucide-plus" @click="openAdd">
        Add record
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
            <th class="px-4 py-2 font-medium">Source</th>
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
            <td class="px-4 py-2">
              <UBadge variant="subtle">{{ qtypeName(rec.qtype) }}</UBadge>
            </td>
            <td class="px-4 py-2 font-mono text-xs whitespace-pre-line max-w-md truncate">
              {{ rec.answers?.length ? rec.answers.join('\n') : '—' }}
            </td>
            <td class="px-4 py-2">{{ rec.ttl }}s</td>
            <td class="px-4 py-2 font-mono text-xs text-muted">{{ rec.source || '—' }}</td>
            <td class="px-4 py-2 text-muted">{{ new Date(rec.queried_at).toLocaleString() }}</td>
            <td class="px-4 py-2 text-right whitespace-nowrap">
              <UButton
                icon="i-lucide-pencil"
                size="xs"
                variant="ghost"
                :disabled="busy !== null"
                @click="openEdit(rec)"
              />
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
            <td colspan="7" class="px-4 py-8 text-center text-muted">No records found.</td>
          </tr>
        </tbody>
      </table>
    </UCard>

    <div class="flex items-center justify-center gap-2" v-if="totalPages > 1">
      <UButton icon="i-lucide-chevron-left" variant="ghost" :disabled="page <= 1" @click="page--" />
      <span class="text-sm text-muted">page {{ page }} / {{ totalPages }}</span>
      <UButton icon="i-lucide-chevron-right" variant="ghost" :disabled="page >= totalPages" @click="page++" />
    </div>

    <UModal v-model:open="addOpen" :title="editing ? 'Edit record (overwrite as manual)' : 'Add manual record'">
      <template #body>
        <form class="space-y-4" @submit.prevent="addRecord">
          <UFormField label="Name" help="Fully qualified name; a trailing dot is added if missing.">
            <UInput v-model="addForm.name" placeholder="app.internal.example.com" class="w-full" :disabled="editing" :autofocus="!editing" />
          </UFormField>
          <div class="flex gap-3">
            <UFormField label="Type" class="w-32">
              <USelect v-model="addForm.qtype" :items="Object.values(QTYPES)" :disabled="editing" />
            </UFormField>
            <UFormField label="TTL (seconds)" class="flex-1">
              <UInput v-model.number="addForm.ttl" type="number" min="1" />
            </UFormField>
          </div>
          <UFormField label="Values" help="One value per line, in zone-file rdata syntax (e.g. 10.0.0.5, or “10 mail.example.com.” for MX).">
            <UTextarea v-model="addForm.values" :rows="3" placeholder="10.0.0.5" class="w-full font-mono" />
          </UFormField>
          <p class="text-xs text-muted">
            Manual records are pinned as the source of truth: they overwrite the stored record for this
            name/type, are served without querying backends, and backend answers can no longer replace them.
          </p>
        </form>
      </template>
      <template #footer>
        <div class="flex justify-end gap-2 w-full">
          <UButton variant="ghost" color="neutral" :disabled="addBusy" @click="addOpen = false">Cancel</UButton>
          <UButton :loading="addBusy" @click="addRecord">{{ editing ? 'Save as manual' : 'Add record' }}</UButton>
        </div>
      </template>
    </UModal>
  </div>
</template>
