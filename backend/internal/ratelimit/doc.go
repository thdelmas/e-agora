// Package ratelimit provides a per-session token-bucket limiter for mutating
// endpoints (R11, on by default). In-memory with idle eviction for the
// single-instance v1; the store is swappable for Redis/Postgres to scale.
//
// Implemented in M3 (docs/07-roadmap.md).
package ratelimit
