// Package services owns appointment business rules, invariants, orchestration, and
// transaction boundaries.
//
// Start a service method from domain behavior, not from an HTTP route or SQL query. Put
// context.Context first, consume narrow repository/provider contracts, return domain
// values plus typed errors, and wrap causes with %w. Constructors must validate required
// dependencies. Test success, invalid state, dependency failure, and cancellation.
package services
