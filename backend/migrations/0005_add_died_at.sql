-- 0005_add_died_at.sql — record each person's date of death so the ranking can
-- filter the deceased. The pool mixes living and historical figures; by default
-- the leaderboard and matchup pool show only the living (died_at IS NULL), with a
-- viewer toggle that opts the dead back in to compare them against the living
-- (docs/05-ranking.md §Filtering the deceased). The source is Wikidata P570 (date
-- of death), read at ingest alongside P31 (docs/06-wikipedia-ingestion.md §Step 2).
-- Nullable: existing rows backfill on the next EAGORA_SEED=force, which re-ingests
-- every subject and now fills died_at (ratings/votes preserved by the upsert).

ALTER TABLE subjects ADD COLUMN IF NOT EXISTS died_at DATE;
