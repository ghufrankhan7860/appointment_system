// Package models contains appointment domain models and, only when clearly separated,
// persistence records.
//
// Define domain entities and value objects around business meaning and invariants. Keep
// JSON tags, GORM fields, HTTP framework types, and provider wire fields out of domain
// types. If persistence shape differs, create a separately named record and translate at
// the repository boundary.
package models
