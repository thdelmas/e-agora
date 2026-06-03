<script setup>
// The visitor's pool selector (docs/10 §4): geographic scope, fame tier and
// the living/deceased status. It writes the shared store and emits `change` so
// the host view reloads the matchup or leaderboard from the newly-scoped pool.
// The ranking itself is one global scale — these only filter which figures
// are drawn.
import { ref, computed, onMounted } from 'vue'
import {
  REGIONS,
  poolRegion,
  poolCountry,
  poolFameTop,
  includeDeceased,
  setPoolRegion,
  setPoolCountry,
  setPoolFameTop,
  setIncludeDeceased,
} from '../store'
import { api } from '../api/client'

const emit = defineEmits(['change'])
defineProps({ disabled: { type: Boolean, default: false } })

// Continents are a fixed list; countries are whichever ones have enough
// subjects to draw from (loaded once). Geographic scope is one axis at two
// zoom levels, so the picker is a single select: a country value wins over a
// continent when set.
const continents = REGIONS.filter((r) => r)
const countries = ref([])
const scope = computed(() => poolCountry.value || poolRegion.value || '')

onMounted(async () => {
  try {
    const res = await api.countries()
    countries.value = res.countries || []
  } catch {
    countries.value = [] // reference data only; the continent scopes still work
  }
})

// One handler for the whole geographic axis: a known continent sets the region
// (clearing any country); anything else is a country (clearing the region);
// the empty value is the whole world. Choosing one zoom level always clears
// the other.
function onScope(e) {
  const val = e.target.value
  if (!val) {
    setPoolRegion('')
    setPoolCountry('')
  } else if (REGIONS.includes(val)) {
    setPoolRegion(val)
    setPoolCountry('')
  } else {
    setPoolCountry(val)
    setPoolRegion('')
  }
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
      <span class="pool-label">Scope</span>
      <select :value="scope" :disabled="disabled" @change="onScope">
        <option value="">🌍 Whole world</option>
        <optgroup label="Continents">
          <option v-for="r in continents" :key="r" :value="r">{{ r }}</option>
        </optgroup>
        <optgroup v-if="countries.length" label="Countries">
          <option v-for="c in countries" :key="c.name" :value="c.name">
            {{ c.name }} ({{ c.count }})
          </option>
        </optgroup>
      </select>
    </label>

    <label class="pool-toggle">
      <input
        type="checkbox"
        :checked="poolFameTop"
        :disabled="disabled"
        @change="onFame"
      />
      ⭐ Famous only
    </label>

    <label class="pool-toggle">
      <input
        type="checkbox"
        :checked="includeDeceased"
        :disabled="disabled"
        @change="onDeceased"
      />
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
