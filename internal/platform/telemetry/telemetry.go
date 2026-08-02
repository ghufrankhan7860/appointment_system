// Package telemetry initializes traces, metrics, profiling, and exporter shutdown.
//
// Add runtime metadata and low-cardinality operation labels, propagate context across I/O,
// and flush providers during graceful shutdown. Protect internal metrics and profiling
// endpoints; never use user IDs or raw URLs as metric labels.
package telemetry
