package services

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"appointment_system/internal/apps/appointment/apperrors"
	"appointment_system/internal/apps/appointment/models"
)

type verificationTransition struct {
	from           models.VerificationStatus
	to             models.VerificationStatus
	reasonRequired bool
}

var (
	verifyTransition    = verificationTransition{from: models.VerificationStatusPending, to: models.VerificationStatusVerified}
	rejectTransition    = verificationTransition{from: models.VerificationStatusPending, to: models.VerificationStatusRejected, reasonRequired: true}
	suspendTransition   = verificationTransition{from: models.VerificationStatusVerified, to: models.VerificationStatusSuspended, reasonRequired: true}
	reinstateTransition = verificationTransition{from: models.VerificationStatusSuspended, to: models.VerificationStatusVerified}
)

// DoctorVerificationService owns admin review of doctor profiles. Every status
// change is written together with an audit row naming the admin.
type DoctorVerificationService struct {
	doctors       DoctorRepository
	verifications DoctorVerificationRepository
	tx            TransactionManager
}

func NewDoctorVerificationService(
	doctors DoctorRepository,
	verifications DoctorVerificationRepository,
	tx TransactionManager,
) (*DoctorVerificationService, error) {
	if doctors == nil {
		return nil, apperrors.ErrNilDoctorRepository
	}
	if verifications == nil {
		return nil, apperrors.ErrNilDoctorVerificationRepository
	}
	if tx == nil {
		return nil, apperrors.ErrNilTransactionManager
	}

	return &DoctorVerificationService{
		doctors:       doctors,
		verifications: verifications,
		tx:            tx,
	}, nil
}

func (dvs *DoctorVerificationService) Verify(
	ctx context.Context,
	actingUserID uint64,
	doctorID uint64,
) (*models.Doctor, error) {
	return dvs.transition(ctx, actingUserID, doctorID, verifyTransition, "")
}

func (dvs *DoctorVerificationService) Reject(
	ctx context.Context,
	actingUserID uint64,
	doctorID uint64,
	reason string,
) (*models.Doctor, error) {
	return dvs.transition(ctx, actingUserID, doctorID, rejectTransition, reason)
}

func (dvs *DoctorVerificationService) Suspend(
	ctx context.Context,
	actingUserID uint64,
	doctorID uint64,
	reason string,
) (*models.Doctor, error) {
	return dvs.transition(ctx, actingUserID, doctorID, suspendTransition, reason)
}

func (dvs *DoctorVerificationService) Reinstate(
	ctx context.Context,
	actingUserID uint64,
	doctorID uint64,
	reason string,
) (*models.Doctor, error) {
	return dvs.transition(ctx, actingUserID, doctorID, reinstateTransition, reason)
}

// List returns doctors in any status for the review queue. An empty status
// lists every doctor.
func (dvs *DoctorVerificationService) List(
	ctx context.Context,
	status models.VerificationStatus,
	limit int,
	offset int,
) ([]*models.Doctor, error) {
	if status != "" && !validVerificationStatus(status) {
		return nil, apperrors.Invalid(apperrors.ErrInvalidDoctorInput, "status", "must be one of pending, verified, rejected, suspended")
	}

	limit, offset = clampPagination(limit, offset)

	doctors, err := dvs.doctors.ListByStatus(ctx, status, limit, offset)
	if err != nil {
		return nil, fmt.Errorf("list doctors for review: %w", err)
	}

	return doctors, nil
}

// GetByID returns a doctor in any status together with its verification history.
func (dvs *DoctorVerificationService) GetByID(ctx context.Context, doctorID uint64) (*models.Doctor, []*models.DoctorVerification, error) {
	if doctorID == 0 {
		return nil, nil, fmt.Errorf("get doctor without id: %w", apperrors.ErrInvalidDoctorInput)
	}

	doctor, err := dvs.doctors.GetByID(ctx, doctorID)
	if err != nil {
		if errors.Is(err, apperrors.ErrDoctorNotFound) {
			return nil, nil, err
		}
		return nil, nil, fmt.Errorf("get doctor for review: %w", err)
	}

	verifications, err := dvs.verifications.ListByDoctor(ctx, doctorID)
	if err != nil {
		return nil, nil, fmt.Errorf("list doctor verifications: %w", err)
	}

	return doctor, verifications, nil
}

func (dvs *DoctorVerificationService) transition(
	ctx context.Context,
	actingUserID uint64,
	doctorID uint64,
	transition verificationTransition,
	reason string,
) (*models.Doctor, error) {
	if err := ctx.Err(); err != nil {
		return nil, fmt.Errorf("review doctor context: %w", err)
	}
	if actingUserID == 0 || doctorID == 0 {
		return nil, fmt.Errorf("review doctor with missing ids: %w", apperrors.ErrInvalidDoctorInput)
	}

	reason = strings.TrimSpace(reason)
	if transition.reasonRequired && reason == "" {
		return nil, apperrors.ErrVerificationReasonRequired
	}

	var reviewed *models.Doctor
	err := dvs.tx.WithinTransaction(ctx, func(txCtx context.Context) error {
		doctor, err := dvs.doctors.GetByID(txCtx, doctorID)
		if err != nil {
			return err
		}
		if doctor.VerificationStatus != transition.from {
			return apperrors.Invalid(apperrors.ErrInvalidVerificationTransition, "", fmt.Sprintf("doctor is %s; this action needs %s", doctor.VerificationStatus, transition.from))
		}

		verifiedAt := doctor.VerifiedAt
		if transition.to == models.VerificationStatusVerified {
			now := time.Now().UTC()
			verifiedAt = &now
		}

		if err := dvs.doctors.UpdateStatus(txCtx, doctorID, transition.from, transition.to, verifiedAt); err != nil {
			return err
		}
		if err := dvs.verifications.Create(txCtx, &models.DoctorVerification{
			DoctorID:         doctorID,
			ReviewedByUserID: actingUserID,
			FromStatus:       transition.from,
			ToStatus:         transition.to,
			Reason:           reason,
		}); err != nil {
			return err
		}

		doctor.VerificationStatus = transition.to
		doctor.VerifiedAt = verifiedAt
		reviewed = doctor
		return nil
	})
	if err != nil {
		if errors.Is(err, apperrors.ErrDoctorNotFound) ||
			errors.Is(err, apperrors.ErrInvalidVerificationTransition) {
			return nil, err
		}
		return nil, fmt.Errorf("review doctor %d: %w", doctorID, err)
	}

	return reviewed, nil
}

func validVerificationStatus(status models.VerificationStatus) bool {
	switch status {
	case models.VerificationStatusPending, models.VerificationStatusVerified,
		models.VerificationStatusRejected, models.VerificationStatusSuspended:
		return true
	default:
		return false
	}
}
