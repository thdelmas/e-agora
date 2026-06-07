<script setup>
import { computed, reactive, ref, watch } from 'vue'
import { api } from '../api/client'
import { poolQuery } from '../store'

// Why a subject is in the current pool + a confirm/infirm control on that
// membership (docs/11 §7). Membership is seeded from Wikidata P27, which is
// trusted but imperfect (a stale colonial citizenship can land a figure in the
// wrong pool), so the crowd gets a signed say that adjusts the belonging score
// without ever touching the global rating. Renders nothing for the world pool,
// where `belonging` is null.
const props = defineProps({
  subjectId: { type: Number, required: true },
  belonging: { type: Object, default: null },
  // 'card' is the compact inline form on a matchup card; 'row' is the roomier
  // form for the per-pool list on the subject view.
  variant: { type: String, default: 'card' },
  // The pool to vote in. Omit on a matchup card (defaults to the active pool
  // via poolQuery()); on the subject view each row passes its own pool, e.g.
  // { country: 'France' } or { region: 'Europe' }.
  poolScope: { type: Object, default: null },
})

// Local, optimistic copy of the tally so the buttons feel instant; reset
// whenever a fresh matchup hands us a new belonging object.
const state = reactive({ confirms: 0, infirms: 0, verdict: '' })
watch(
  () => props.belonging,
  (b) => {
    state.confirms = b?.confirms ?? 0
    state.infirms = b?.infirms ?? 0
    state.verdict = b?.viewerVerdict ?? ''
  },
  { immediate: true },
)

const busy = ref(false)
const error = ref('')

async function send(target) {
  if (busy.value) return
  // Clicking the active verdict again retracts it (toggle off).
  const verdict = state.verdict === target ? 'none' : target
  busy.value = true
  error.value = ''
  try {
    const opts = props.poolScope || poolQuery()
    const res = await api.membership(props.subjectId, verdict, opts)
    state.confirms = res.confirms
    state.infirms = res.infirms
    state.verdict = res.viewerVerdict ?? ''
  } catch (e) {
    error.value =
      e.code === 'rate_limited'
        ? 'Slow down a moment.'
        : 'Could not save — try again.'
  } finally {
    busy.value = false
  }
}

const tally = computed(() => {
  const c = state.confirms
  const i = state.infirms
  if (!c && !i) return ''
  return `${c} confirmed · ${i} disputed`
})
</script>

<template>
  <div v-if="belonging" class="membership" :class="`is-${variant}`">
    <p class="why">
      <span class="why-label">Why here?</span>
      <span class="why-reason">{{ belonging.reason }}</span>
    </p>
    <div class="judge" role="group" aria-label="Does this person belong here?">
      <button
        type="button"
        class="vote confirm"
        :class="{ active: state.verdict === 'confirm' }"
        :disabled="busy"
        :aria-pressed="state.verdict === 'confirm'"
        @click="send('confirm')"
      >✓ Belongs</button>
      <button
        type="button"
        class="vote infirm"
        :class="{ active: state.verdict === 'infirm' }"
        :disabled="busy"
        :aria-pressed="state.verdict === 'infirm'"
        @click="send('infirm')"
      >✗ Doesn’t</button>
    </div>
    <p v-if="tally" class="tally">{{ tally }}</p>
    <p v-if="error" class="err" role="alert">{{ error }}</p>
  </div>
</template>

<style scoped>
.membership {
  margin-top: 0.6rem;
  padding-top: 0.6rem;
  border-top: 1px solid var(--hairline, rgba(0, 0, 0, 0.1));
  font-size: 0.85rem;
}
.why {
  margin: 0 0 0.4rem;
  color: var(--muted, #667);
  line-height: 1.3;
}
.why-label {
  font-weight: 600;
  margin-right: 0.35rem;
}
.judge {
  display: flex;
  gap: 0.4rem;
}
.vote {
  flex: 1;
  padding: 0.3rem 0.5rem;
  border: 1px solid var(--hairline, rgba(0, 0, 0, 0.18));
  border-radius: 0.5rem;
  background: transparent;
  cursor: pointer;
  font: inherit;
  font-size: 0.82rem;
}
.vote:disabled {
  opacity: 0.5;
  cursor: progress;
}
.vote.confirm.active {
  background: rgba(45, 160, 120, 0.16);
  border-color: rgba(45, 160, 120, 0.6);
}
.vote.infirm.active {
  background: rgba(200, 90, 70, 0.16);
  border-color: rgba(200, 90, 70, 0.6);
}
.tally {
  margin: 0.35rem 0 0;
  color: var(--muted, #889);
  font-size: 0.78rem;
}
.err {
  margin: 0.35rem 0 0;
  color: #c0392b;
  font-size: 0.78rem;
}
.is-row {
  border-top: none;
  padding-top: 0;
}
</style>
