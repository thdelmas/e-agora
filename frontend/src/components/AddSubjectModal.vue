<script setup>
// S3 — add a subject (R8): paste a Wikipedia URL or search, validate, confirm.
// Gated by a valid token + unused add allowance. Wired in M5 — placeholder.
defineEmits(['close'])
</script>

<template>
  <div class="modal-backdrop" @click.self="$emit('close')">
    <div class="modal" role="dialog" aria-modal="true" aria-label="Add a subject">
      <h2>Add someone</h2>
      <p class="muted">Paste a Wikipedia URL or search a name. (Implemented in M5.)</p>
      <button class="ghost" @click="$emit('close')">Close</button>
    </div>
  </div>
</template>
