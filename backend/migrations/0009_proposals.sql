-- 0009_proposals.sql — the belonging axis (docs/11-belonging-and-proposals.md).
-- Pool membership is *inferred* from Wikidata geography today (P27→P30), which is
-- thin per country and geopolitically surprising at the boundaries (Türkiye lands
-- in both Europe and Asia). Measure it instead: on entering a pool a visitor is
-- asked, recall-framed, "who comes to mind here?" — each answer is one proposal.
-- Recall frequency per (pool, subject) is a *belonging* score, a separate axis
-- from the Glicko rating: belonging decides who's *in* a pool, the rating ranks
-- them.
--
-- A pool is a computed predicate, not a row, so proposals key on a canonical pool
-- *key* string (docs/11 §5): 'world' | 'continent:Europe' | 'country:France'. The
-- key is the geographic *scope* only — fame/status are view filters layered on
-- top, not membership.

-- Append-only recall log (mirrors votes): one row per proposal, the raw signal.
CREATE TABLE proposals (
  id          BIGINT GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
  session_id  TEXT   NOT NULL REFERENCES sessions(id),
  pool_key    TEXT   NOT NULL,
  subject_id  BIGINT NOT NULL REFERENCES subjects(id),
  created_at  TIMESTAMPTZ NOT NULL DEFAULT now()
);
CREATE INDEX idx_proposals_pool    ON proposals(pool_key);
CREATE INDEX idx_proposals_session ON proposals(session_id);

-- Maintained aggregate: recall count per (pool, subject) — the belonging
-- numerator. Derivable from proposals, materialized so the draw and per-pool
-- leaderboard read it cheaply (like the rating counters on subjects).
CREATE TABLE pool_belonging (
  pool_key    TEXT    NOT NULL,
  subject_id  BIGINT  NOT NULL REFERENCES subjects(id),
  proposals   INTEGER NOT NULL DEFAULT 0,
  updated_at  TIMESTAMPTZ NOT NULL DEFAULT now(),
  PRIMARY KEY (pool_key, subject_id)
);

-- Per-pool denominator: total proposals the pool has seen, so belonging is a
-- *share* (smoothed) rather than a raw count (docs/11 §4).
CREATE TABLE pool_stats (
  pool_key    TEXT    PRIMARY KEY,
  proposals   INTEGER NOT NULL DEFAULT 0,
  updated_at  TIMESTAMPTZ NOT NULL DEFAULT now()
);
