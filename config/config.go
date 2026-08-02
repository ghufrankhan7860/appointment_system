// Package config defines typed runtime configuration, loading, defaults, and startup
// validation.
//
// Add fields grouped by concern, such as HTTP, database, Redis, Kafka, and telemetry.
// Load environment values once, validate required URLs, durations, and limits, then
// pass the typed struct into cmd. Business packages must not read environment values
// or call Viper directly.
package config

