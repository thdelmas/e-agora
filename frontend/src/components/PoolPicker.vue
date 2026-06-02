<script setup>
// The visitor's pool selector (docs/10 §4): region, fame tier and the
// living/deceased status. It writes the shared store and emits `change` so the
// host view reloads the matchup or leaderboard from the newly-scoped pool. The
// ranking itself is one global scale — these only filter which figures are drawn.
import {
  REGIONS,
  poolRegion,
  poolFameTop,
  includeDeceased,
  setPoolRegion,
  setPoolFameTop,
  setIncludeDeceased,
} from '../store'

const emit = defineEmits(['change'])
defineProps({ disabled: { type: Boolean, default: false } })

const regionLabel = (r) => r || '🌍 Whole world'

function onRegion(e) {
  setPoolRegion(e.target.value)
  emit('change')
}
function onFame(e) {
  setPoolFameTop(e.target.checked)
  emit('change')
}
function onDeceased(e) {
  setIncludeDeceased(e.target.checked)
  emit('change')
}
</script>

<template>
  <div class="pool-picker" role="group" aria-label="Choose who to compare">
    <label class="pool-field">
      <span class="pool-label">Region</span>
      <select :value="poolRegion" :disabled="disabled" @change="onRegion">
        <option v-for="r in REGIONS" :key="r" :value="r">{{ regionLabel(r) }}</option>
      </select>
    </label>

    <label class="pool-toggle">
      <input type="checkbox" :checked="poolFameTop" :disabled="disabled" @change="onFame" />
      ⭐ Famous only
    </label>

    <label class="pool-toggle">
      <input type="checkbox" :checked="includeDeceased" :disabled="disabled" @change="onDeceased" />
      Include figures who have died
    </label>
  </div>
</template>

<style scoped>
.pool-picker {
  display: flex;
  flex-wrap: wrap;
  align-items: center;
  justify-content: center;
  gap: 0.75rem 1.25rem;
  margin: 0.5rem auto 0.25rem;
  font-size: 0.9rem;
}
.pool-field {
  display: inline-flex;
  align-items: center;
  gap: 0.4rem;
}
.pool-label {
  opacity: 0.8;
}
.pool-field select {
  padding: 0.25rem 0.5rem;
  border-radius: 6px;
}
.pool-toggle {
  display: inline-flex;
  align-items: center;
  gap: 0.4rem;
  cursor: pointer;
}
</style>
