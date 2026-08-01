<script setup lang="ts">
import { use } from 'echarts/core'
import { CanvasRenderer } from 'echarts/renderers'
import { LineChart } from 'echarts/charts'
import { GridComponent, TooltipComponent, LegendComponent } from 'echarts/components'
import VChart from 'vue-echarts'

use([CanvasRenderer, LineChart, GridComponent, TooltipComponent, LegendComponent])

interface Bucket {
  unix: number
  queries: number
  errors: number
  cache_hits: number
  backend_errors: number
}

const props = defineProps<{ buckets: Bucket[] }>()

const option = computed(() => ({
  animation: false,
  tooltip: { trigger: 'axis' },
  legend: { data: ['Queries', 'Cache hits', 'Errors'], bottom: 0 },
  grid: { left: 40, right: 16, top: 16, bottom: 40 },
  xAxis: {
    type: 'category',
    data: props.buckets.map(b => new Date(b.unix * 1000).toLocaleTimeString()),
  },
  yAxis: { type: 'value', minInterval: 1 },
  series: [
    { name: 'Queries', type: 'line', showSymbol: false, data: props.buckets.map(b => b.queries), color: '#00bc7d' },
    { name: 'Cache hits', type: 'line', showSymbol: false, data: props.buckets.map(b => b.cache_hits), color: '#615fff' },
    { name: 'Errors', type: 'line', showSymbol: false, data: props.buckets.map(b => b.errors), color: '#fb2c36' },
  ],
}))
</script>

<template>
  <VChart :option="option" autoresize class="h-64 w-full" />
</template>
