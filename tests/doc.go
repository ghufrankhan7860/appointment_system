// Package tests is reserved for cross-package integration and end-to-end tests.
//
// Keep ordinary unit tests beside the package under test. Add tests here only when behavior
// crosses package boundaries or starts the assembled application. Make external
// dependencies reproducible, isolate test data, and test cancellation and cleanup.
package tests
