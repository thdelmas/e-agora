<script setup>
// One subject in a matchup. Rating is intentionally absent so the visitor isn't
// biased before choosing (docs/04-api.md §Resource shapes). `side` ('a' | 'b')
// tints the card for the head-to-head vote — teal vs amber.
defineProps({
  subject: { type: Object, required: true },
  side: { type: String, default: 'a' },
})
defineEmits(['prefer'])
</script>

<template>
  <article class="card" :class="`side-${side}`">
    <span class="side-tag" aria-hidden="true">{{ side.toUpperCase() }}</span>

    <img v-if="subject.imageUrl" :src="subject.imageUrl" :alt="subject.name" class="portrait" />
    <div v-else class="portrait placeholder" aria-hidden="true"></div>

    <h2 class="name">{{ subject.name }}</h2>
    <p v-if="subject.description" class="desc">{{ subject.description }}</p>

    <a :href="subject.wikipediaUrl" target="_blank" rel="noopener" class="wiki">Read on Wikipedia ↗</a>
    <button class="prefer" @click="$emit('prefer', subject.id)">Prefer {{ subject.name.split(' ')[0] }}</button>
  </article>
</template>
