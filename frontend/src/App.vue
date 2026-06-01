<script setup>
import { computed, onMounted, ref } from 'vue'
import { useRouter, useRoute } from 'vue-router'
import SiteFooter from './components/SiteFooter.vue'
import AddSubjectModal from './components/AddSubjectModal.vue'
import HumanityCheckModal from './components/HumanityCheckModal.vue'
import WelcomeModal from './components/WelcomeModal.vue'
import { me, refreshMe, welcomeSeen, acknowledgeWelcome } from './store'

const router = useRouter()
const route = useRoute()
const showAdd = ref(false)

// Until a visitor casts their first vote they have no access: every nav action
// (Stats, Add someone, Rankings) wears the lock and nudges them back to vote.
const lockedTo = { path: '/', query: { locked: '1' } }

// The public stats page is open to everyone — it must render without the
// humanity check or welcome step (those gate the voting arena, not a page of
// anonymous aggregate numbers). Matches the ungated /api/stats endpoint.
const isPublicPage = computed(() => route.name === 'stats')

onMounted(refreshMe)

// Onboarding gate sequence: land → humanity check → welcome + privacy → agora.
// We wait for `me` to load so nothing flashes before the visitor's status is
// known.
//
// R12 — the humanity check gates entry to the agora itself, not just voting: an
// unverified visitor meets it first and reaches nothing else until they pass.
const needsHuman = computed(() => me.loaded && !me.humanVerified)
// Then the one-time welcome + anonymity note, before the agora opens.
const needsWelcome = computed(() => me.loaded && !needsHuman.value && !welcomeSeen.value)

function onHumanVerified() {
  refreshMe()
}

const addTitle = computed(() => {
  if (!me.hasAccess) return 'Vote once to unlock adding'
  if (!me.canAdd) return "You've already added someone in this 24h window"
  return 'Add a person with a Wikipedia page'
})

function onAdd() {
  if (!me.hasAccess) { router.push(lockedTo); return }
  showAdd.value = true
}

function openLeaderboard() {
  router.push(me.hasAccess ? '/leaderboard' : lockedTo)
}
</script>

<template>
  <div class="app">
    <header class="topbar">
      <RouterLink to="/" class="wordmark">e-agora</RouterLink>
      <span class="tagline">where the people decide, one vote at a time</span>

      <nav class="nav">
        <!-- The vote arena is the one destination that's never gated — it's how
             a visitor earns access — so Vote stays a plain link with no lock. -->
        <RouterLink to="/" class="nav-vote" title="Back to the voting arena">Vote</RouterLink>
        <RouterLink :to="me.hasAccess ? '/stats' : lockedTo" class="nav-stats" :class="{ locked: !me.hasAccess }" :title="me.hasAccess ? 'Public statistics — anonymous & open to all' : 'Vote once to unlock the stats'">
          {{ me.hasAccess ? 'Stats' : 'Stats 🔒' }}
        </RouterLink>
        <button class="nav-add" :class="{ locked: !me.hasAccess }" :disabled="me.hasAccess && !me.canAdd" :title="addTitle" @click="onAdd">
          {{ me.hasAccess ? '+ Add someone' : '+ Add someone 🔒' }}
        </button>
        <button class="nav-board" :class="{ locked: !me.hasAccess, active: me.hasAccess && route.name === 'leaderboard' }" :title="me.hasAccess ? 'World rankings' : 'Vote once to unlock the rankings'" @click="openLeaderboard">
          {{ me.hasAccess ? 'Rankings' : 'Rankings 🔒' }}
        </button>
      </nav>
    </header>

    <main class="content">
      <RouterView v-if="isPublicPage || (me.loaded && !needsHuman && !needsWelcome)" />
    </main>

    <SiteFooter />

    <AddSubjectModal v-if="showAdd" @close="showAdd = false" />
    <HumanityCheckModal v-if="needsHuman && !isPublicPage" gate @verified="onHumanVerified" />
    <WelcomeModal v-else-if="needsWelcome && !isPublicPage" @enter="acknowledgeWelcome" />
  </div>
</template>
