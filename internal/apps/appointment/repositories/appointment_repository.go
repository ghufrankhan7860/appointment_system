// Package repositories contains appointment persistence adapters.
//
// Implement only persistence behavior required by a consuming service contract. Pass
// context to every query, parameterize values, bound result sets, translate records into
// domain models, and classify database errors. Accept a narrow transaction/query object
// when needed. Do not enforce business eligibility, call providers, or publish events.
package repositories
