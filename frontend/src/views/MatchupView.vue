<script setup>
import { onMounted, ref } from 'vue'
import { api } from '../api/client'
import PoliticianCard from '../components/PoliticianCard.vue'
import LanguageNote from '../components/LanguageNote.vue'

const matchup = ref(null)
const error = ref('')
const loading = ref(true)

async function load() {
  loading.value = true
  error.value = ''
  try {
    matchup.value = await api.matchup()
  } catch (e) {
    // The backend endpoints are stubbed until M3 — show a friendly state.
    error.value = e.message || 'Could not load a matchup.'
  } finally {
    loading.value = false
  }
}

onMounted(load)
</script>

<template>
  <section class="matchup">
    <h1 class="prompt">Who do you prefer?</h1>

    <p v-if="loading" class="muted">Loading a matchup…</p>

    <div v-else-if="matchup" class="cards">
      <PoliticianCard :subject="matchup.a" />
      <span class="vs">vs</span>
      <PoliticianCard :subject="matchup.b" />
      <LanguageNote v-if="matchup.fallbackApplied" :lang="matchup.displayLang" />
    </div>

    <p v-else class="muted">
      The agora is still being set up — check back soon.
      <span class="detail">({{ error }})</span>
    </p>

    <button class="ghost" @click="load">Skip / show another pair</button>
  </section>
</template>
