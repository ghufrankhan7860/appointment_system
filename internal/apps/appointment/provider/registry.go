// Package provider selects interchangeable appointment-related external capabilities.
//
// Add a registry only when multiple adapters implement one application-owned interface.
// Register stable provider IDs during startup, reject duplicates and unknown IDs, and
// return the interface rather than a concrete adapter. Shared workflows must not switch on
// provider-specific types or responses.
package provider
