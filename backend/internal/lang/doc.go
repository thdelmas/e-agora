// Package lang resolves the display language: it maps Accept-Language (and an
// optional ?lang= override) to a Wikipedia code by primary subtag, and applies
// the R9 rule — the visitor's language only if both subjects have it, else
// English (docs/01-functional-spec.md §Internationalization).
//
// Implemented in M3 (docs/07-roadmap.md).
package lang
