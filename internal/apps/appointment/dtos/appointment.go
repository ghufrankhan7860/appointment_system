// Package dtos defines explicit appointment inputs and outputs at application and API
// boundaries.
//
// Split request and response files when real contracts are added. Declare JSON fields and
// validation rules explicitly, document empty collection semantics, and map DTOs to domain
// values in the controller. Never expose persistence records as API responses.
package dtos
