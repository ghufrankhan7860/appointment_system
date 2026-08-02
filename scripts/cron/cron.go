// Package cron contains scheduled, run-once application jobs.
//
// Reuse application services, accept cancellation, process bounded batches, report
// progress without sensitive data, and return a final error to cmd. Document whether jobs
// may overlap and how exclusivity is enforced. Test partial failure and cancellation.
package cron
