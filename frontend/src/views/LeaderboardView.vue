<script setup>
import { onMounted, ref } from 'vue'
import { api } from '../api/client'
import LeaderboardRow from '../components/LeaderboardRow.vue'
import AccessBanner from '../components/AccessBanner.vue'

const board = ref(null)
const error = ref('')

async function load() {
  try {
    board.value = await api.leaderboard({ limit: 100 })
  } catch (e) {
    error.value = e.message || 'Could not load the leaderboard.'
  }
}

onMounted(load)
</script>

<template>
  <section class="leaderboard">
    <AccessBanner />
    <h1>World rankings</h1>
    <p class="muted">Reflects the aggregated preferences of anonymous visitors.</p>

    <template v-if="board">
      <p class="stat">Total votes cast (all visitors): {{ board.totalVotes }}</p>
      <ol class="rows">
        <LeaderboardRow v-for="entry in board.entries" :key="entry.subject.id" :entry="entry" />
      </ol>
    </template>
    <p v-else-if="error" class="muted">{{ error }}</p>

    <RouterLink to="/" class="cta">Keep voting</RouterLink>
  </section>
</template>
