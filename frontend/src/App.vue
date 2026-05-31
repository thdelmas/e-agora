<script setup>
import SiteFooter from './components/SiteFooter.vue'
</script>

<template>
  <div class="app">
    <header class="topbar">
      <RouterLink to="/" class="wordmark">e-agora</RouterLink>
      <span class="tagline">decided one head-to-head at a time</span>
    </header>

    <main class="content">
      <RouterView />
    </main>

    <SiteFooter />
  </div>
</template>
