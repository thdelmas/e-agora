<script setup>
// One subject in a matchup. Rating is intentionally absent so the visitor isn't
// biased before choosing (docs/04-api.md §Resource shapes).
defineProps({ subject: { type: Object, required: true } })
defineEmits(['prefer'])
</script>

<template>
  <article class="card">
    <img v-if="subject.imageUrl" :src="subject.imageUrl" :alt="subject.name" class="portrait" />
    <div v-else class="portrait placeholder" aria-hidden="true"></div>

    <h2 class="name">{{ subject.name }}</h2>
    <p v-if="subject.description" class="desc">{{ subject.description }}</p>
    <p v-if="subject.country" class="country">{{ subject.country }}</p>

    <a :href="subject.wikipediaUrl" target="_blank" rel="noopener" class="wiki">Wikipedia ↗</a>
    <button class="prefer" @click="$emit('prefer', subject.id)">Prefer</button>
  </article>
</template>
