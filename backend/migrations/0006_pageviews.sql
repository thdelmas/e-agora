-- 0006_pageviews.sql — per-language Wikipedia pageviews, the recognition signal
-- (docs/10-recognition-and-pools.md). The board only measured *global* notability
-- (sitelink count), but recognition is *local*: a visitor recognizes the figures
-- read in their own language, plus a handful of globally-famous names. Pageviews
-- per language capture that local attention. It is a fact about the *article*,
-- never the visitor — the no-profiling stance (05/09) is intact.
--
-- subject_pageviews holds a trailing-window view count per (subject, language),
-- refreshed by the sync over a served-languages set. global_views denormalizes
-- the cross-language sum so the matchup draw and the fame-tier pool can weight by
-- worldwide attention without a per-row aggregation.

CREATE TABLE subject_pageviews (
  subject_id BIGINT NOT NULL REFERENCES subjects(id) ON DELETE CASCADE,
  lang       TEXT   NOT NULL,
  views      BIGINT NOT NULL DEFAULT 0,
  window_end DATE   NOT NULL DEFAULT CURRENT_DATE,
  PRIMARY KEY (subject_id, lang)
);
-- Per-language scans (per-pool leaderboards, M12) and the matchup join read by
-- language; the PK (subject_id, lang) already serves the per-subject join.
CREATE INDEX idx_pageviews_lang ON subject_pageviews(lang, subject_id);

-- Sum of subject_pageviews across languages; the global-fame lever (β) and the
-- "premier league" fame-tier pool. Maintained by store.RefreshGlobalViews after
-- each sync. Defaults to 0 so a subject ingested before its pageviews are fetched
-- still draws (the recognition score falls back to its sitelink count).
ALTER TABLE subjects ADD COLUMN global_views BIGINT NOT NULL DEFAULT 0;
CREATE INDEX idx_subjects_fame ON subjects(global_views DESC) WHERE active;
