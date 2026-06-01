-- 0004_add_extract.sql — store the Wikipedia lead paragraph per translation.
-- The matchup card showed only the one-line short description, so visitors faced
-- two people they didn't know with too little to form a preference. The REST
-- summary already returns the lead `extract` (we were discarding it); persist it
-- so the card can surface a few sentences inline (docs/06-wikipedia-ingestion.md
-- §Step 2). Nullable: existing rows backfill on the next EAGORA_SEED=force (en)
-- or lazily as languages are first requested.

ALTER TABLE subject_translations ADD COLUMN IF NOT EXISTS extract TEXT;
