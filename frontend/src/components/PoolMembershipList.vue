<script setup>
import { onMounted, ref } from 'vue'
import { api } from '../api/client'
import MembershipControl from './MembershipControl.vue'

// The full "which pools is this figure in, and why" list for the subject detail
// view (docs/11 §7): one row per citizenship and per continent those span
// (docs/10 §4), each with its Wikidata reason and a confirm/infirm control.
// Drop into the subject view with <PoolMembershipList :subject-id="id" />.
const props = defineProps({
  subjectId: { type: Number, required: true },
})

const pools = ref([])
const loaded = ref(false)

onMounted(async () => {
  try {
    const res = await api.subjectPools(props.subjectId)
    pools.value = res.pools || []
  } catch {
    pools.value = []
  } finally {
    loaded.value = true
  }
})

// The subjectPools row carries scope ('country'|'continent') + label; the
// membership endpoint wants the same query shape the picker sends.
function scopeOf(pool) {
  return pool.scope === 'continent'
    ? { region: pool.label }
    : { country: pool.label }
}
</script>

<template>
  <section v-if="loaded && pools.length" class="pools">
    <h3 class="pools-title">Pools</h3>
    <ul class="pool-list">
      <li v-for="p in pools" :key="p.poolKey" class="pool-item">
        <div class="pool-head">
          <span class="pool-label">{{ p.label }}</span>
          <span class="pool-scope">{{ p.scope }}</span>
          <span v-if="p.excluded" class="pool-out" title="Removed by the crowd"
            >dropped</span
          >
        </div>
        <MembershipControl
          :subject-id="subjectId"
          :belonging="p"
          :pool-scope="scopeOf(p)"
          variant="row"
        />
      </li>
    </ul>
  </section>
</template>

<style scoped>
.pools {
  margin-top: 1.5rem;
}
.pools-title {
  font-size: 1rem;
  margin: 0 0 0.6rem;
}
.pool-list {
  list-style: none;
  margin: 0;
  padding: 0;
  display: grid;
  gap: 0.75rem;
}
.pool-item {
  border: 1px solid var(--hairline, rgba(0, 0, 0, 0.12));
  border-radius: 0.6rem;
  padding: 0.7rem 0.85rem;
}
.pool-head {
  display: flex;
  align-items: baseline;
  gap: 0.5rem;
}
.pool-label {
  font-weight: 600;
}
.pool-scope {
  font-size: 0.75rem;
  color: var(--muted, #778);
  text-transform: uppercase;
  letter-spacing: 0.03em;
}
.pool-out {
  margin-left: auto;
  font-size: 0.72rem;
  color: #c0392b;
  border: 1px solid currentColor;
  border-radius: 0.4rem;
  padding: 0.05rem 0.35rem;
}
</style>
