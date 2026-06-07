package ingest

import (
	"context"
	"time"
)

// --- seeder tests (fakes, no network) ----------------------------------------

type fakeWriter struct {
	count    int
	subjects int
	// returned by AllSubjectQIDs (the sync's existing pool)
	qids []string
	// returned by SubjectQIDsMissingGeo (the backfill work-list)
	missingGeo     []string
	translations   []translation
	pageviews      []pageview
	geos           []geo
	globalRefishes int
}

type translation struct{ lang, name, desc, extract, img, url string }
type pageview struct {
	subjectID int64
	lang      string
	views     int64
}
type geo struct {
	qid        string
	countries  []string
	continents []string
}

func (f *fakeWriter) CountSubjects(context.Context) (int, error) {
	return f.count, nil
}
func (f *fakeWriter) AllSubjectQIDs(context.Context) ([]string, error) {
	return f.qids, nil
}
func (f *fakeWriter) UpsertSubject(
	_ context.Context, _, _, _ string, _ []string, _ string,
) (int64, error) {
	f.subjects++
	return int64(f.subjects), nil
}
func (f *fakeWriter) UpsertTranslation(
	_ context.Context, _ int64, lang, name, desc, extract, img, url string,
) error {
	f.translations = append(
		f.translations, translation{lang, name, desc, extract, img, url},
	)
	return nil
}
func (f *fakeWriter) UpsertPageviews(
	_ context.Context, subjectID int64, lang string, views int64,
) error {
	f.pageviews = append(f.pageviews, pageview{subjectID, lang, views})
	return nil
}
func (f *fakeWriter) RefreshGlobalViews(context.Context) error {
	f.globalRefishes++
	return nil
}
func (f *fakeWriter) SetSubjectGeo(
	_ context.Context, qid string, countries, continents []string,
) error {
	f.geos = append(f.geos, geo{qid, countries, continents})
	return nil
}
func (f *fakeWriter) SubjectQIDsMissingGeo(context.Context) ([]string, error) {
	return f.missingGeo, nil
}

type fakeFetcher struct {
	entity    func(qid string) (EntityFacts, error)
	summary   func(lang, title string) (Summary, error)
	leaders   func() ([]string, error)
	pageviews func(lang, title string) (int64, error)
}

func (f fakeFetcher) Entity(
	_ context.Context, qid string,
) (EntityFacts, error) {
	return f.entity(qid)
}
func (f fakeFetcher) Summary(
	_ context.Context, lang, title string,
) (Summary, error) {
	return f.summary(lang, title)
}
func (f fakeFetcher) LeaderQIDs(context.Context) ([]string, error) {
	if f.leaders == nil {
		return nil, nil
	}
	return f.leaders()
}
func (f fakeFetcher) Pageviews(
	_ context.Context, lang, title string, _, _ time.Time,
) (int64, error) {
	if f.pageviews == nil {
		return 0, nil
	}
	return f.pageviews(lang, title)
}
