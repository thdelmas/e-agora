<script setup>
// A figure's dedicated view, reached by clicking a leaderboard row. It shows
// the profile (portrait, bio, Wikipedia link) alongside the ranking numbers —
// the same conservative score, rating ± deviation and win record the board
// derives, so the two always reconcile (docs/04-api.md §GET /subjects/{id}).
import { computed, onMounted, ref, watch } from 'vue'
import { useRoute } from 'vue-router'
import { api } from '../api/client'
import AccessBanner from '../components/AccessBanner.vue'

const route = useRoute()

const detail = ref(null)
const error = ref('')
const loading = ref(true)
const expanded = ref(false)

async function load() {
  loading.value = true
  error.value = ''
  detail.value = null
  try {
    // Carry the pool flags from the URL so the rank matches the board the
    // visitor came from (the leaderboard row links with them).
    detail.value = await api.subject(route.params.id, route.query)
  } catch (e) {
    error.value = e.message || 'Could not load that figure.'
  } finally {
    loading.value = false
  }
}

onMounted(load)
// Re-fetch when navigating straight between two subject pages (no unmount).
watch(() => route.params.id, load)

const s = computed(() => detail.value?.subject || {})

// Mirror LeaderboardRow's derivations exactly so a figure's numbers read the
// same here as on the board: round first, then derive the headline score from
// the SAME rounded rating and deviation (score = rating − 2 × rd).
const rating = computed(() => Math.round(detail.value?.rating || 0))
const rd = computed(() => Math.round(detail.value?.ratingDeviation || 0))
const provisional = computed(() => rd.value > 110)
const score = computed(() => rating.value - 2 * rd.value)

const comparisons = computed(() => detail.value?.comparisons || 0)
const wins = computed(() => detail.value?.wins || 0)
const winPct = computed(() =>
  comparisons.value > 0
    ? Math.round((wins.value / comparisons.value) * 100)
    : 0,
)
const medal = computed(
  () => ({ 1: '🥇', 2: '🥈', 3: '🥉' }[detail.value?.rank] || null),
)
</script>

<template>
  <section class="subject-view">
    <AccessBanner />
    <RouterLink to="/leaderboard" class="back">
      ← Back to the rankings
    </RouterLink>

    <template v-if="detail">
      <article class="subject-card">
        <header class="subject-head">
          <img
            v-if="s.imageUrl"
            :src="s.imageUrl"
            :alt="s.name"
            class="portrait"
          />
          <div v-else class="portrait placeholder" aria-hidden="true"></div>

          <div class="head-text">
            <span class="rank-badge">
              <span v-if="medal" aria-hidden="true">{{ medal }}</span>
              <span v-else>#{{ detail.rank }}</span>
              <span class="rank-word">in this pool</span>
            </span>
            <h1 class="name">{{ s.name }}</h1>
            <p v-if="s.deceased" class="deceased-note">
              ✝ Deceased<template v-if="s.diedYear">
                ({{ s.diedYear }})</template>
            </p>
            <p v-if="s.description" class="desc">{{ s.description }}</p>
          </div>
        </header>

        <!-- Ranking numbers -->
        <dl class="stats-grid">
          <div class="stat" :class="{ provisional }">
            <dt>Rank score</dt>
            <dd>
              {{ score }}
              <span v-if="provisional" class="prov-tag">provisional</span>
            </dd>
            <p class="stat-note">
              rating {{ rating }} ± {{ rd }} — the conservative score is
              rating minus twice the ± uncertainty.
            </p>
          </div>
          <div class="stat">
            <dt>Win rate</dt>
            <dd>{{ winPct }}%</dd>
            <p class="stat-note">{{ wins }} of {{ comparisons }} votes won.</p>
          </div>
          <div class="stat">
            <dt>Votes</dt>
            <dd>{{ comparisons.toLocaleString() }}</dd>
            <p class="stat-note">head-to-head comparisons recorded.</p>
          </div>
        </dl>

        <!-- Wikipedia lead paragraph -->
        <template v-if="s.extract">
          <p class="lead" :class="{ clamped: !expanded }">{{ s.extract }}</p>
          <button
            class="more"
            type="button"
            :aria-expanded="expanded"
            @click="expanded = !expanded"
          >
            {{ expanded ? 'Show less' : 'Show more' }}
          </button>
        </template>

        <a
          v-if="s.wikipediaUrl"
          :href="s.wikipediaUrl"
          target="_blank"
          rel="noopener"
          class="wiki"
          :aria-label="`Read about ${s.name} on Wikipedia (opens in a new tab)`"
        >
          <span class="wiki-mark" aria-hidden="true">W</span>
          <span class="wiki-label">Read on Wikipedia</span>
          <span class="wiki-arrow" aria-hidden="true">↗</span>
        </a>
      </article>
    </template>

    <p v-else-if="loading" class="muted">Looking them up…</p>
    <p v-else-if="error" class="muted">{{ error }}</p>
  </section>
</template>
