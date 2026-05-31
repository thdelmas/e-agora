package store

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
)

var (
	// ErrAlreadyExists means a subject with that Wikidata QID is already present.
	ErrAlreadyExists = errors.New("store: subject already exists")
	// ErrAddLimit means the access token already spent its one add (R8.1).
	ErrAddLimit = errors.New("store: add limit reached for token")
)

// NewSubject carries the fields needed to create a user-added subject (R8).
type NewSubject struct {
	QID, Name      string
	Langs          []string
	EnName, EnDesc string
	EnImage, EnURL string
}

// SubjectIDByQID returns the id of a subject with the given QID, and whether it
// exists — used to reject duplicate adds early with a helpful 409.
func (s *Store) SubjectIDByQID(ctx context.Context, qid string) (int64, bool, error) {
	var id int64
	err := s.pool.QueryRow(ctx, `SELECT id FROM subjects WHERE wikidata_id = $1`, qid).Scan(&id)
	if errors.Is(err, pgx.ErrNoRows) {
		return 0, false, nil
	}
	if err != nil {
		return 0, false, fmt.Errorf("subject by qid: %w", err)
	}
	return id, true, nil
}

// AddTokenUsed reports whether a token (by jti) has already spent its one add —
// a cheap precheck before doing any network work.
func (s *Store) AddTokenUsed(ctx context.Context, jti string) (bool, error) {
	var exists bool
	if err := s.pool.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM subject_add_log WHERE jti = $1)`, jti).Scan(&exists); err != nil {
		return false, fmt.Errorf("add-token check: %w", err)
	}
	return exists, nil
}

// InsertUserSubject creates a user-added subject, its English translation, and
// claims the token's single add allowance — all atomically (R8.1). The jti claim
// is the LAST gate, so a duplicate-QID (ErrAlreadyExists) or a spent token
// (ErrAddLimit) rolls back without consuming the allowance.
func (s *Store) InsertUserSubject(ctx context.Context, ns NewSubject, jti string, tokenExp time.Time) (int64, error) {
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return 0, err
	}
	defer tx.Rollback(ctx)

	var id int64
	err = tx.QueryRow(ctx, `
		INSERT INTO subjects (wikidata_id, canonical_name, source, available_langs)
		VALUES ($1, $2, 'user', $3) RETURNING id`,
		ns.QID, ns.Name, ns.Langs).Scan(&id)
	if err != nil {
		if isUniqueViolation(err) {
			return 0, ErrAlreadyExists
		}
		return 0, fmt.Errorf("insert subject: %w", err)
	}

	tag, err := tx.Exec(ctx, `
		INSERT INTO subject_add_log (jti, subject_id, token_exp)
		VALUES ($1, $2, $3) ON CONFLICT (jti) DO NOTHING`,
		jti, id, tokenExp)
	if err != nil {
		return 0, fmt.Errorf("claim add token: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return 0, ErrAddLimit // token already spent — rollback undoes the insert
	}

	if _, err := tx.Exec(ctx, `
		INSERT INTO subject_translations (subject_id, lang, name, description, image_url, wikipedia_url)
		VALUES ($1, 'en', $2, NULLIF($3, ''), NULLIF($4, ''), $5)`,
		id, ns.EnName, ns.EnDesc, ns.EnImage, ns.EnURL); err != nil {
		return 0, fmt.Errorf("insert translation: %w", err)
	}

	if err := tx.Commit(ctx); err != nil {
		return 0, err
	}
	return id, nil
}

func isUniqueViolation(err error) bool {
	var pg *pgconn.PgError
	return errors.As(err, &pg) && pg.Code == "23505"
}
