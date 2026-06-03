<script setup>
// Public, ungated transparency dashboard (docs/04-api.md §GET /api/stats).
// Everything shown is an aggregate count over anonymous data — no figure here
// can be traced to any visitor, consistent with e-agora's anonymity promise.
import { computed, onMounted, ref } from 'vue'
import { api } from '../api/client'
import TimeSeriesChart from '../components/TimeSeriesChart.vue'

const data = ref(null)
const error = ref('')
const loading = ref(true)
const days = ref(30)

const WINDOWS = [7, 30, 90]

async function load() {
  loading.value = true
  error.value = ''
  try {
    data.value = await api.stats(days.value)
  } catch (e) {
    data.value = null
    error.value = e.message || 'Could not load the stats.'
  } finally {
    loading.value = false
  }
}

function setWindow(d) {
  if (d === days.value) return
  days.value = d
  load()
}

onMounted(load)

function seriesOf(key) {
  return (data.value?.daily || []).map((d) => ({ date: d.date, value: d[key] }))
}
const votesSeries = computed(() => seriesOf('votes'))
const visitorsSeries = computed(() => seriesOf('visitors'))
const votersSeries = computed(() => seriesOf('voters'))
const addedSeries = computed(() => seriesOf('added'))

// In-window subtotals (the all-time figures live in the headline cards).
const windowTotals = computed(() => {
  const d = data.value?.daily || []
  const sum = (k) => d.reduce((a, r) => a + r[k], 0)
  return {
    votes: sum('votes'),
    visitors: sum('visitors'),
    voters: sum('voters'),
    added: sum('added'),
  }
})

const totals = computed(() => data.value?.totals || {})
const isEmpty = computed(
  () =>
    data.value &&
    (totals.value.votes || 0) === 0 &&
    (totals.value.visitors || 0) === 0,
)

function fmt(n) {
  return (n ?? 0).toLocaleString()
}
</script>

<template>
  <section class="stats">
    <h1><span class="crown">📊</span> The agora in numbers</h1>
    <p class="muted intro">
      A live, public look at the arena. Every figure below is an aggregate
      count over <strong>anonymous</strong> activity — never anything that
      could identify a visitor.
    </p>

    <template v-if="data">
      <!-- All-time headline figures -->
      <div class="stat-cards">
        <div class="stat-card votes">
          <span class="stat-num">{{ fmt(totals.votes) }}</span>
          <span class="stat-key">🗳️ votes cast</span>
        </div>
        <div class="stat-card visitors">
          <span class="stat-num">{{ fmt(totals.visitors) }}</span>
          <span class="stat-key">👥 visitors</span>
        </div>
        <div class="stat-card voters">
          <span class="stat-num">{{ fmt(totals.voters) }}</span>
          <span class="stat-key">✋ have voted</span>
        </div>
        <div class="stat-card pool">
          <span class="stat-num">{{ fmt(totals.subjects) }}</span>
          <span class="stat-key">🏛️ in the arena</span>
        </div>
        <div class="stat-card added">
          <span class="stat-num">{{ fmt(totals.userContributed) }}</span>
          <span class="stat-key">➕ added by visitors</span>
        </div>
      </div>

      <p v-if="isEmpty" class="empty-note">
        No activity yet — be the first to step into the arena and vote.
      </p>

      <!-- Time-window picker -->
      <div class="window-picker" role="group" aria-label="Time window">
        <button
          v-for="d in WINDOWS"
          :key="d"
          class="window-btn"
          :class="{ active: d === days }"
          :aria-pressed="d === days"
          @click="setWindow(d)"
        >
          {{ d }} days
        </button>
      </div>

      <!-- Charts -->
      <div class="charts">
        <figure class="chart-card">
          <figcaption>
            <h2>🗳️ Votes over time</h2>
            <span class="chart-sub">
              {{ fmt(windowTotals.votes) }} in the last {{ days }} days
            </span>
          </figcaption>
          <TimeSeriesChart
            :series="votesSeries"
            mode="bars"
            color="#0f9d8e"
            unit="votes"
            label="Votes per day"
          />
        </figure>

        <figure class="chart-card">
          <figcaption>
            <h2>👥 Visitors over time</h2>
            <span class="chart-sub">
              {{ fmt(windowTotals.visitors) }} new in the last {{ days }} days
            </span>
          </figcaption>
          <TimeSeriesChart
            :series="visitorsSeries"
            mode="area"
            color="#ef7a45"
            unit="visitors"
            label="New visitors per day"
          />
          <p class="chart-note">
            A “visitor” is an anonymous browser the first time we see it —
            counted with no IP, account, or identity. It reflects new visitors
            per day, not raw page views (which we don’t log).
          </p>
        </figure>

        <figure class="chart-card">
          <figcaption>
            <h2>✋ Active voters per day</h2>
            <span class="chart-sub">distinct anonymous voters each day</span>
          </figcaption>
          <TimeSeriesChart
            :series="votersSeries"
            mode="area"
            color="#1f6e4c"
            unit="voters"
            label="Active voters per day"
          />
        </figure>

        <figure class="chart-card">
          <figcaption>
            <h2>➕ People added per day</h2>
            <span class="chart-sub">
              {{ fmt(windowTotals.added) }} added in the last {{ days }} days
            </span>
          </figcaption>
          <TimeSeriesChart
            :series="addedSeries"
            mode="bars"
            color="#d8a23a"
            unit="added"
            label="People added per day"
          />
        </figure>
      </div>

      <p class="privacy-note stats-privacy">
        🔒 <strong>Privacy by design.</strong> These charts are built only
        from anonymous counts — choices, sessions, and pool growth. We never
        record who you are, where you are, or your IP. There is nothing here,
        and nothing in the database, that can be traced back to a person.
      </p>
    </template>

    <p v-else-if="loading" class="muted">Tallying the agora…</p>
    <p v-else-if="error" class="muted">{{ error }}</p>

    <RouterLink to="/" class="cta">← Back to the arena</RouterLink>
  </section>
</template>
