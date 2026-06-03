// Package token mints and verifies the stateless, signed 24h access token that
// gates the leaderboard (R10, docs/02-architecture.md §Access tokens). The
// token carries no identifier — payload {iss, iat, exp, jti} signed with
// HMAC-SHA256.
//
// Implemented in M3 (docs/07-roadmap.md).
package token
