package apperrors

import "errors"

// ValidationError says which field of a request is wrong and why. It wraps a
// sentinel, so errors.Is(err, ErrInvalidPatientInput) keeps working, and
// handlers show Reason(err) to the client next to their generic message.
type ValidationError struct {
	Sentinel error
	Field    string
	Reason   string
}

func (e *ValidationError) Error() string {
	return reasonText(e.Field, e.Reason) + ": " + e.Sentinel.Error()
}

func (e *ValidationError) Unwrap() error {
	return e.Sentinel
}

// Invalid reports a client mistake in field. field is the JSON name; leave it
// empty when the rule spans several fields and reason already names them.
func Invalid(sentinel error, field string, reason string) error {
	return &ValidationError{Sentinel: sentinel, Field: field, Reason: reason}
}

// Reason returns the client-facing explanation carried in err, such as
// "dateOfBirth cannot be in the future", or "" when err has none.
func Reason(err error) string {
	var validationError *ValidationError
	if errors.As(err, &validationError) {
		return reasonText(validationError.Field, validationError.Reason)
	}

	return ""
}

func reasonText(field string, reason string) string {
	if field == "" {
		return reason
	}

	return field + " " + reason
}
