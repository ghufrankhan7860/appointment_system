// Package redis constructs and manages the distributed-cache client.
//
// Add bounded dial and operation timeouts, pool limits, health checks, telemetry, and
// graceful close behavior. This package implements technical access only; appointment key,
// TTL, invalidation, and staleness policy belongs to the app cache package.
package redis
