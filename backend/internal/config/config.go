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
}

// Load reads configuration from the environment, applying documented defaults.
func Load() Config {
	return Config{
		Addr:          env("EAGORA_ADDR", ":8080"),
		DatabaseURL:   env("DATABASE_URL", "postgres://eagora:eagora@localhost:5432/eagora?sslmode=disable"),
		Seed:          env("EAGORA_SEED", "auto"),
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
	}
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
