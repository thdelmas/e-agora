-- 0001_init.sql — initial schema (docs/03-data-model.md).
-- Six tables: subjects, subject_translations, votes, sessions,
-- subject_add_log, and the schema_migrations bookkeeping table.

CREATE TABLE subjects (
  id              BIGINT  GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
  wikidata_id     TEXT    NOT NULL UNIQUE,
  canonical_name  TEXT    NOT NULL,
  country         TEXT,
  source          TEXT    NOT NULL DEFAULT 'seed' CHECK (source IN ('seed','user')),
  available_langs TEXT[]  NOT NULL DEFAULT '{}',
  rating          DOUBLE PRECISION NOT NULL DEFAULT 1500,
  wins            INTEGER NOT NULL DEFAULT 0,
  losses          INTEGER NOT NULL DEFAULT 0,
  comparisons     INTEGER NOT NULL DEFAULT 0,
  active          BOOLEAN NOT NULL DEFAULT TRUE,
  created_at      TIMESTAMPTZ NOT NULL DEFAULT now(),
  updated_at      TIMESTAMPTZ NOT NULL DEFAULT now()
);
CREATE INDEX idx_subjects_board    ON subjects(rating DESC) WHERE active;
CREATE INDEX idx_subjects_coverage ON subjects(comparisons) WHERE active;
CREATE INDEX idx_subjects_langs    ON subjects USING GIN (available_langs);

CREATE TABLE subject_translations (
  subject_id    BIGINT NOT NULL REFERENCES subjects(id) ON DELETE CASCADE,
  lang          TEXT   NOT NULL,
  name          TEXT   NOT NULL,
  description   TEXT,
  image_url     TEXT,
  wikipedia_url TEXT   NOT NULL,
  fetched_at    TIMESTAMPTZ NOT NULL DEFAULT now(),
  PRIMARY KEY (subject_id, lang)
);

CREATE TABLE sessions (
  id                    TEXT    PRIMARY KEY,
  contributions         INTEGER NOT NULL DEFAULT 0,
  human_verified_until  TIMESTAMPTZ,
  created_at            TIMESTAMPTZ NOT NULL DEFAULT now(),
  last_seen_at          TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE TABLE votes (
  id                    BIGINT GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
  session_id            TEXT   NOT NULL REFERENCES sessions(id),
  winner_id             BIGINT NOT NULL REFERENCES subjects(id),
  loser_id              BIGINT NOT NULL REFERENCES subjects(id),
  winner_rating_before  DOUBLE PRECISION NOT NULL,
  loser_rating_before   DOUBLE PRECISION NOT NULL,
  created_at            TIMESTAMPTZ NOT NULL DEFAULT now(),
  CHECK (winner_id <> loser_id)
);
CREATE INDEX idx_votes_session ON votes(session_id);
CREATE INDEX idx_votes_created ON votes(created_at);

CREATE TABLE subject_add_log (
  jti        TEXT   PRIMARY KEY,
  subject_id BIGINT NOT NULL REFERENCES subjects(id),
  token_exp  TIMESTAMPTZ NOT NULL,
  created_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE TABLE schema_migrations (
  version    INTEGER PRIMARY KEY,
  applied_at TIMESTAMPTZ NOT NULL DEFAULT now()
);
