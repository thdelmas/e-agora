// Package human implements the anonymous, dissent-based humanity check that
// gates voting (R12, docs/01-functional-spec.md S4). The default provider issues
// a stateless signed challenge and grants a time-boxed human-verified status on
// the session; the provider is an interface (dissent | turnstile | pow).
//
// Implemented in M3 (docs/07-roadmap.md).
package human
