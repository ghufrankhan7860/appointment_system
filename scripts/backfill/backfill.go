// Package backfill contains explicit, restartable data repair and import operations.
//
// Use checkpoints, deterministic batches, bounded concurrency, cancellation, and safe
// resume semantics. Keep large data changes out of startup migrations. Return errors to the
// command layer and document rollback or forward-fix behavior before deployment.
package backfill
