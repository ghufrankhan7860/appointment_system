// Package tests holds every test in the repository, one folder per package under
// test (tests/services, tests/handler, ...). Each folder is an external package
// named <package>_test, so tests exercise exported behaviour only: inject
// collaborators such as repositories or the clock through constructors instead
// of reaching into unexported fields. Shared fakes live in each folder's
// fakes_test.go. Database tests skip when Postgres is not running (make db).
package tests
