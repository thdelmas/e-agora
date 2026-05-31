<script setup>
// Live 24h access countdown + re-lock state (R10), read from the shared store.
// The window is fixed (not rolling): voting during it doesn't extend it.
import { computed, onMounted, onUnmounted, ref } from 'vue'
import { me } from '../store'

const now = ref(Date.now())
let timer
onMounted(() => {
  timer = setInterval(() => (now.value = Date.now()), 1000)
})
onUnmounted(() => clearInterval(timer))

const msLeft = computed(() =>
  me.accessExpiresAt ? new Date(me.accessExpiresAt).getTime() - now.value : 0,
)
const active = computed(() => me.hasAccess && msLeft.value > 0)

const label = computed(() => {
  if (!active.value) return 'Vote once to unlock the rankings for 24 hours.'
  const s = Math.floor(msLeft.value / 1000)
  const h = Math.floor(s / 3600)
  const m = Math.floor((s % 3600) / 60)
  const sec = s % 60
  const t = h > 0 ? `${h}h ${m}m` : m > 0 ? `${m}m ${sec}s` : `${sec}s`
  return `Access expires in ${t} — vote again after it lapses to renew.`
})
</script>

<template>
  <div class="access-banner" :class="{ active }" aria-live="polite">{{ label }}</div>
</template>
