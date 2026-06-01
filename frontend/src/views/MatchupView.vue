<script setup>
import { computed, onMounted, onUnmounted, ref } from 'vue'
import { useRoute } from 'vue-router'
import { api } from '../api/client'
import { applyVote, refreshMe } from '../store'
import PoliticianCard from '../components/PoliticianCard.vue'
import LanguageNote from '../components/LanguageNote.vue'
import HumanityCheckModal from '../components/HumanityCheckModal.vue'

const route = useRoute()

const matchup = ref(null)
const error = ref('')
const loading = ref(true)
const busy = ref(false) // a vote is in flight
const toast = ref('')

const showHuman = ref(false)
const pendingVote = ref(null) // { winnerId, loserId } awaiting humanity check

const lockedHint = computed(() => route.query.locked === '1')

let toastTimer
function flash(msg, seconds = 3) {
  toast.value = msg
  clearTimeout(toastTimer)
  toastTimer = setTimeout(() => (toast.value = ''), seconds * 1000)
}

async function load() {
  loading.value = true
  error.value = ''
  try {
    matchup.value = await api.matchup()
  } catch (e) {
    matchup.value = null
    error.value = e.message || 'Could not load a matchup.'
  } finally {
    loading.value = false
  }
}

function prefer(winnerId) {
  if (busy.value || !matchup.value) return
  const { a, b } = matchup.value
  const loserId = winnerId === a.id ? b.id : a.id
  castVote(winnerId, loserId)
}

async function castVote(winnerId, loserId) {
  busy.value = true
  try {
    const res = await api.vote(winnerId, loserId)
    applyVote(res)
    // applyVote unlocks access instantly, but the vote response carries no
    // `canAdd` flag — so without this refresh the "Add someone" button stays
    // disabled after the first vote. Pull authoritative state alongside the
    // next matchup fetch.
    await Promise.all([refreshMe(), load()])
  } catch (e) {
    if (e.code === 'human_check_required') {
      pendingVote.value = { winnerId, loserId }
      showHuman.value = true
    } else if (e.code === 'rate_limited') {
      flash('Whoa — slow down a moment.', Number(e.retryAfter) || 3)
    } else {
      flash(e.message || 'Your vote didn’t go through.')
    }
  } finally {
    busy.value = false
  }
}

function onVerified() {
  showHuman.value = false
  const pv = pendingVote.value
  pendingVote.value = null
  if (pv) castVote(pv.winnerId, pv.loserId)
}

function onHumanClosed() {
  showHuman.value = false
  pendingVote.value = null
}

function onKey(e) {
  if (showHuman.value || busy.value || !matchup.value) return
  if (e.key === 'ArrowLeft') prefer(matchup.value.a.id)
  else if (e.key === 'ArrowRight') prefer(matchup.value.b.id)
  else if (e.key === 's' || e.key === 'S') load()
}

onMounted(() => {
  load()
  window.addEventListener('keydown', onKey)
})
onUnmounted(() => {
  window.removeEventListener('keydown', onKey)
  clearTimeout(toastTimer)
})
</script>

<template>
  <section class="matchup">
    <p v-if="lockedHint" class="locked-hint">🔓 Vote once to unlock the rankings for 24 hours.</p>

    <h1 class="prompt">Who would you rather have as a <em>leader?</em></h1>
    <p class="subprompt">Two figures enter the agora. Pick the one you'd rather see in charge.</p>

    <p v-if="loading" class="muted">Summoning two challengers…</p>

    <div v-else-if="matchup" class="cards" :class="{ busy }">
      <PoliticianCard :key="matchup.a.id" :subject="matchup.a" side="a" @prefer="prefer" />
      <span class="vs" aria-hidden="true">VS</span>
      <PoliticianCard :key="matchup.b.id" :subject="matchup.b" side="b" @prefer="prefer" />
    </div>

    <p v-else class="muted">
      The agora is still being set up — check back soon.
      <span v-if="error" class="detail">({{ error }})</span>
    </p>

    <p v-if="matchup" class="privacy-note">
      🔒 <strong>Your vote is anonymous.</strong> We record only your choice — never your identity.
      No account, no email, no IP tracking, nothing that can be traced back to you.
    </p>

    <LanguageNote v-if="matchup && matchup.fallbackApplied" :lang="matchup.displayLang" />

    <button class="ghost" :disabled="busy" @click="load">↻ Skip — show me another pair</button>
    <p class="kbd muted">Tip: <kbd>←</kbd> / <kbd>→</kbd> to choose, <kbd>S</kbd> to skip.</p>

    <transition name="fade">
      <p v-if="toast" class="toast" role="status">{{ toast }}</p>
    </transition>

    <HumanityCheckModal v-if="showHuman" @verified="onVerified" @close="onHumanClosed" />
  </section>
</template>
