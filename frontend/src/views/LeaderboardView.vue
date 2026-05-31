<script setup>
import { onMounted, onUnmounted, ref, watch } from 'vue'
import { api } from '../api/client'
import LeaderboardRow from '../components/LeaderboardRow.vue'
import AccessBanner from '../components/AccessBanner.vue'

// The pool holds hundreds of subjects, so we page through them instead of
// capping the view at the first 100. Each scroll to the bottom (or click of the
// fallback button) pulls the next slice by offset; we stop once a short page
// signals the end (docs/04-api.md §GET /leaderboard — limit/offset).
const PAGE_SIZE = 50

const entries = ref([])
const totalVotes = ref(0)
const error = ref('')
const loading = ref(false) // a page fetch is in flight
const reachedEnd = ref(false)
const started = ref(false) // first page has been requested

let offset = 0
const seen = new Set() // subject ids already shown — guards against dup keys
                       // if the ranking shifts between page fetches

const sentinel = ref(null)
let observer = null

async function loadMore() {
  if (loading.value || reachedEnd.value) return
  loading.value = true
  error.value = ''
  try {
    const page = await api.leaderboard({ limit: PAGE_SIZE, offset })
    totalVotes.value = page.totalVotes
    for (const entry of page.entries) {
      if (seen.has(entry.subject.id)) continue
      seen.add(entry.subject.id)
      entries.value.push(entry)
    }
    offset += page.count
    if (page.count < PAGE_SIZE) reachedEnd.value = true
  } catch (e) {
    error.value = e.message || 'Could not load the leaderboard.'
  } finally {
    loading.value = false
    started.value = true
  }
}

// Auto-load the next page as the sentinel scrolls into view; the button it sits
// on is also a manual fallback (and the path taken without IntersectionObserver).
watch(sentinel, (el, prev) => {
  if (prev) observer?.unobserve(prev)
  if (!el) return
  if (!observer && typeof IntersectionObserver !== 'undefined') {
    observer = new IntersectionObserver(
      (items) => {
        if (items.some((it) => it.isIntersecting)) loadMore()
      },
      { rootMargin: '300px' },
    )
  }
  observer?.observe(el)
})

onMounted(loadMore)
onUnmounted(() => observer?.disconnect())
</script>

<template>
  <section class="leaderboard">
    <AccessBanner />
    <h1><span class="crown">🏛️</span> World rankings</h1>
    <p class="muted">Forged head-to-head from the aggregated preferences of anonymous visitors.</p>

    <template v-if="entries.length">
      <p class="stat">🗳️ {{ totalVotes.toLocaleString() }} votes cast by visitors worldwide</p>
      <ol class="rows">
        <LeaderboardRow v-for="entry in entries" :key="entry.subject.id" :entry="entry" />
      </ol>

      <button
        v-if="!reachedEnd"
        ref="sentinel"
        class="ghost load-more"
        :disabled="loading"
        @click="loadMore"
      >
        {{ loading ? 'Loading…' : 'Load more' }}
      </button>
      <p v-else class="muted end-note">— that's everyone in the agora —</p>
    </template>

    <p v-else-if="loading" class="muted">Tallying the rankings…</p>
    <p v-else-if="error" class="muted">{{ error }}</p>
    <p v-else-if="started" class="muted">No rankings yet — be the first to cast a vote.</p>

    <RouterLink to="/" class="cta">← Back to the arena</RouterLink>
  </section>
</template>
