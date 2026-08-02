// Package kafka contains broker producer and consumer adapters.
//
// Define bounded retries, delivery acknowledgements, commit behavior, cancellation,
// tracing, and health checks here. Application-owned event schemas and idempotency policy
// must remain outside vendor-specific APIs. Never commit a message before processing has
// completed successfully or reached a deliberate terminal disposition.
package kafka
