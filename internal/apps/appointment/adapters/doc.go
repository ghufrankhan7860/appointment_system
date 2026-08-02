// Package adapters is the parent for external appointment provider implementations.
//
// Create one child package per real provider. Keep vendor SDK and wire DTO types inside
// that child, translate to application-owned inputs and outputs, classify provider errors,
// and enforce timeouts, bounded idempotent retries, response-size limits, tracing, and
// redaction. Run shared contract tests against every implementation.
package adapters
