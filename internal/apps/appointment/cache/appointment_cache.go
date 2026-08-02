// Package cache owns appointment-specific cache policy.
//
// Before adding a cache, document its key and tenant scope, TTL, acceptable staleness,
// invalidation strategy, serialization version, failure behavior, and stampede control.
// Treat cache failure as best-effort unless the design explicitly makes it authoritative.
// Keep Redis client construction in the platform adapter and composition root.
package cache
