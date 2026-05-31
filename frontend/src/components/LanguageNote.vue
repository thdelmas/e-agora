<script setup>
// "Shown in English" fallback notice when R9 dropped the visitor's language
// because one subject lacked it (docs/01-functional-spec.md §S1).
defineProps({ lang: { type: String, default: 'en' } })
</script>

<template>
  <p class="language-note muted">
    Shown in English — one of these isn't available in your language.
  </p>
</template>
