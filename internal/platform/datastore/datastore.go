// Package datastore owns SQL client construction, pool policy, transaction primitives,
// migration execution hooks, readiness checks, and shutdown.
//
// Configure explicit connection and query timeouts, pool limits, context propagation, and
// slow-query telemetry. Return narrow infrastructure types to composition roots; do not
// place appointment queries or business rules here.
package datastore
