// Package subjects implements user-contributed additions to the pool (R8/R8.1):
// resolve an input to a Wikidata QID, require a human (Q5) with a Wikipedia page
// (R2), dedupe by QID, and insert while atomically claiming the access token's
// one add allowance. Token-gating itself is enforced in the HTTP layer.
package subjects

import (
	"context"
	"errors"
	"strings"
	"time"

	"github.com/thdelmas/e-agora/backend/internal/ingest"
	"github.com/thdelmas/e-agora/backend/internal/model"
	"github.com/thdelmas/e-agora/backend/internal/store"
)

// Service-level errors, mapped to HTTP status codes by the handler.
var (
	ErrBadInput  = errors.New("subjects: unusable input")      // 422 not_a_wikipedia_page
	ErrNoPage    = errors.New("subjects: no Wikipedia page")   // 422 not_a_wikipedia_page
	ErrNotPerson = errors.New("subjects: not a person")        // 422 not_a_person
	ErrExists    = errors.New("subjects: already in the pool") // 409 already_exists
	ErrAddLimit  = errors.New("subjects: add limit reached")   // 429 add_limit_reached
)

// Fetcher is the upstream slice the service needs (satisfied by *ingest.Client).
type Fetcher interface {
	ResolveWikipediaURL(ctx context.Context, raw string) (string, error)
	Entity(ctx context.Context, qid string) (ingest.EntityFacts, error)
	Summary(ctx context.Context, lang, title string) (ingest.Summary, error)
}

// Store is the persistence slice the service needs (satisfied by *store.Store).
type Store interface {
	AddTokenUsed(ctx context.Context, jti string) (bool, error)
	SubjectIDByQID(ctx context.Context, qid string) (int64, bool, error)
	InsertUserSubject(ctx context.Context, ns store.NewSubject, jti string, tokenExp time.Time) (int64, error)
	InsertRecalledSubject(ctx context.Context, ns store.NewSubject) (int64, error)
}

// Service orchestrates adds.
type Service struct {
	fetch Fetcher
	store Store
}

func New(f Fetcher, s Store) *Service { return &Service{fetch: f, store: s} }

// AddInput is the request: exactly one of URL or WikidataID.
type AddInput struct {
	URL        string
	WikidataID string
}

// Add validates and inserts a user-contributed subject for the given access
// token (jti/exp), returning the public projection. Errors are the sentinels
// above. The token's allowance is consumed only on success (R8.1).
func (s *Service) Add(ctx context.Context, in AddInput, jti string, tokenExp time.Time) (model.SubjectPublic, error) {
	// Cheap precheck so a spent token never triggers network work.
	if used, err := s.store.AddTokenUsed(ctx, jti); err != nil {
		return model.SubjectPublic{}, err
	} else if used {
		return model.SubjectPublic{}, ErrAddLimit
	}

	qid := strings.TrimSpace(in.WikidataID)
	if qid == "" {
		if strings.TrimSpace(in.URL) == "" {
			return model.SubjectPublic{}, ErrBadInput
		}
		resolved, err := s.fetch.ResolveWikipediaURL(ctx, in.URL)
		switch {
		case errors.Is(err, ingest.ErrBadInput):
			return model.SubjectPublic{}, ErrBadInput
		case errors.Is(err, ingest.ErrNotFound):
			return model.SubjectPublic{}, ErrNoPage
		case err != nil:
			return model.SubjectPublic{}, err
		}
		qid = resolved
	}
	if !looksLikeQID(qid) {
		return model.SubjectPublic{}, ErrBadInput
	}

	if _, exists, err := s.store.SubjectIDByQID(ctx, qid); err != nil {
		return model.SubjectPublic{}, err
	} else if exists {
		return model.SubjectPublic{}, ErrExists
	}

	facts, err := s.fetch.Entity(ctx, qid)
	if errors.Is(err, ingest.ErrNotFound) {
		return model.SubjectPublic{}, ErrNoPage
	}
	if err != nil {
		return model.SubjectPublic{}, err
	}
	if !facts.IsHuman {
		return model.SubjectPublic{}, ErrNotPerson
	}
	if facts.EnwikiTitle == "" {
		return model.SubjectPublic{}, ErrNoPage
	}

	name := firstNonEmpty(facts.LabelEn, facts.EnwikiTitle)
	langs := facts.Langs
	if len(langs) == 0 {
		langs = []string{"en"}
	}

	enName, enDesc, enExtract, enImage := name, "", "", ""
	enURL := "https://en.wikipedia.org/wiki/" + strings.ReplaceAll(facts.EnwikiTitle, " ", "_")
	if sum, err := s.fetch.Summary(ctx, "en", facts.EnwikiTitle); err == nil {
		enName = firstNonEmpty(sum.Name, name)
		enDesc, enExtract, enImage = sum.Description, sum.Extract, sum.ImageURL
		if sum.WikipediaURL != "" {
			enURL = sum.WikipediaURL
		}
	}

	id, err := s.store.InsertUserSubject(ctx, store.NewSubject{
		QID: qid, Name: name, Langs: langs, DiedAt: facts.DiedAt,
		EnName: enName, EnDesc: enDesc, EnExtract: enExtract, EnImage: enImage, EnURL: enURL,
	}, jti, tokenExp)
	switch {
	case errors.Is(err, store.ErrAlreadyExists):
		return model.SubjectPublic{}, ErrExists
	case errors.Is(err, store.ErrAddLimit):
		return model.SubjectPublic{}, ErrAddLimit
	case err != nil:
		return model.SubjectPublic{}, err
	}

	return model.SubjectPublic{
		ID: id, WikidataID: qid, Name: enName, Description: enDesc,
		Extract: enExtract, ImageURL: enImage, WikipediaURL: enURL,
		Deceased: facts.DiedAt != "",
	}, nil
}

// EnsureFromURL resolves a Wikipedia URL to a subject id for the belonging recall
// step (docs/11 §3): if the figure is already in the pool it returns their id;
// otherwise it validates (a human with an English Wikipedia page) and ingests
// them *ungated by the add-token* — recall precedes voting, so the rate limit and
// belonging demotion are the controls, not the one-per-token allowance. Returns
// the sentinel errors above. created reports whether a new subject was inserted.
func (s *Service) EnsureFromURL(ctx context.Context, url string) (id int64, created bool, err error) {
	if strings.TrimSpace(url) == "" {
		return 0, false, ErrBadInput
	}
	qid, err := s.fetch.ResolveWikipediaURL(ctx, url)
	switch {
	case errors.Is(err, ingest.ErrBadInput):
		return 0, false, ErrBadInput
	case errors.Is(err, ingest.ErrNotFound):
		return 0, false, ErrNoPage
	case err != nil:
		return 0, false, err
	}
	if !looksLikeQID(qid) {
		return 0, false, ErrBadInput
	}
	if existing, ok, err := s.store.SubjectIDByQID(ctx, qid); err != nil {
		return 0, false, err
	} else if ok {
		return existing, false, nil // already in the pool — just propose them
	}

	facts, err := s.fetch.Entity(ctx, qid)
	if errors.Is(err, ingest.ErrNotFound) {
		return 0, false, ErrNoPage
	}
	if err != nil {
		return 0, false, err
	}
	if !facts.IsHuman {
		return 0, false, ErrNotPerson
	}
	if facts.EnwikiTitle == "" {
		return 0, false, ErrNoPage
	}

	name := firstNonEmpty(facts.LabelEn, facts.EnwikiTitle)
	langs := facts.Langs
	if len(langs) == 0 {
		langs = []string{"en"}
	}
	enName, enDesc, enExtract, enImage := name, "", "", ""
	enURL := "https://en.wikipedia.org/wiki/" + strings.ReplaceAll(facts.EnwikiTitle, " ", "_")
	if sum, err := s.fetch.Summary(ctx, "en", facts.EnwikiTitle); err == nil {
		enName = firstNonEmpty(sum.Name, name)
		enDesc, enExtract, enImage = sum.Description, sum.Extract, sum.ImageURL
		if sum.WikipediaURL != "" {
			enURL = sum.WikipediaURL
		}
	}

	id, err = s.store.InsertRecalledSubject(ctx, store.NewSubject{
		QID: qid, Name: name, Langs: langs, DiedAt: facts.DiedAt,
		EnName: enName, EnDesc: enDesc, EnExtract: enExtract, EnImage: enImage, EnURL: enURL,
	})
	if errors.Is(err, store.ErrAlreadyExists) {
		// Raced with a concurrent add — the figure now exists; propose them.
		if existing, ok, e := s.store.SubjectIDByQID(ctx, qid); e == nil && ok {
			return existing, false, nil
		}
	}
	if err != nil {
		return 0, false, err
	}
	return id, true, nil
}

func looksLikeQID(s string) bool {
	if len(s) < 2 || (s[0] != 'Q' && s[0] != 'q') {
		return false
	}
	for _, r := range s[1:] {
		if r < '0' || r > '9' {
			return false
		}
	}
	return true
}

func firstNonEmpty(vals ...string) string {
	for _, v := range vals {
		if v != "" {
			return v
		}
	}
	return ""
}
