-- 0011_membership.sql — the membership-confidence axis on top of belonging
-- (docs/11-belonging-and-proposals.md §7). Geographic pool membership comes from
-- Wikidata P27 (docs/10 §4), trusted by default but imperfect: a stale colonial
-- citizenship that Wikidata never marked ended (Ismaïl Omar Guelleh still listed
-- as a French citizen) silently lands a figure in the wrong pool. Recall
-- ("who comes to mind here?") is a *positive-only* signal — it can surface a
-- missing member but can't argue one OUT. So we give the visitor a second, signed
-- judgment on a *shown* membership: confirm ("yes, belongs here") or infirm ("no,
-- doesn't"). This is kept SEPARATE from the recall-share belonging score: recall
-- measures who comes to mind, this measures whether a given geographic match is
-- real. It feeds a Beta posterior-mean confidence (see store/membership.go) that
-- gates the draw and, on strong consensus, removes the figure from the pool —
-- the global Glicko rating is never touched (belonging ≠ rating).
--
-- Keyed on the same canonical pool key as proposals (docs/11 §5):
-- 'continent:Europe' | 'country:France'. The 'world' pool is never voted on —
-- everyone belongs to the world.

-- One standing membership verdict per (session, pool, subject): a session can
-- change its mind (confirm → infirm) or retract (row deleted), so this is the
-- source of truth the aggregate is recomputed from. verdict: +1 confirm, -1
-- infirm.
CREATE TABLE membership_votes (
  id          BIGINT GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
  session_id  TEXT     NOT NULL REFERENCES sessions(id),
  pool_key    TEXT     NOT NULL,
  subject_id  BIGINT   NOT NULL REFERENCES subjects(id),
  verdict     SMALLINT NOT NULL CHECK (verdict IN (-1, 1)),
  created_at  TIMESTAMPTZ NOT NULL DEFAULT now(),
  updated_at  TIMESTAMPTZ NOT NULL DEFAULT now(),
  UNIQUE (session_id, pool_key, subject_id)
);
CREATE INDEX idx_membership_votes_pool ON membership_votes(pool_key);

-- Maintained aggregate: confirm / infirm tallies per (pool, subject), the inputs
-- to the membership confidence. Derivable from membership_votes, materialized so
-- the draw and leaderboard can gate/filter cheaply (like pool_belonging).
CREATE TABLE pool_membership (
  pool_key    TEXT    NOT NULL,
  subject_id  BIGINT  NOT NULL REFERENCES subjects(id),
  confirms    INTEGER NOT NULL DEFAULT 0,
  infirms     INTEGER NOT NULL DEFAULT 0,
  updated_at  TIMESTAMPTZ NOT NULL DEFAULT now(),
  PRIMARY KEY (pool_key, subject_id)
);
