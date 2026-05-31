-- 0002_glicko2.sql — migrate the rating engine from Elo to Glicko-2
-- (docs/05-ranking.md). Glicko-2 tracks two extra numbers per subject: the
-- rating deviation (RD, our uncertainty about the rating) and the volatility
-- (how erratic results have been). The `rating` column is reused unchanged.

ALTER TABLE subjects
  ADD COLUMN rd         DOUBLE PRECISION NOT NULL DEFAULT 350,
  ADD COLUMN volatility DOUBLE PRECISION NOT NULL DEFAULT 0.06;

-- Existing ratings carry information, so don't reset already-voted subjects to
-- full uncertainty (which would let one fresh vote swing a 70-duel veteran like
-- a newcomer). Give them a tighter starting RD that shrinks with the evidence
-- they already have. A one-time heuristic; the live model refines RD per vote
-- from here on. Brand-new subjects (comparisons = 0) keep the 350 default.
UPDATE subjects
   SET rd = GREATEST(50, 350 - 25 * LEAST(comparisons, 12))
 WHERE comparisons > 0;

-- Leaderboard now ranks by the conservative rating (rating − 2·RD): a subject
-- only climbs once its rating is both high AND well-established, so a lucky
-- 3-vote run can't top the board (docs/05-ranking.md §Leaderboard ordering).
DROP INDEX IF EXISTS idx_subjects_board;
CREATE INDEX idx_subjects_board ON subjects ((rating - 2 * rd) DESC) WHERE active;

-- Snapshot the full pre-vote state in the audit log so ratings remain
-- replayable (docs/03-data-model.md §votes). Nullable: rows written before this
-- migration predate Glicko-2 and legitimately have no RD/volatility.
ALTER TABLE votes
  ADD COLUMN winner_rd_before  DOUBLE PRECISION,
  ADD COLUMN loser_rd_before   DOUBLE PRECISION,
  ADD COLUMN winner_vol_before DOUBLE PRECISION,
  ADD COLUMN loser_vol_before  DOUBLE PRECISION;
