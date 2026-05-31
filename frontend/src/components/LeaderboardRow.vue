<script setup>
import { computed } from 'vue'

const props = defineProps({ entry: { type: Object, required: true } })

// Percentage of a subject's votes that were wins/losses. comparisons == wins +
// losses (every vote bumps one of them), so it doubles as the vote count.
function pct(part, total) {
  return total > 0 ? Math.round((part / total) * 100) : 0
}

const winPct = computed(() => pct(props.entry.wins, props.entry.comparisons))
const medal = computed(() => ({ 1: '🥇', 2: '🥈', 3: '🥉' }[props.entry.rank] || null))

// Glicko-2 rating deviation: how unsure we still are of this rating. A high RD
// means few/erratic votes, so the rank is provisional — flag it rather than
// pretend the number is settled. ~110 is the usual "established" cutoff.
const rd = computed(() => Math.round(props.entry.ratingDeviation || 0))
const provisional = computed(() => rd.value > 110)
</script>

<template>
  <li class="row" :class="entry.rank <= 3 ? ['top', `top-${entry.rank}`] : []">
    <span class="rank">
      <span v-if="medal" class="medal" aria-hidden="true">{{ medal }}</span>
      <template v-else>{{ entry.rank }}</template>
    </span>

    <img v-if="entry.subject.imageUrl" :src="entry.subject.imageUrl" :alt="entry.subject.name" class="thumb" />
    <span v-else class="thumb" aria-hidden="true"></span>

    <span>
      <span class="name">{{ entry.subject.name }}</span>
    </span>

    <span class="winbar">
      <span class="winbar-track" :title="`${winPct}% win rate`">
        <span class="winbar-fill" :style="{ width: winPct + '%' }"></span>
      </span>
      <span class="winbar-label">{{ winPct }}% won · {{ entry.comparisons }} votes</span>
    </span>

    <span class="rating" :class="{ provisional }"
          :title="`Glicko-2 rating ${Math.round(entry.rating)} ± ${rd}${provisional ? ' · provisional (needs more votes)' : ''}`">
      <span class="rating-val">{{ Math.round(entry.rating) }}</span>
      <span class="rating-unit">±{{ rd }}{{ provisional ? ' ?' : '' }}</span>
    </span>
  </li>
</template>
