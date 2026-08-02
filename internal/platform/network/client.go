// Package network creates outbound HTTP clients with shared reliability policy.
//
// Set connect and request timeouts, response-size limits, trace propagation, redacted
// logging, and bounded jittered retries for safe operations. Retries must respect context
// cancellation and idempotency. Provider DTOs do not belong in this package.
package network
