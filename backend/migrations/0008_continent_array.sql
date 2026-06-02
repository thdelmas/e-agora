-- 0008_continent_array.sql — a figure's country can straddle more than one
-- continent (Russia → Europe+Asia, Egypt → Africa+Asia, Türkiye, …), and the
-- region pool (docs/10-recognition-and-pools.md §4) should place such figures in
-- EVERY continent they belong to, not just one. Widen continent from a single
-- label to a set: existing scalar values become one-element arrays; NULL (no
-- resolvable region) stays NULL. Only the contiguous transcontinental states get
-- multiple entries — a country's P30 also names its *overseas territories'*
-- continents, so the resolver keeps just the primary for everyone else (ingest
-- geo.go), and a re-resolve (the startup geo backfill) fills the second continent
-- for the transcontinental few once this column can hold it.
ALTER TABLE subjects
	ALTER COLUMN continent TYPE TEXT[]
	USING (CASE WHEN continent IS NULL THEN NULL ELSE ARRAY[continent] END);

-- The region filter becomes a containment test (continent @> ARRAY[region]); GIN
-- indexes that, where the prior btree indexed only the scalar.
DROP INDEX IF EXISTS idx_subjects_continent;
CREATE INDEX idx_subjects_continent ON subjects USING GIN (continent) WHERE active;
