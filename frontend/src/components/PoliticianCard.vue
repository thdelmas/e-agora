<script setup>
import { ref } from 'vue'
// One subject in a matchup. Rating is intentionally absent so the visitor isn't
// biased before choosing (docs/04-api.md §Resource shapes). `side` ('a' | 'b')
// tints the card for the head-to-head vote — teal vs amber.
// The Wikipedia lead paragraph (`extract`) is shown inline, clamped, so a visitor
// can form an opinion on someone they don't know without leaving to read the page.
defineProps({
  subject: { type: Object, required: true },
  side: { type: String, default: 'a' },
})
defineEmits(['prefer'])

const expanded = ref(false)
</script>

<template>
  <article class="card" :class="`side-${side}`">
    <span class="side-tag" aria-hidden="true">{{ side.toUpperCase() }}</span>

    <img v-if="subject.imageUrl" :src="subject.imageUrl" :alt="subject.name" class="portrait" />
    <div v-else class="portrait placeholder" aria-hidden="true"></div>

    <h2 class="name">{{ subject.name }}</h2>
    <p v-if="subject.deceased" class="deceased-note">✝ Deceased<template v-if="subject.diedYear"> ({{ subject.diedYear }})</template></p>
    <p v-if="subject.description" class="desc">{{ subject.description }}</p>

    <template v-if="subject.extract">
      <p class="lead" :class="{ clamped: !expanded }">{{ subject.extract }}</p>
      <button class="more" type="button" :aria-expanded="expanded" @click="expanded = !expanded">
        {{ expanded ? 'Show less' : 'Show more' }}
      </button>
    </template>

    <a :href="subject.wikipediaUrl" target="_blank" rel="noopener" class="wiki"
       :aria-label="`Read about ${subject.name} on Wikipedia (opens in a new tab)`">
      <span class="wiki-mark" aria-hidden="true">W</span>
      <span class="wiki-label">Read on Wikipedia</span>
      <span class="wiki-arrow" aria-hidden="true">↗</span>
    </a>
    <button class="prefer" @click="$emit('prefer', subject.id)">Prefer {{ subject.name.split(' ')[0] }}</button>
  </article>
</template>
