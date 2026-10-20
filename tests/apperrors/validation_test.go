package apperrors_test

import (
	"appointment_system/internal/apps/appointment/apperrors"
	"errors"
	"fmt"
	"testing"
)

func TestValidationErrorReason(t *testing.T) {
	tests := []struct {
		name       string
		err        error
		wantReason string
		wantIs     error
	}{
		{
			name:       "field and reason",
			err:        apperrors.Invalid(apperrors.ErrInvalidPatientInput, "dateOfBirth", "cannot be in the future"),
			wantReason: "dateOfBirth cannot be in the future",
			wantIs:     apperrors.ErrInvalidPatientInput,
		},
		{
			name:       "reason spanning several fields",
			err:        apperrors.Invalid(apperrors.ErrInvalidAvailabilityInput, "", "give both startTime and endTime, or neither for a whole day off"),
			wantReason: "give both startTime and endTime, or neither for a whole day off",
			wantIs:     apperrors.ErrInvalidAvailabilityInput,
		},
		{
			name:       "still found after wrapping",
			err:        fmt.Errorf("create patient: %w", apperrors.Invalid(apperrors.ErrInvalidPatientInput, "name", "is required")),
			wantReason: "name is required",
			wantIs:     apperrors.ErrInvalidPatientInput,
		},
		{
			name:       "plain sentinel has no reason",
			err:        apperrors.ErrPatientNotFound,
			wantReason: "",
			wantIs:     apperrors.ErrPatientNotFound,
		},
		{
			name:       "internal error has no reason",
			err:        fmt.Errorf("get patient with missing ids: %w", apperrors.ErrInvalidPatientInput),
			wantReason: "",
			wantIs:     apperrors.ErrInvalidPatientInput,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := apperrors.Reason(tt.err); got != tt.wantReason {
				t.Fatalf("Reason = %q, want %q", got, tt.wantReason)
			}
			if !errors.Is(tt.err, tt.wantIs) {
				t.Fatalf("errors.Is(%v, %v) = false, want true", tt.err, tt.wantIs)
			}
		})
	}
}
