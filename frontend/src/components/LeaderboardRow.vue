<script setup>
defineProps({ entry: { type: Object, required: true } })
</script>

<template>
  <li class="row">
    <span class="rank">{{ entry.rank }}</span>
    <img v-if="entry.subject.imageUrl" :src="entry.subject.imageUrl" :alt="entry.subject.name" class="thumb" />
    <span class="name">{{ entry.subject.name }}</span>
    <span class="country">{{ entry.subject.country }}</span>
    <span class="rating">{{ Math.round(entry.rating) }}</span>
    <span class="record">{{ entry.wins }}–{{ entry.losses }}</span>
  </li>
</template>
