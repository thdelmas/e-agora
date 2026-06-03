<script setup>
import { computed, onMounted, onUnmounted, ref } from 'vue'
import { useRoute } from 'vue-router'
import { api } from '../api/client'
import {
  applyVote,
  refreshMe,
  poolQuery,
  poolKeyOf,
  poolLabel,
  hasRecalled,
  markRecalled,
} from '../store'
import PoliticianCard from '../components/PoliticianCard.vue'
import LanguageNote from '../components/LanguageNote.vue'
import PoolPicker from '../components/PoolPicker.vue'
import RecallStep from '../components/RecallStep.vue'
import HumanityCheckModal from '../components/HumanityCheckModal.vue'

const route = useRoute()

const matchup = ref(null)
const error = ref('')
const loading = ref(true)
const busy = ref(false) // a vote is in flight
const toast = ref('')

// The belonging recall step (docs/11 §2): shown once per pool scope, before
// the matchup, to learn who the visitor associates with this pool.
const showRecall = ref(false)
const poolLabelText = computed(() => poolLabel())

const showHuman = ref(false)
const pendingVote = ref(null) // { winnerId, loserId } awaiting humanity check
// a recall proposal is in flight (may ingest a new figure)
const proposing = ref(false)

const lockedHint = computed(() => route.query.locked === '1')

let toastTimer
function flash(msg, seconds = 3) {
  toast.value = msg
  clearTimeout(toastTimer)
  toastTimer = setTimeout(() => (toast.value = ''), seconds * 1000)
}

// Show the recall step whenever we enter a pool scope the visitor hasn't
// answered.
function syncRecall() {
  showRecall.value = !hasRecalled(poolKeyOf())
}

async function load() {
  loading.value = true
  error.value = ''
  try {
    matchup.value = await api.matchup(poolQuery())
  } catch (e) {
    matchup.value = null
    error.value = e.message || 'Could not load a matchup.'
  } finally {
    loading.value = false
  }
}

// The pool changed: re-ask recall for the new scope (if unanswered) and reload.
function onPoolChange() {
  syncRecall()
  load()
}

// --- belonging recall --------------------------------------------------------

// payload is { subjectId } for an existing figure or { url } for a Wikipedia
// page not yet in the pool (ingested on first recall). The latter does network
// work, so guard against a double-submit while it's in flight.
async function doPropose(payload) {
  if (proposing.value) return
  proposing.value = true
  try {
    await api.propose(payload, poolQuery())
    markRecalled(poolKeyOf())
    showRecall.value = false
    flash('Thanks — noted who comes to mind here.')
  } catch (e) {
    if (e.code === 'not_a_person') {
      flash('e-agora is for people — that page isn’t a person.')
    } else if (e.code === 'not_a_wikipedia_page') {
      flash('Couldn’t find a Wikipedia page for that.')
    } else if (e.code === 'rate_limited') {
      flash('Whoa — slow down a moment.', Number(e.retryAfter) || 3)
    } else {
      flash(e.message || 'Could not record that.')
    }
  } finally {
    proposing.value = false
  }
}

function onSkip() {
  // "I don't know anyone here" still answers the prompt
  markRecalled(poolKeyOf())
  showRecall.value = false
}

// --- voting ------------------------------------------------------------------

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
  if (showHuman.value || showRecall.value || busy.value || !matchup.value) {
    return
  }
  if (e.key === 'ArrowLeft') prefer(matchup.value.a.id)
  else if (e.key === 'ArrowRight') prefer(matchup.value.b.id)
  else if (e.key === 's' || e.key === 'S') load()
}

onMounted(() => {
  syncRecall()
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
    <p v-if="lockedHint" class="locked-hint">
      🔓 Vote once to unlock the rankings for 24 hours.
    </p>

    <template v-if="!showRecall">
      <h1 class="prompt">Who would you rather have as a <em>leader?</em></h1>
      <p class="subprompt">
        Two figures enter the agora. Pick the one you'd rather see in charge.
      </p>
    </template>

    <p v-if="loading" class="muted">Summoning two challengers…</p>

    <RecallStep
      v-else-if="showRecall"
      :label="poolLabelText"
      :busy="proposing"
      @propose="doPropose"
      @skip="onSkip"
    />

    <div v-else-if="matchup" class="cards" :class="{ busy }">
      <PoliticianCard
        :key="matchup.a.id"
        :subject="matchup.a"
        side="a"
        @prefer="prefer"
      />
      <span class="vs" aria-hidden="true">VS</span>
      <PoliticianCard
        :key="matchup.b.id"
        :subject="matchup.b"
        side="b"
        @prefer="prefer"
      />
    </div>

    <p v-else class="muted">
      The agora is still being set up — check back soon.
      <span v-if="error" class="detail">({{ error }})</span>
    </p>

    <p v-if="matchup && !showRecall" class="privacy-note">
      🔒 <strong>Your vote is anonymous.</strong> We record only your
      choice — never your identity. No account, no email, no IP tracking,
      nothing that can be traced back to you.
    </p>

    <LanguageNote
      v-if="matchup && !showRecall && matchup.fallbackApplied"
      :lang="matchup.displayLang"
    />

    <button
      v-if="!showRecall"
      class="ghost"
      :disabled="busy"
      @click="load"
    >↻ Skip — show me another pair</button>
    <p v-if="!showRecall" class="kbd muted">
      Tip: <kbd>←</kbd> / <kbd>→</kbd> to choose, <kbd>S</kbd> to skip.
    </p>

    <PoolPicker :disabled="busy" @change="onPoolChange" />
    <p class="pool-hint muted">
      Pick a region or “famous only” to compare people you’re more likely
      to know.
    </p>

    <transition name="fade">
      <p v-if="toast" class="toast" role="status">{{ toast }}</p>
    </transition>

    <HumanityCheckModal
      v-if="showHuman"
      @verified="onVerified"
      @close="onHumanClosed"
    />
  </section>
</template>
