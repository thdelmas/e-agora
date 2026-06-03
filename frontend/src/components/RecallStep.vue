<script setup>
// The belonging recall step (docs/11 §2): on entering a pool, ask the visitor who
// comes to mind here. Whatever they name is a *proposal* — the crowd-recall signal
// that decides who belongs in a pool, separate from the rating. Presentational:
// it searches existing figures and emits the pick (or a skip); the host view does
// the gated POST and the humanity check, reusing the matchup's flow.
import { ref, watch } from 'vue'
import { api } from '../api/client'

defineProps({ label: { type: String, required: true } })
const emit = defineEmits(['propose', 'skip'])

const query = ref('')
const results = ref([])
const searching = ref(false)
let timer

watch(query, (q) => {
  clearTimeout(timer)
  const term = q.trim()
  if (!term) {
    results.value = []
    return
  }
  // Debounce so a fast typist makes one request, not one per keystroke.
  timer = setTimeout(async () => {
    searching.value = true
    try {
      const res = await api.recall(term)
      results.value = res.results || []
    } catch {
      results.value = [] // reference lookup; a failure just shows no matches
    } finally {
      searching.value = false
    }
  }, 200)
})
</script>

<template>
  <div class="recall" role="group" aria-label="Who comes to mind">
    <h2 class="recall-q">Who comes to mind for <em>{{ label }}</em>?</h2>
    <p class="recall-hint muted">Name anyone you associate with this pool — it teaches the agora who belongs here.</p>

    <input
      v-model="query"
      class="recall-input"
      type="text"
      placeholder="Start typing a name…"
      autocomplete="off"
      aria-label="Search for a person"
    />

    <ul v-if="results.length" class="recall-results">
      <li v-for="r in results" :key="r.id">
        <button type="button" @click="emit('propose', r.id)">{{ r.name }}</button>
      </li>
    </ul>
    <p v-else-if="query.trim() && !searching" class="muted recall-empty">No match yet — keep typing.</p>

    <button type="button" class="ghost recall-skip" @click="emit('skip')">I don’t know anyone here</button>
  </div>
</template>

<style scoped>
.recall {
  max-width: 28rem;
  margin: 1rem auto;
  text-align: center;
}
.recall-q {
  font-size: 1.3rem;
  margin-bottom: 0.25rem;
}
.recall-hint {
  margin: 0 0 0.5rem;
}
.recall-input {
  width: 100%;
  padding: 0.5rem 0.75rem;
  border-radius: 8px;
  font-size: 1rem;
}
.recall-results {
  list-style: none;
  padding: 0;
  margin: 0.5rem 0 0;
  display: flex;
  flex-direction: column;
  gap: 0.3rem;
}
.recall-results button {
  width: 100%;
  padding: 0.55rem 0.75rem;
  border-radius: 6px;
  cursor: pointer;
  text-align: left;
}
.recall-empty {
  margin: 0.5rem 0 0;
}
.recall-skip {
  margin-top: 0.75rem;
}
</style>
