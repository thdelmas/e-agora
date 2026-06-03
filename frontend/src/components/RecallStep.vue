<script setup>
// The belonging recall step (docs/11 §2–§3): on entering a pool, ask the visitor
// who comes to mind here. The type-ahead searches figures already in the agora
// *and* Wikipedia, so a name we haven't seeded yet still resolves — picking a
// Wikipedia result ingests them on first recall. Presentational: it emits the
// pick (a subjectId for an existing figure, or a Wikipedia url for a new one),
// or a skip; the host view does the gated POST.
import { ref, watch } from 'vue'
import { api } from '../api/client'

defineProps({
  label: { type: String, required: true },
  busy: { type: Boolean, default: false },
})
const emit = defineEmits(['propose', 'skip'])

const query = ref('')
const items = ref([]) // { kind: 'existing'|'wiki', name, description?, url?, id? }
const searching = ref(false)
let timer
let seq = 0

watch(query, (q) => {
  clearTimeout(timer)
  const term = q.trim()
  if (!term) {
    items.value = []
    return
  }
  // Debounce so a fast typist makes one round of requests, not one per keystroke.
  timer = setTimeout(() => runSearch(term), 220)
})

async function runSearch(term) {
  const mine = ++seq
  searching.value = true
  // Existing figures and Wikipedia candidates in parallel; ignore a stale response
  // if the visitor kept typing.
  const [dbRes, wikiRes] = await Promise.allSettled([api.recall(term), api.searchSubjects(term)])
  if (mine !== seq) return
  const existing = (dbRes.value?.results || []).map((r) => ({ kind: 'existing', id: r.id, name: r.name }))
  const seen = new Set(existing.map((e) => e.name.toLowerCase()))
  const wiki = (wikiRes.value?.results || [])
    .filter((r) => r.title && !seen.has(r.title.toLowerCase()))
    .map((r) => ({ kind: 'wiki', name: r.title, description: r.description, url: r.wikipediaUrl }))
  items.value = [...existing, ...wiki].slice(0, 8)
  searching.value = false
}

function pick(it) {
  emit('propose', it.kind === 'existing' ? { subjectId: it.id } : { url: it.url })
}
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
      :disabled="busy"
      aria-label="Search for a person"
    />

    <ul v-if="items.length" class="recall-results">
      <li v-for="it in items" :key="it.kind + (it.id || it.url)">
        <button type="button" :disabled="busy" @click="pick(it)">
          <span class="r-name">{{ it.name }}</span>
          <span v-if="it.kind === 'wiki'" class="r-tag">＋ from Wikipedia</span>
          <span v-if="it.description" class="r-desc muted">{{ it.description }}</span>
        </button>
      </li>
    </ul>
    <p v-else-if="query.trim() && !searching" class="muted recall-empty">No match yet — keep typing.</p>

    <button type="button" class="ghost recall-skip" :disabled="busy" @click="emit('skip')">
      I don’t know anyone here
    </button>
  </div>
</template>

<style scoped>
.recall {
  max-width: 30rem;
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
  padding: 0.5rem 0.75rem;
  border-radius: 6px;
  cursor: pointer;
  text-align: left;
  display: flex;
  flex-direction: column;
  gap: 0.1rem;
}
.recall-results button:disabled {
  cursor: default;
  opacity: 0.6;
}
.r-name {
  font-weight: 600;
}
.r-tag {
  font-size: 0.75rem;
  opacity: 0.7;
}
.r-desc {
  font-size: 0.8rem;
}
.recall-empty {
  margin: 0.5rem 0 0;
}
.recall-skip {
  margin-top: 0.75rem;
}
</style>
