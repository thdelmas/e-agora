package subjects

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/thdelmas/e-agora/backend/internal/ingest"
	"github.com/thdelmas/e-agora/backend/internal/store"
)

type fakeFetcher struct {
	resolve func(string) (string, error)
	entity  func(string) (ingest.EntityFacts, error)
	summary func(string, string) (ingest.Summary, error)
}

func (f fakeFetcher) ResolveWikipediaURL(_ context.Context, raw string) (string, error) {
	return f.resolve(raw)
}
func (f fakeFetcher) Entity(_ context.Context, qid string) (ingest.EntityFacts, error) {
	return f.entity(qid)
}
func (f fakeFetcher) Summary(_ context.Context, lang, title string) (ingest.Summary, error) {
	if f.summary == nil {
		return ingest.Summary{}, errors.New("no summary")
	}
	return f.summary(lang, title)
}

type fakeStore struct {
	used      bool
	exists    bool
	inserted  int
	insertErr error
}

func (f *fakeStore) AddTokenUsed(context.Context, string) (bool, error) { return f.used, nil }
func (f *fakeStore) SubjectIDByQID(context.Context, string) (int64, bool, error) {
	return 0, f.exists, nil
}
func (f *fakeStore) InsertUserSubject(context.Context, store.NewSubject, string, time.Time) (int64, error) {
	if f.insertErr != nil {
		return 0, f.insertErr
	}
	f.inserted++
	return 99, nil
}

func human() fakeFetcher {
	return fakeFetcher{
		entity: func(string) (ingest.EntityFacts, error) {
			return ingest.EntityFacts{QID: "Q1", IsHuman: true, LabelEn: "A Person", EnwikiTitle: "A_Person", Langs: []string{"en"}}, nil
		},
		summary: func(string, string) (ingest.Summary, error) {
			return ingest.Summary{Name: "A Person", Description: "A politician.", WikipediaURL: "https://en.wikipedia.org/wiki/A_Person"}, nil
		},
	}
}

func TestAdd_Success(t *testing.T) {
	st := &fakeStore{}
	out, err := New(human(), st).Add(context.Background(), AddInput{WikidataID: "Q1"}, "jti", time.Now().Add(time.Hour))
	if err != nil {
		t.Fatalf("Add: %v", err)
	}
	if out.ID != 99 || out.Name != "A Person" || st.inserted != 1 {
		t.Errorf("unexpected result: %+v inserted=%d", out, st.inserted)
	}
}

func TestAdd_Errors(t *testing.T) {
	hum := human()
	cases := []struct {
		name    string
		fetch   fakeFetcher
		store   *fakeStore
		in      AddInput
		wantErr error
	}{
		{"token already used", hum, &fakeStore{used: true}, AddInput{WikidataID: "Q1"}, ErrAddLimit},
		{"duplicate qid", hum, &fakeStore{exists: true}, AddInput{WikidataID: "Q1"}, ErrExists},
		{"no input", hum, &fakeStore{}, AddInput{}, ErrBadInput},
		{"not a qid", hum, &fakeStore{}, AddInput{WikidataID: "banana"}, ErrBadInput},
		{
			"not a person",
			fakeFetcher{entity: func(string) (ingest.EntityFacts, error) {
				return ingest.EntityFacts{QID: "Q1", IsHuman: false, EnwikiTitle: "X"}, nil
			}},
			&fakeStore{}, AddInput{WikidataID: "Q1"}, ErrNotPerson,
		},
		{
			"no english page",
			fakeFetcher{entity: func(string) (ingest.EntityFacts, error) {
				return ingest.EntityFacts{QID: "Q1", IsHuman: true, EnwikiTitle: ""}, nil
			}},
			&fakeStore{}, AddInput{WikidataID: "Q1"}, ErrNoPage,
		},
		{
			"url resolves to no page",
			fakeFetcher{resolve: func(string) (string, error) { return "", ingest.ErrNotFound }},
			&fakeStore{}, AddInput{URL: "https://en.wikipedia.org/wiki/Nope"}, ErrNoPage,
		},
		{
			"insert hits add limit (race)",
			hum, &fakeStore{insertErr: store.ErrAddLimit}, AddInput{WikidataID: "Q1"}, ErrAddLimit,
		},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			_, err := New(c.fetch, c.store).Add(context.Background(), c.in, "jti", time.Now().Add(time.Hour))
			if !errors.Is(err, c.wantErr) {
				t.Errorf("err = %v, want %v", err, c.wantErr)
			}
		})
	}
}
