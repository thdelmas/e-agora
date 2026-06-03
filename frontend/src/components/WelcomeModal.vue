<script setup>
// Shown once, right after the humanity check passes and before the agora
// opens: a short welcome, the anonymity promise, and the one-time home-region
// question (docs/10 §4). The region is pre-highlighted from the browser's
// volunteered language — never IP (docs/10 §6) — so most visitors just
// confirm. Both the acknowledgement and the chosen region are stored locally,
// never sent to the server beyond the per-request query flag, so a returning
// visitor isn't re-asked.
import { ref } from 'vue'
import { REGIONS, suggestRegion, setHomeRegion } from '../store'

const emit = defineEmits(['enter'])

const selected = ref(suggestRegion())
const regionLabel = (r) => r || '🌍 Whole world'

function enter() {
  setHomeRegion(selected.value)
  emit('enter')
}
</script>

<template>
  <div class="modal-backdrop">
    <div
      class="modal welcome"
      role="dialog"
      aria-modal="true"
      aria-label="Welcome to e-agora"
    >
      <h2>Welcome to the agora</h2>
      <p class="muted">
        Two public figures enter — you pick who you'd rather have as a leader.
        Thousands of these one-on-one votes pool into a single, ever-shifting
        ranking — shaped by people, not editors.
      </p>

      <p class="privacy-note">
        🔒 <strong>Your votes are anonymous.</strong> We record only your
        choices — never your identity. No account, no email, no name, no
        IP-address or device tracking. There's nothing to sign up for and
        nothing that can be traced back to you: just your picks, pooled with
        everyone else's.
      </p>

      <div class="region-step">
        <p class="region-q">Where do you follow politics most closely?</p>
        <div
          class="region-chips"
          role="group"
          aria-label="Choose your home region"
        >
          <button
            v-for="r in REGIONS"
            :key="r"
            type="button"
            class="region-chip"
            :class="{ active: selected === r }"
            :aria-pressed="selected === r"
            @click="selected = r"
          >
            {{ regionLabel(r) }}
          </button>
        </div>
        <p class="region-hint muted">
          We'll lean your matchups toward people you're likelier to know —
          without ever hiding the rest of the world.
        </p>
      </div>

      <button class="enter-btn" @click="enter">Enter the agora →</button>
    </div>
  </div>
</template>

<style scoped>
.region-step {
  margin-top: 1.3rem;
  text-align: left;
}
.region-q {
  margin: 0 0 0.6rem;
  font-weight: 700;
  color: var(--ink);
}
.region-chips {
  display: flex;
  flex-wrap: wrap;
  gap: 0.45rem;
}
.region-chip {
  font: inherit;
  font-size: 0.9rem;
  padding: 0.4rem 0.8rem;
  border-radius: 999px;
  border: 1px solid var(--border);
  background: var(--card);
  color: var(--ink-soft);
  cursor: pointer;
  transition: border-color 0.16s, background 0.16s, color 0.16s,
    box-shadow 0.16s;
}
.region-chip:hover {
  border-color: var(--laurel);
}
.region-chip.active {
  background: linear-gradient(135deg, var(--laurel), var(--laurel-deep));
  border-color: var(--laurel-deep);
  color: #fff;
  box-shadow: 0 4px 12px rgba(31, 110, 76, 0.24);
}
.region-hint {
  margin: 0.6rem 0 0;
  font-size: 0.82rem;
}
</style>
