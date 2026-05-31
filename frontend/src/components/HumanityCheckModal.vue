<script setup>
// S4 — the dissent-based humanity check (R12). Read the statement and decide:
// a thinking human refuses a sycophantic "oath" (and agrees with a sincere
// "control"); a compliant script doesn't. Click-only (no typing). We also report
// a soft interaction-timing summary — never the gate on its own, just a signal.
//
// In `gate` mode this is the entry gate to the whole agora: there's no way to
// dismiss it — you think for yourself and pass, or you stay out.
import { onMounted, onUnmounted, ref } from 'vue'
import { api } from '../api/client'

const props = defineProps({ gate: { type: Boolean, default: false } })
const emit = defineEmits(['verified', 'close'])

const challenge = ref(null)
const nudge = ref('')
const submitting = ref(false)
let shownAt = 0
let pointerMoves = 0

async function loadChallenge() {
  challenge.value = null
  try {
    challenge.value = await api.humanChallenge()
    shownAt = performance.now()
    pointerMoves = 0
  } catch {
    nudge.value = 'Could not load the check — close this and try voting again.'
  }
}

function onMove() {
  pointerMoves++
}

async function choose(optionId) {
  if (submitting.value || !challenge.value) return
  submitting.value = true
  const decideMs = Math.round(performance.now() - shownAt)
  const timing = { decideMs, instant: decideMs < 150, pointerMoves }
  try {
    const res = await api.humanVerify(challenge.value.challengeId, optionId, timing)
    if (res.verified) {
      emit('verified')
      return
    }
    nudge.value = nudgeFor(res.reason)
    challenge.value = res.challengeId
      ? { challengeId: res.challengeId, prompt: res.prompt, kind: res.kind, options: res.options }
      : null
    shownAt = performance.now()
    pointerMoves = 0
  } catch (e) {
    nudge.value = e.message || 'Try again.'
    await loadChallenge()
  } finally {
    submitting.value = false
  }
}

function nudgeFor(reason) {
  switch (reason) {
    case 'too_fast':
      return 'Take a breath — that was a bit quick. Decide again.'
    case 'expired':
      return 'That one expired — here’s a fresh one.'
    case 'try_again':
      return 'This is a sanctuary for humans who think by themselves.'
    default:
      return 'Here’s a fresh one.'
  }
}

function onKey(e) {
  if (e.key === 'Escape' && !props.gate) emit('close')
}
onMounted(() => {
  loadChallenge()
  window.addEventListener('mousemove', onMove)
  window.addEventListener('keydown', onKey)
})
onUnmounted(() => {
  window.removeEventListener('mousemove', onMove)
  window.removeEventListener('keydown', onKey)
})
</script>

<template>
  <div class="modal-backdrop" @click.self="!gate && $emit('close')">
    <div class="modal" role="dialog" aria-modal="true" aria-label="Humanity check">
      <h2>{{ gate ? 'Enter the agora' : 'One quick thing' }}</h2>
      <p class="muted">The agora is for citizens who think for themselves — not scripts. Read this, then decide.</p>

      <template v-if="challenge">
        <blockquote class="oath">“{{ challenge.prompt }}”</blockquote>
        <p v-if="nudge" class="nudge">{{ nudge }}</p>
        <div class="human-options">
          <button v-for="o in challenge.options" :key="o.id" class="opt" :disabled="submitting" @click="choose(o.id)">
            {{ o.label }}
          </button>
        </div>
      </template>
      <p v-else class="muted">Loading…</p>

      <button v-if="!gate" class="ghost" @click="$emit('close')">Cancel</button>
    </div>
  </div>
</template>
