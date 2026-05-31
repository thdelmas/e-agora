<script setup>
// Shown once, right after the humanity check passes and before the agora opens:
// a short welcome and the anonymity promise. Acknowledgement is stored locally
// (never sent to the server) so a returning visitor isn't shown it again.
defineEmits(['enter'])
</script>

<template>
  <div class="modal-backdrop">
    <div class="modal welcome" role="dialog" aria-modal="true" aria-label="Welcome to e-agora">
      <h2>Welcome to the agora</h2>
      <p class="muted">
        Two public figures enter — you pick who you'd rather have as a leader. Thousands of these
        one-on-one votes pool into a single, ever-shifting ranking — shaped by people,
        not editors.
      </p>

      <p class="privacy-note">
        🔒 <strong>Your votes are anonymous.</strong> We record only your choices —
        never your identity. No account, no email, no name, no IP-address or device
        tracking. There's nothing to sign up for and nothing that can be traced back
        to you: just your picks, pooled with everyone else's.
      </p>

      <button class="enter-btn" @click="$emit('enter')">Enter the agora →</button>
    </div>
  </div>
</template>
