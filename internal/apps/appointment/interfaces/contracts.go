// Package interfaces contains application-owned capability contracts shared across more
// than one appointment package.
//
// Define an interface here only when this application consumes behavior that needs a real
// boundary, such as transactions, event publication, or an interchangeable provider. Keep
// contracts narrow and domain-oriented. Prefer placing a single-consumer interface beside
// that consumer, and never mirror a concrete implementation mechanically.
package interfaces
