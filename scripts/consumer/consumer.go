// Package consumer contains long-running message processing loops.
//
// Validate event type and schema version, propagate context metadata, call application
// services, and assume at-least-once delivery. Define idempotency, retryable versus
// permanent errors, bounded backoff, dead-letter behavior, commit decisions, shutdown, and
// tests before wiring a consumer command.
package consumer
