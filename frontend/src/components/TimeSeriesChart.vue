<script setup>
// A tiny dependency-free SVG chart for the public stats page. Renders a daily
// series either as an area+line (cumulative-feel trends) or as bars (discrete
// counts). It scales to its container width via the viewBox (no JS resize), and
// is screen-reader friendly: an `aria-label` summarizes the series and each
// point carries a native <title> tooltip.
import { computed } from 'vue'

const props = defineProps({
  series: { type: Array, required: true }, // [{ date: 'YYYY-MM-DD', value: Number }]
  mode: { type: String, default: 'area' }, // 'area' | 'bars'
  color: { type: String, default: '#0f9d8e' },
  label: { type: String, default: 'time series' },
  unit: { type: String, default: '' },
})

// Internal drawing space; the SVG scales uniformly to the container width.
const W = 1000
const H = 320
const PAD = { l: 54, r: 16, t: 18, b: 30 }
const x0 = PAD.l
const x1 = W - PAD.r
const yTop = PAD.t
const yBase = H - PAD.b

const n = computed(() => props.series.length)
const values = computed(() => props.series.map((p) => p.value || 0))
const maxVal = computed(() => Math.max(1, ...values.value))

function xAt(i) {
  if (n.value <= 1) return (x0 + x1) / 2
  return x0 + ((x1 - x0) * i) / (n.value - 1)
}
function yAt(v) {
  return yBase - (v / maxVal.value) * (yBase - yTop)
}

const points = computed(() =>
  props.series.map((p, i) => ({
    value: p.value || 0,
    x: xAt(i),
    y: yAt(p.value || 0),
    short: formatShort(p.date),
  })),
)

// Bars geometry.
const slot = computed(() => (x1 - x0) / Math.max(1, n.value))
const barW = computed(() => Math.max(1, Math.min(38, slot.value * 0.66)))
function barX(i) {
  return x0 + i * slot.value + (slot.value - barW.value) / 2
}

// Area fill + line stroke paths.
const linePath = computed(() =>
  points.value.map((p, i) => `${i ? 'L' : 'M'}${p.x.toFixed(1)},${p.y.toFixed(1)}`).join(' '),
)
const areaPath = computed(() => {
  const pts = points.value
  if (!pts.length) return ''
  const first = pts[0]
  const last = pts[pts.length - 1]
  return (
    `M${first.x.toFixed(1)},${yBase} ` +
    pts.map((p) => `L${p.x.toFixed(1)},${p.y.toFixed(1)}`).join(' ') +
    ` L${last.x.toFixed(1)},${yBase} Z`
  )
})

// Dots only when the series is short enough to stay legible.
const showDots = computed(() => props.mode === 'area' && n.value > 1 && n.value <= 45)

// Sparse x-axis labels (first / middle / last) so they don't crowd.
const xLabels = computed(() => {
  const pts = points.value
  if (!pts.length) return []
  const idxs = pts.length <= 2 ? pts.map((_, i) => i) : [0, Math.floor((pts.length - 1) / 2), pts.length - 1]
  return [...new Set(idxs)].map((i) => ({
    x: pts[i].x,
    label: pts[i].short,
    anchor: i === 0 ? 'start' : i === pts.length - 1 ? 'end' : 'middle',
  }))
})

const ariaLabel = computed(() => {
  const total = values.value.reduce((a, b) => a + b, 0)
  return `${props.label}: ${total.toLocaleString()} ${props.unit} total over ${n.value} days; peak ${maxVal.value.toLocaleString()} in a day.`
})

// Unique gradient id per instance without relying on randomness.
const uid = nextId()
const gradId = `eachart-grad-${uid}`

function formatShort(d) {
  if (!d) return ''
  const [y, m, day] = d.split('-').map(Number)
  if (!y) return d
  return new Date(y, m - 1, day).toLocaleDateString(undefined, { month: 'short', day: 'numeric' })
}
function fmt(v) {
  return Number(v).toLocaleString()
}
</script>

<script>
// Module-scoped counter for stable, collision-free gradient ids.
let _uid = 0
function nextId() {
  return _uid++
}
</script>

<template>
  <figure class="chart" :class="`chart-${mode}`">
    <svg :viewBox="`0 0 ${W} ${H}`" class="chart-svg" role="img" :aria-label="ariaLabel">
      <defs>
        <linearGradient :id="gradId" x1="0" y1="0" x2="0" y2="1">
          <stop offset="0%" :stop-color="color" stop-opacity="0.34" />
          <stop offset="100%" :stop-color="color" stop-opacity="0.02" />
        </linearGradient>
      </defs>

      <!-- gridlines + y-axis bounds -->
      <line :x1="x0" :y1="yTop" :x2="x1" :y2="yTop" class="grid grid-faint" />
      <line :x1="x0" :y1="yBase" :x2="x1" :y2="yBase" class="grid" />
      <text :x="x0 - 10" :y="yTop" class="ylabel" text-anchor="end" dominant-baseline="middle">{{ fmt(maxVal) }}</text>
      <text :x="x0 - 10" :y="yBase" class="ylabel" text-anchor="end" dominant-baseline="middle">0</text>

      <!-- bars -->
      <template v-if="mode === 'bars'">
        <rect
          v-for="(p, i) in points"
          :key="i"
          :x="barX(i)"
          :y="p.y"
          :width="barW"
          :height="Math.max(0, yBase - p.y)"
          rx="2"
          class="bar"
          :style="{ fill: color }"
        >
          <title>{{ p.short }}: {{ fmt(p.value) }} {{ unit }}</title>
        </rect>
      </template>

      <!-- area + line -->
      <template v-else>
        <path :d="areaPath" :fill="`url(#${gradId})`" class="area" />
        <path :d="linePath" fill="none" :stroke="color" class="line" />
        <template v-if="showDots">
          <circle v-for="(p, i) in points" :key="i" :cx="p.x" :cy="p.y" r="3.5" class="dot" :style="{ fill: color }">
            <title>{{ p.short }}: {{ fmt(p.value) }} {{ unit }}</title>
          </circle>
        </template>
      </template>

      <!-- x-axis labels -->
      <text v-for="(l, i) in xLabels" :key="'x' + i" :x="l.x" :y="H - 9" class="xlabel" :text-anchor="l.anchor">
        {{ l.label }}
      </text>
    </svg>
  </figure>
</template>
