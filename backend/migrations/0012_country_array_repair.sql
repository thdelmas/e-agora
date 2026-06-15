-- 0012_country_array_repair.sql — repair the country widening that 0010 was
-- meant to do but that never reached an already-deployed database.
--
-- 0010_country_array.sql was originally numbered 0009, colliding with
-- 0009_proposals.sql. The migration runner keys on the integer version
-- (schema_migrations PRIMARY KEY) and skips any version already recorded
-- (store/migrate.go), so on a database that had already recorded version 9 the
-- duplicate was silently skipped: subjects.country stayed scalar TEXT while the
-- deployed code (RandomPair, the leaderboard) switched to the array containment
-- operator country @> ARRAY[label]. That is a plan-time type error
-- ("operator does not exist: text @> text[]"), so every matchup draw 500s
-- regardless of filters. Renumbering 0010 fixed FRESH databases but cannot
-- retro-apply to one whose bookkeeping already reflects the skip — hence this
-- forward, self-numbered repair.
--
-- Idempotent and safe on BOTH states: it widens only if country is still
-- scalar TEXT (the broken DBs), and is a no-op where 0010 already ran (country
-- is TEXT[] — re-running 0010's ALTER there would itself fail, since
-- ARRAY[col] can't apply to a TEXT[] column). The runner hardening that lands
-- alongside this migration prevents the collision class going forward.
DO $$
BEGIN
	IF (
		SELECT atttypid
		FROM pg_attribute
		WHERE attrelid = 'subjects'::regclass
		  AND attname = 'country'
		  AND NOT attisdropped
	) = 'text'::regtype THEN
		ALTER TABLE subjects
			ALTER COLUMN country TYPE TEXT[]
			USING (CASE WHEN country IS NULL THEN NULL ELSE ARRAY[country] END);
	END IF;
END $$;

-- The GIN index 0010 creates over the array column (the leaderboard and matchup
-- country filters both use @>). IF NOT EXISTS so this is a no-op where 0010
-- already created it.
CREATE INDEX IF NOT EXISTS idx_subjects_country
	ON subjects USING GIN (country) WHERE active;
