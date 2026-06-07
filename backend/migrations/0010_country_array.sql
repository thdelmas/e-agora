-- 0010_country_array.sql — a figure can hold more than one current citizenship
-- (P27), and the region pool (docs/10-recognition-and-pools.md §4) should place
-- such figures in EVERY country they're a citizen of, not just the first one
-- listed. Elon Musk is the motivating case: his P27 lists South Africa, Canada
-- and the United States, all rank-normal with no end-time, so document order
-- alone filed him under South Africa and he vanished from the US board. Widen
-- country from a single label to a set, exactly as 0008 did for continent:
-- existing scalar values become one-element arrays; NULL (no resolvable country)
-- stays NULL. The continent column already holds the union of every country's
-- continents (a re-resolve / the startup geo backfill fills the rest).
ALTER TABLE subjects
	ALTER COLUMN country TYPE TEXT[]
	USING (CASE WHEN country IS NULL THEN NULL ELSE ARRAY[country] END);

-- The country filter becomes a containment test (country @> ARRAY[label]); GIN
-- indexes that. There was no btree on the scalar country before, so this is a
-- net-new index — the leaderboard and matchup country filters both use @>.
CREATE INDEX idx_subjects_country ON subjects USING GIN (country) WHERE active;
