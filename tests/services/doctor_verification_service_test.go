package services_test

import (
	"context"
	"errors"
	"testing"

	"appointment_system/internal/apps/appointment/apperrors"
	"appointment_system/internal/apps/appointment/constant"
	"appointment_system/internal/apps/appointment/models"
	"appointment_system/internal/apps/appointment/services"
)

func TestDoctorVerificationServiceTransitions(t *testing.T) {
	tests := []struct {
		name           string
		current        models.VerificationStatus
		missing        bool
		updateErr      error
		action         string
		reason         string
		wantStatus     models.VerificationStatus
		wantErr        error
		wantVerifiedAt bool
	}{
		{name: "verify pending", current: models.VerificationStatusPending, action: "Verify", wantStatus: models.VerificationStatusVerified, wantVerifiedAt: true},
		{name: "verify already verified", current: models.VerificationStatusVerified, action: "Verify", wantErr: apperrors.ErrInvalidVerificationTransition},
		{name: "verify rejected", current: models.VerificationStatusRejected, action: "Verify", wantErr: apperrors.ErrInvalidVerificationTransition},
		{name: "reject pending with reason", current: models.VerificationStatusPending, action: "Reject", reason: "bad number", wantStatus: models.VerificationStatusRejected},
		{name: "reject without reason", current: models.VerificationStatusPending, action: "Reject", reason: "  ", wantErr: apperrors.ErrVerificationReasonRequired},
		{name: "reject verified", current: models.VerificationStatusVerified, action: "Reject", reason: "late", wantErr: apperrors.ErrInvalidVerificationTransition},
		{name: "suspend verified with reason", current: models.VerificationStatusVerified, action: "Suspend", reason: "complaint", wantStatus: models.VerificationStatusSuspended},
		{name: "suspend without reason", current: models.VerificationStatusVerified, action: "Suspend", wantErr: apperrors.ErrVerificationReasonRequired},
		{name: "suspend pending", current: models.VerificationStatusPending, action: "Suspend", reason: "complaint", wantErr: apperrors.ErrInvalidVerificationTransition},
		{name: "reinstate suspended", current: models.VerificationStatusSuspended, action: "Reinstate", wantStatus: models.VerificationStatusVerified, wantVerifiedAt: true},
		{name: "reinstate verified", current: models.VerificationStatusVerified, action: "Reinstate", wantErr: apperrors.ErrInvalidVerificationTransition},
		{name: "missing doctor", missing: true, action: "Verify", wantErr: apperrors.ErrDoctorNotFound},
		{name: "concurrent review lost the race", current: models.VerificationStatusPending, updateErr: apperrors.ErrInvalidVerificationTransition, action: "Verify", wantErr: apperrors.ErrInvalidVerificationTransition},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			doctors := &fakeDoctorRepo{updateStatusErr: tt.updateErr}
			if !tt.missing {
				doctors.byID = &models.Doctor{ID: 5, VerificationStatus: tt.current}
			}
			verifications := &fakeDoctorVerificationRepo{}
			tx := &fakeTxManager{}
			service, err := services.NewDoctorVerificationService(doctors, verifications, tx)
			if err != nil {
				t.Fatalf("NewDoctorVerificationService returned error: %v", err)
			}

			var reviewed *models.Doctor
			switch tt.action {
			case "Verify":
				reviewed, err = service.Verify(context.Background(), 1, 5)
			case "Reject":
				reviewed, err = service.Reject(context.Background(), 1, 5, tt.reason)
			case "Suspend":
				reviewed, err = service.Suspend(context.Background(), 1, 5, tt.reason)
			case "Reinstate":
				reviewed, err = service.Reinstate(context.Background(), 1, 5, tt.reason)
			}
			if !errors.Is(err, tt.wantErr) {
				t.Fatalf("%s error = %v, want %v", tt.action, err, tt.wantErr)
			}
			if tt.wantErr != nil {
				if len(verifications.created) != 0 {
					t.Fatalf("%s wrote %d audit rows after failing", tt.action, len(verifications.created))
				}
				return
			}

			if !tx.called {
				t.Fatalf("%s did not run inside transaction", tt.action)
			}
			if reviewed.VerificationStatus != tt.wantStatus || doctors.updateStatusTo != tt.wantStatus || doctors.updateStatusFrom != tt.current {
				t.Fatalf("status change %s -> %s, reviewed %s", doctors.updateStatusFrom, doctors.updateStatusTo, reviewed.VerificationStatus)
			}
			if tt.wantVerifiedAt && reviewed.VerifiedAt == nil {
				t.Fatalf("verified_at not set")
			}
			if len(verifications.created) != 1 {
				t.Fatalf("audit rows = %d, want 1", len(verifications.created))
			}
			audit := verifications.created[0]
			if audit.ReviewedByUserID != 1 || audit.FromStatus != tt.current || audit.ToStatus != tt.wantStatus {
				t.Fatalf("audit row = %+v", audit)
			}
		})
	}
}

func TestDoctorVerificationServiceList(t *testing.T) {
	tests := []struct {
		name    string
		status  models.VerificationStatus
		wantErr error
	}{
		{name: "pending queue", status: models.VerificationStatusPending},
		{name: "every status", status: ""},
		{name: "unknown status", status: "approved", wantErr: apperrors.ErrInvalidDoctorInput},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			doctors := &fakeDoctorRepo{}
			service, _ := services.NewDoctorVerificationService(doctors, &fakeDoctorVerificationRepo{}, &fakeTxManager{})
			if _, err := service.List(context.Background(), tt.status, 0, 0); !errors.Is(err, tt.wantErr) {
				t.Fatalf("List error = %v, want %v", err, tt.wantErr)
			}
			if tt.wantErr == nil && (doctors.listStatus != tt.status || doctors.listLimit != constant.DefaultListLimit) {
				t.Fatalf("list called with %q/%d", doctors.listStatus, doctors.listLimit)
			}
		})
	}
}
