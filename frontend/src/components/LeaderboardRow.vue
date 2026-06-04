<script setup>
import { computed } from 'vue'
import { poolQuery } from '../store'

const props = defineProps({ entry: { type: Object, required: true } })

// Clicking a row opens that figure's dedicated view. We carry the current pool
// flags so its shown rank matches this board (docs/04-api.md §GET /subjects).
const to = computed(() => ({
  name: 'subject',
  params: { id: props.entry.subject.id },
  query: poolQuery(),
}))

// Percentage of a subject's votes that were wins/losses. comparisons == wins +
// losses (every vote bumps one of them), so it doubles as the vote count.
function pct(part, total) {
  return total > 0 ? Math.round((part / total) * 100) : 0
}

const winPct = computed(() => pct(props.entry.wins, props.entry.comparisons))
const medal = computed(
  () => ({ 1: '🥇', 2: '🥈', 3: '🥉' }[props.entry.rank] || null),
)

// Glicko-2 numbers. RD is how unsure we still are of the rating: a high RD
// means few/erratic votes, so the rank is provisional — flag it rather than
// pretend the number is settled. ~110 is the usual "established" cutoff.
const rating = computed(() => Math.round(props.entry.rating))
const rd = computed(() => Math.round(props.entry.ratingDeviation || 0))
const provisional = computed(() => rd.value > 110)

// The board is sorted server-side by the conservative rating — rating minus
// twice the deviation — so a figure only climbs once its rating is both high
// AND well-established (backend/internal/store/leaderboard.go). Derive the
// headline from the SAME rounded rating and deviation shown beneath it, so the
// displayed numbers always reconcile (score = rating − 2 × rd) instead of
// being rounded independently and disagreeing by a point in the tooltip and
// subtitle.
// Row order comes from the backend rank, so a sub-point rounding tie here can
// never reorder the list.
const score = computed(() => rating.value - 2 * rd.value)
</script>

<template>
  <li class="row" :class="entry.rank <= 3 ? ['top', `top-${entry.rank}`] : []">
   <RouterLink
     :to="to"
     class="row-link"
     :aria-label="`View ${entry.subject.name}`"
   >
    <span class="rank">
      <span v-if="medal" class="medal" aria-hidden="true">{{ medal }}</span>
      <template v-else>{{ entry.rank }}</template>
    </span>

    <img
      v-if="entry.subject.imageUrl"
      :src="entry.subject.imageUrl"
      :alt="entry.subject.name"
      class="thumb"
    />
    <span v-else class="thumb" aria-hidden="true"></span>

    <span class="name-cell">
      <span class="name">{{ entry.subject.name }}</span>
      <span
        v-if="entry.subject.deceased"
        class="deceased"
        :title="entry.subject.diedYear
          ? `Deceased ${entry.subject.diedYear}`
          : 'Deceased'"
      >✝<template
          v-if="entry.subject.diedYear"
        > {{ entry.subject.diedYear }}</template></span>
    </span>

    <span class="winbar">
      <span class="winbar-track" :title="`${winPct}% win rate`">
        <span class="winbar-fill" :style="{ width: winPct + '%' }"></span>
      </span>
      <span class="winbar-label">
        {{ winPct }}% won · {{ entry.comparisons }} votes
      </span>
    </span>

    <span
      class="rating"
      :class="{ provisional }"
      :title="
        `Rank score ${score} = rating ${rating} − 2 × deviation ${rd}` +
        (provisional ? ' · provisional (needs more votes)' : '')
      "
    >
      <span class="rating-val">{{ score }}</span>
      <span class="rating-unit">
        rating {{ rating }} ±{{ rd }}{{ provisional ? ' ?' : '' }}
      </span>
    </span>
   </RouterLink>
  </li>
</template>
