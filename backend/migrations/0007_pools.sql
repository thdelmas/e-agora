-- 0007_pools.sql — country + continent for the visitor-selectable *region pool*
-- (docs/10-recognition-and-pools.md §4). This re-adds a country column that
-- 0003 dropped, but for a different purpose: 0003 removed a per-figure country
-- *label/stat* ("we rank figures regardless of country"); this is a scoping axis
-- so a visitor can choose to vote within "European leaders" or "African leaders"
-- — the leaders are still ranked on one global scale, the pool just filters which
-- contests are drawn. Both are Wikidata-derived at sync (country = P27 country of
-- citizenship; continent = that country's P30), best-effort and nullable: a
-- subject with no resolved country simply doesn't match a region pool.

ALTER TABLE subjects ADD COLUMN country   TEXT;
ALTER TABLE subjects ADD COLUMN continent TEXT;
CREATE INDEX idx_subjects_continent ON subjects(continent) WHERE active;
