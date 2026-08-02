// Package handler contains HTTP adapters for appointment operations.
//
// For each endpoint:
//  1. Bind path, query, header, and body data into an API DTO.
//  2. Perform transport-level validation and enforce input-size limits.
//  3. Call one controller method with context.Context first.
//  4. Map response DTOs and typed application errors to stable HTTP responses.
//
// Keep business rules, transactions, and database queries out of this package. Test
// handlers with httptest, including invalid input and error mapping.
package handler
