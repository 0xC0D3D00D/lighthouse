<script setup lang="ts">
import { use } from 'echarts/core'
import { CanvasRenderer } from 'echarts/renderers'
import { LineChart } from 'echarts/charts'
import { GridComponent, TooltipComponent, LegendComponent } from 'echarts/components'
import VChart from 'vue-echarts'
import 'vue-echarts/style.css'

use([CanvasRenderer, LineChart, GridComponent, TooltipComponent, LegendComponent])

interface Bucket {
  unix: number
  queries: number
  errors: number
  cache_hits: number
  backend_errors: number
}

const props = defineProps<{ buckets: Bucket[] }>()

const colorMode = useColorMode()

// Series and chrome colors validated per mode against the card surface
// (contrast >= 3:1, CVD-safe adjacent pairs).
const theme = computed(() => colorMode.value === 'dark'
  ? {
      queries: '#00a86e',
      cacheHits: '#615fff',
      errors: '#fb2c36',
      text: '#cad5e2',
      muted: '#90a1b9',
      grid: '#2c3a52',
      axis: '#45556c',
      tooltipBg: '#1d293d',
      tooltipBorder: '#314158',
    }
  : {
      queries: '#007a52',
      cacheHits: '#4f39f6',
      errors: '#c10007',
      text: '#45556c',
      muted: '#62748e',
      grid: '#e2e8f0',
      axis: '#cad5e2',
      tooltipBg: '#ffffff',
      tooltipBorder: '#e2e8f0',
    })

const option = computed(() => ({
  animation: false,
  tooltip: {
    trigger: 'axis',
    backgroundColor: theme.value.tooltipBg,
    borderColor: theme.value.tooltipBorder,
    textStyle: { color: theme.value.text },
  },
  legend: {
    data: ['Queries', 'Cache hits', 'Errors'],
    bottom: 0,
    textStyle: { color: theme.value.text },
  },
  grid: { left: 40, right: 16, top: 16, bottom: 40 },
  xAxis: {
    type: 'category',
    data: props.buckets.map(b => new Date(b.unix * 1000).toLocaleTimeString()),
    axisLabel: { color: theme.value.muted },
    axisLine: { lineStyle: { color: theme.value.axis } },
    axisTick: { lineStyle: { color: theme.value.axis } },
  },
  yAxis: {
    type: 'value',
    minInterval: 1,
    axisLabel: { color: theme.value.muted },
    splitLine: { lineStyle: { color: theme.value.grid } },
  },
  series: [
    { name: 'Queries', type: 'line', showSymbol: false, lineStyle: { width: 2 }, data: props.buckets.map(b => b.queries), color: theme.value.queries },
    { name: 'Cache hits', type: 'line', showSymbol: false, lineStyle: { width: 2 }, data: props.buckets.map(b => b.cache_hits), color: theme.value.cacheHits },
    { name: 'Errors', type: 'line', showSymbol: false, lineStyle: { width: 2 }, data: props.buckets.map(b => b.errors), color: theme.value.errors },
  ],
}))
</script>

<template>
  <div class="h-64 w-full">
    <VChart :option="option" autoresize />
  </div>
</template>
