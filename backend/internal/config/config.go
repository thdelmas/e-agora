// Package config loads e-agora's runtime configuration from environment
// variables. Defaults match docs/02-architecture.md §Config so the app runs
// locally with zero setup beyond a reachable PostgreSQL.
package config

import (
	"os"
	"strconv"
	"strings"
	"time"
)

// Config holds all tunables for the backend. Fields are populated by Load.
type Config struct {
	Addr          string        // EAGORA_ADDR
	DatabaseURL   string        // DATABASE_URL
	Seed          string        // EAGORA_SEED: auto|off|force
	SyncInterval  time.Duration // EAGORA_SYNC_INTERVAL: cadence of the Wikidata refresh (off to disable)
	FallbackLang  string        // EAGORA_FALLBACK_LANG
	TokenSecret   string        // EAGORA_TOKEN_SECRET
	AccessTTL     time.Duration // EAGORA_ACCESS_TTL
	AddsPerToken  int           // EAGORA_ADDS_PER_TOKEN
	RateLimitOn   bool          // EAGORA_RATELIMIT
	VoteBurst     int           // EAGORA_VOTE_BURST
	VoteRate      float64       // EAGORA_VOTE_RATE (tokens/sec)
	HumanProvider string        // EAGORA_HUMAN_PROVIDER: dissent|turnstile|pow
	HumanTTL      time.Duration // EAGORA_HUMAN_TTL
	StaticDir     string        // EAGORA_STATIC_DIR: serve the built SPA same-origin (prod)
	CORSOrigin    string        // EAGORA_CORS_ORIGIN (dev only)
	PublicURL     string        // EAGORA_PUBLIC_URL: canonical base URL for SEO (no trailing slash)

	// Recognition & matchup pairing (docs/10-recognition-and-pools.md).
	PageviewLangs  []string      // EAGORA_PAGEVIEW_LANGS: served languages to record pageviews for (off/empty disables)
	PageviewWindow time.Duration // EAGORA_PAGEVIEW_WINDOW: trailing window summed per language
	RecoBase       float64       // EAGORA_RECO_BASE: weight on sitelink count (graceful fallback when pageviews are absent)
	RecoAlpha      float64       // EAGORA_RECO_ALPHA: weight on local attention (views in the visitor's language)
	RecoBeta       float64       // EAGORA_RECO_BETA: weight on global fame (views across all languages)
	RecoGamma      float64       // EAGORA_RECO_GAMMA: weight on sphere affinity (the visitor language's share of attention)
	DiscoveryRate  float64       // EAGORA_DISCOVERY_RATE: fraction of matchups drawing a coverage-biased challenger
	FameTierPct    float64       // EAGORA_FAME_TIER_PCT: global_views percentile cutoff for the "famous only" pool (0.7 = top 30%)
}

// defaultPageviewLangs are the ~20 largest / most-served Wikipedia editions —
// broad coverage of the world's major language spheres without fetching all 300.
var defaultPageviewLangs = []string{
	"en", "es", "fr", "de", "ru", "pt", "it", "zh", "ja", "ar",
	"nl", "pl", "fa", "tr", "id", "uk", "ko", "hi", "sv", "vi",
}

// Load reads configuration from the environment, applying documented defaults.
func Load() Config {
	return Config{
		Addr:          env("EAGORA_ADDR", ":8080"),
		DatabaseURL:   env("DATABASE_URL", "postgres://eagora:eagora@localhost:5432/eagora?sslmode=disable"),
		Seed:          env("EAGORA_SEED", "auto"),
		SyncInterval:  envSyncInterval("EAGORA_SYNC_INTERVAL", 24*time.Hour),
		FallbackLang:  env("EAGORA_FALLBACK_LANG", "en"),
		TokenSecret:   env("EAGORA_TOKEN_SECRET", ""),
		AccessTTL:     envDuration("EAGORA_ACCESS_TTL", 24*time.Hour),
		AddsPerToken:  envInt("EAGORA_ADDS_PER_TOKEN", 1),
		RateLimitOn:   env("EAGORA_RATELIMIT", "on") != "off",
		VoteBurst:     envInt("EAGORA_VOTE_BURST", 20),
		VoteRate:      envFloat("EAGORA_VOTE_RATE", 1),
		HumanProvider: env("EAGORA_HUMAN_PROVIDER", "dissent"),
		HumanTTL:      envDuration("EAGORA_HUMAN_TTL", 24*time.Hour),
		StaticDir:     env("EAGORA_STATIC_DIR", ""),
		CORSOrigin:    env("EAGORA_CORS_ORIGIN", ""),
		PublicURL:     strings.TrimRight(env("EAGORA_PUBLIC_URL", ""), "/"),

		PageviewLangs:  envList("EAGORA_PAGEVIEW_LANGS", defaultPageviewLangs),
		PageviewWindow: envDuration("EAGORA_PAGEVIEW_WINDOW", 90*24*time.Hour),
		// Defaults: local attention (α) and global fame (β) pull equally hard — the
		// two recognition levers — with a sphere boost (γ) and a small sitelink
		// floor (base) that keeps every subject drawable and degrades to the old
		// langs-count weighting before pageviews land. Tune against real pool data
		// (the per-language pageview spread) once it's flowing.
		RecoBase:      envFloat("EAGORA_RECO_BASE", 1.0),
		RecoAlpha:     envFloat("EAGORA_RECO_ALPHA", 3.0),
		RecoBeta:      envFloat("EAGORA_RECO_BETA", 3.0),
		RecoGamma:     envFloat("EAGORA_RECO_GAMMA", 1.5),
		DiscoveryRate: envFloat("EAGORA_DISCOVERY_RATE", 0.15),
		FameTierPct:   envFloat("EAGORA_FAME_TIER_PCT", 0.7),
	}
}

// envList parses a comma-separated list (e.g. "en,fr,de"), trimming and
// lowercasing each entry and dropping empties. "off"/"none"/"disabled" (and an
// all-empty value) return an empty slice — the caller treats that as disabled.
func envList(key string, fallback []string) []string {
	v, ok := os.LookupEnv(key)
	if !ok || v == "" {
		return fallback
	}
	switch strings.ToLower(strings.TrimSpace(v)) {
	case "off", "none", "disabled":
		return nil
	}
	var out []string
	for _, part := range strings.Split(v, ",") {
		if p := strings.ToLower(strings.TrimSpace(part)); p != "" {
			out = append(out, p)
		}
	}
	return out
}

func env(key, fallback string) string {
	if v, ok := os.LookupEnv(key); ok && v != "" {
		return v
	}
	return fallback
}

func envInt(key string, fallback int) int {
	if v, ok := os.LookupEnv(key); ok {
		if n, err := strconv.Atoi(v); err == nil {
			return n
		}
	}
	return fallback
}

func envFloat(key string, fallback float64) float64 {
	if v, ok := os.LookupEnv(key); ok {
		if f, err := strconv.ParseFloat(v, 64); err == nil {
			return f
		}
	}
	return fallback
}

func envDuration(key string, fallback time.Duration) time.Duration {
	if v, ok := os.LookupEnv(key); ok {
		if d, err := time.ParseDuration(v); err == nil {
			return d
		}
	}
	return fallback
}

// envSyncInterval parses the Wikidata-refresh cadence as a Go duration, mapping
// "off"/"none"/"disabled" (and any zero/negative value) to 0, which disables the
// scheduler. An unparseable value falls back to the default.
func envSyncInterval(key string, fallback time.Duration) time.Duration {
	v, ok := os.LookupEnv(key)
	if !ok || v == "" {
		return fallback
	}
	switch strings.ToLower(strings.TrimSpace(v)) {
	case "off", "none", "disabled":
		return 0
	}
	d, err := time.ParseDuration(v)
	if err != nil {
		return fallback
	}
	if d < 0 {
		return 0
	}
	return d
}
