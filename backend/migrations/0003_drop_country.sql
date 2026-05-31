-- 0003_drop_country.sql — drop the subjects.country column.
-- e-agora ranks political figures regardless of which country they belong to,
-- so the per-figure country label (and the derived "countries represented"
-- stat) is removed. No index referenced it; the drop is unconditional.

ALTER TABLE subjects DROP COLUMN IF EXISTS country;
