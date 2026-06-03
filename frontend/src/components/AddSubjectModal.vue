<script setup>
// S3 — add a subject (R8/R8.1): paste a Wikipedia URL, or search a name and
// pick a result. The server validates it's a human with a page, dedupes, and
// consumes the token's one add allowance. Inline errors mirror the API codes.
import { onMounted, onUnmounted, ref, watch } from 'vue'
import { api } from '../api/client'
import { refreshMe } from '../store'

const emit = defineEmits(['close'])

function onKey(e) {
  if (e.key === 'Escape') emit('close')
}
onMounted(() => window.addEventListener('keydown', onKey))
onUnmounted(() => window.removeEventListener('keydown', onKey))

const query = ref('')
const results = ref([])
const searching = ref(false)
const adding = ref(false)
const message = ref('')
const messageType = ref('') // 'error' | 'success'
const done = ref(false)
// the subject we just inserted, shown as a confirmation card
const added = ref(null)

const urlRe = /^https?:\/\/[a-z-]+(\.m)?\.wikipedia\.org\/wiki\/.+/i
const isUrl = () => urlRe.test(query.value.trim())

let debounce
watch(query, () => {
  message.value = ''
  clearTimeout(debounce)
  if (isUrl() || query.value.trim().length < 2) {
    results.value = []
    return
  }
  debounce = setTimeout(runSearch, 300)
})

async function runSearch() {
  searching.value = true
  try {
    const r = await api.searchSubjects(query.value.trim())
    results.value = r.results || []
  } catch {
    results.value = []
  } finally {
    searching.value = false
  }
}

async function add(payload, label) {
  if (adding.value) return
  adding.value = true
  message.value = ''
  try {
    const r = await api.addSubject(payload)
    added.value = r.subject || { name: label }
    done.value = true
    await refreshMe()
  } catch (e) {
    messageType.value = 'error'
    message.value = addError(e)
  } finally {
    adding.value = false
  }
}

function addError(e) {
  switch (e.code) {
    case 'not_a_person':
      return 'e-agora is for people — that page isn’t a person.'
    case 'not_a_wikipedia_page':
      return 'We couldn’t find a Wikipedia page for that.'
    case 'already_exists':
      return 'They’re already in the agora.'
    case 'add_limit_reached':
      return 'You can add one person per 24 hours — vote again after your ' +
        'access renews.'
    case 'access_required':
    case 'access_expired':
      return 'Vote once to unlock adding.'
    case 'rate_limited':
      return 'Slow down a moment, then try again.'
    default:
      return e.message || 'Could not add that subject.'
  }
}
</script>

<template>
  <div class="modal-backdrop" @click.self="$emit('close')">
    <div
      class="modal"
      role="dialog"
      aria-modal="true"
      aria-label="Add a subject"
    >
      <h2>Add someone</h2>

      <template v-if="done">
        <p class="nudge success">They’re in the agora.</p>
        <div class="added-card">
          <img
            v-if="added?.imageUrl"
            :src="added.imageUrl"
            :alt="added.name"
            class="added-portrait"
          />
          <span
            v-else
            class="added-portrait placeholder"
            aria-hidden="true"
          ></span>
          <span class="added-text">
            <strong class="added-name">{{ added?.name }}</strong>
            <span v-if="added?.description" class="muted detail">
              {{ added.description }}
            </span>
            <a
              v-if="added?.wikipediaUrl"
              :href="added.wikipediaUrl"
              target="_blank"
              rel="noopener"
              class="wiki-btn"
            >Read on Wikipedia ↗</a>
          </span>
        </div>
        <p class="muted hint">
          {{ added?.name?.split(' ')[0] || 'They' }} will start appearing in
          matchups right away. New additions begin near the bottom of the
          rankings and climb as people vote on them.
        </p>
        <button class="prefer" @click="$emit('close')">Done</button>
      </template>

      <template v-else>
        <p class="muted">
          Paste a Wikipedia link, or search a name. Any person with a
          Wikipedia page is welcome.
        </p>
        <input
          v-model="query"
          class="add-input"
          type="text"
          placeholder="https://en.wikipedia.org/wiki/… or a name"
          autofocus
        />

        <p v-if="message" class="nudge" :class="messageType">{{ message }}</p>

        <button
          v-if="isUrl()"
          class="prefer"
          :disabled="adding"
          @click="add({ url: query.trim() })"
        >
          {{ adding ? 'Adding…' : 'Add this page' }}
        </button>

        <p v-else-if="searching" class="muted">Searching…</p>

        <ul v-else-if="results.length" class="search-results">
          <li v-for="r in results" :key="r.wikipediaUrl">
            <button
              class="result"
              :disabled="adding"
              @click="add({ url: r.wikipediaUrl }, r.title)"
            >
              <img
                v-if="r.imageUrl"
                :src="r.imageUrl"
                :alt="r.title"
                class="result-thumb"
              />
              <span
                class="result-thumb placeholder"
                v-else
                aria-hidden="true"
              ></span>
              <span class="result-text">
                <strong>{{ r.title }}</strong>
                <span class="muted detail">{{ r.description }}</span>
              </span>
            </button>
          </li>
        </ul>

        <button class="ghost" @click="$emit('close')">Cancel</button>
      </template>
    </div>
  </div>
</template>
