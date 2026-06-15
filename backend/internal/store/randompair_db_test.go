package store

import (
	"context"
	"os"
	"testing"

	"github.com/thdelmas/e-agora/backend/migrations"
)

// TestRandomPairAgainstDB exercises RandomPair against a real PostgreSQL, the
// only place a bug like the integer-coercion of the Beta-prior parameters
// shows up: with literal SQL or pure-Go tests the priors are numeric, but a
// bound parameter ($19..$22) with no cast is inferred as integer, turning the
// membership confidence into integer division and dividing memPriorMean (0.8)
// down to 0 — "division by zero" on every draw. The ::float8 casts in the
// query prevent that; this test guards them.
//
// Skipped unless EAGORA_TEST_DSN points at a throwaway database (CI wires one
// up as a service; see .github/workflows/ci.yml).
func TestRandomPairAgainstDB(t *testing.T) {
	dsn := os.Getenv("EAGORA_TEST_DSN")
	if dsn == "" {
		t.Skip("set EAGORA_TEST_DSN to run the DB-backed RandomPair test")
	}
	ctx := context.Background()
	st, err := Open(ctx, dsn)
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	defer st.Close()
	if _, err := st.Migrate(ctx, migrations.FS); err != nil {
		t.Fatalf("migrate: %v", err)
	}

	// Three living subjects, no belonging/membership rows — the prior-only path
	// that triggered the division-by-zero. Equal global_views so all clear the
	// fame-tier cutoff (so the famous pool still has >=2). Idempotent QIDs so
	// reruns against a persistent DB are harmless.
	if _, err := st.pool.Exec(ctx, `
		INSERT INTO subjects (wikidata_id, canonical_name, available_langs,
		                      global_views, continent, country)
		VALUES ('TEST_RP_1','RP Alice','{en,fr}',1000,
		         ARRAY['Europe'],ARRAY['France']),
		       ('TEST_RP_2','RP Bob','{en}',1000,
		         ARRAY['Europe'],ARRAY['Germany']),
		       ('TEST_RP_3','RP Carol','{en,es}',1000,
		         ARRAY['Europe'],ARRAY['Spain'])
		ON CONFLICT (wikidata_id) DO NOTHING`); err != nil {
		t.Fatalf("seed: %v", err)
	}

	reco := RecoParams{
		Base: 1, Alpha: 3, Beta: 3, Gamma: 1.5,
		Region: 2.5, Country: 3.5, DiscoveryRate: 0.15,
	}
	pools := map[string]Pool{
		"default":         {FamePct: 0.7},
		"famous":          {FameTop: true, FamePct: 0.7},
		"famous+deceased": {FameTop: true, IncludeDeceased: true, FamePct: 0.7},
		"region":          {Continent: "Europe", FamePct: 0.7},
	}
	for name, pool := range pools {
		// Repeat to cover the random() ordering branches.
		for i := 0; i < 10; i++ {
			pair, err := st.RandomPair(ctx, "en", "FR", "France", reco, pool)
			if err != nil {
				t.Fatalf("[%s] RandomPair: %v", name, err)
			}
			if len(pair) != 2 {
				t.Fatalf("[%s] want 2 subjects, got %d", name, len(pair))
			}
		}
	}
}
