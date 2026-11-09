package services

import (
	"context"
	"errors"
	"fmt"

	"appointment_system/internal/apps/appointment/apperrors"
	"appointment_system/internal/apps/appointment/models"
)

// PatientService owns user-scoped patient use cases, authorization, and
// cross-repository orchestration. It never deletes a canonical patient;
// access removal is a link revoke owned elsewhere.
type PatientService struct {
	patients     PatientRepository
	userPatients UserPatientRepository
	history      UserPatientHistoryRepository
	tx           TransactionManager
	logger       Logger
}

func NewPatientService(
	patients PatientRepository,
	userPatients UserPatientRepository,
	history UserPatientHistoryRepository,
	tx TransactionManager,
	logger Logger,
) (*PatientService, error) {
	if patients == nil {
		return nil, apperrors.ErrNilPatientRepository
	}
	if userPatients == nil {
		return nil, apperrors.ErrNilUserPatientRepository
	}
	if history == nil {
		return nil, apperrors.ErrNilUserPatientHistoryRepository
	}
	if tx == nil {
		return nil, apperrors.ErrNilTransactionManager
	}
	if logger == nil {
		return nil, apperrors.ErrNilLogger
	}
	return &PatientService{patients: patients, userPatients: userPatients, history: history, tx: tx, logger: logger}, nil
}

func (s *PatientService) Create(
	ctx context.Context,
	actingUserID uint64,
	patient *models.Patient,
	relationship models.Relationship,
) (*models.Patient, error) {
	if err := ctx.Err(); err != nil {
		return nil, fmt.Errorf("create patient context: %w", err)
	}
	if actingUserID == 0 {
		return nil, fmt.Errorf("create patient without acting user: %w", apperrors.ErrInvalidPatientInput)
	}
	if patient == nil {
		return nil, apperrors.ErrNilPatientRequest
	}
	if err := validatePatientDemographics(patient); err != nil {
		return nil, err
	}
	if !validRelationship(relationship) {
		return nil, apperrors.Invalid(apperrors.ErrInvalidPatientInput, "relationship", "must be one of self, father, mother, spouse, child, sibling, guardian, caregiver, other")
	}

	var created *models.Patient
	err := s.tx.WithinTransaction(ctx, func(txCtx context.Context) error {
		next, err := s.patients.Create(txCtx, patient)
		if err != nil {
			return fmt.Errorf("create patient record: %w", err)
		}
		link := &models.UserPatient{
			UserID:              actingUserID,
			PatientID:           next.ID,
			PatientRelationship: relationship,
			AccessLevel:         models.AccessLevelFull,
			Status:              models.UserPatientStatusActive,
		}
		if _, err := s.userPatients.Create(txCtx, link); err != nil {
			if errors.Is(err, apperrors.ErrUserPatientAlreadyExists) {
				return err
			}
			return fmt.Errorf("create user-patient link: %w", err)
		}
		if err := s.history.Create(txCtx, &models.UserPatientHistory{
			UserID:          actingUserID,
			PatientID:       next.ID,
			ChangedByUserID: actingUserID,
			ToStatus:        link.Status,
			ToAccessLevel:   link.AccessLevel,
			Reason:          models.UserPatientChangeCreated,
		}); err != nil {
			return fmt.Errorf("create user-patient history: %w", err)
		}
		created = next
		return nil
	})
	if err != nil {
		if errors.Is(err, apperrors.ErrUserPatientAlreadyExists) {
			return nil, err
		}
		if errors.Is(err, apperrors.ErrNilPatientRequest) ||
			errors.Is(err, apperrors.ErrInvalidPatientInput) {
			return nil, err
		}
		return nil, err
	}

	s.logger.InfoContext(ctx, "patient created", "patient_id", created.ID, "relationship", relationship)

	return created, nil
}

func (s *PatientService) GetByID(
	ctx context.Context,
	actingUserID uint64,
	patientID uint64,
) (*models.LinkedPatient, error) {
	if err := ctx.Err(); err != nil {
		return nil, fmt.Errorf("get patient context: %w", err)
	}
	if actingUserID == 0 || patientID == 0 {
		return nil, fmt.Errorf("get patient with missing ids: %w", apperrors.ErrInvalidPatientInput)
	}

	link, err := s.userPatients.GetActiveByUserAndPatient(ctx, actingUserID, patientID)
	if err != nil {
		if errors.Is(err, apperrors.ErrUserPatientNotFound) {
			return nil, apperrors.ErrPatientNotFound
		}
		return nil, fmt.Errorf("authorize patient read: %w", err)
	}

	patient, err := s.patients.GetByID(ctx, patientID)
	if err != nil {
		if errors.Is(err, apperrors.ErrPatientNotFound) {
			return nil, err
		}
		return nil, fmt.Errorf("get patient by id: %w", err)
	}

	return &models.LinkedPatient{Patient: *patient, Link: *link}, nil
}

func (s *PatientService) List(
	ctx context.Context,
	actingUserID uint64,
	limit int,
	offset int,
) ([]*models.LinkedPatient, error) {
	if err := ctx.Err(); err != nil {
		return nil, fmt.Errorf("list patients context: %w", err)
	}
	if actingUserID == 0 {
		return nil, fmt.Errorf("list patients without acting user: %w", apperrors.ErrInvalidPatientInput)
	}

	limit, offset = clampPagination(limit, offset)

	userPatients, err := s.userPatients.ListActiveByUser(ctx, actingUserID, limit, offset)
	if err != nil {
		return nil, fmt.Errorf("list active user-patients: %w", err)
	}
	if len(userPatients) == 0 {
		return []*models.LinkedPatient{}, nil
	}

	ids := make([]uint64, 0, len(userPatients))
	for _, link := range userPatients {
		if link == nil {
			continue
		}
		ids = append(ids, link.PatientID)
	}
	if len(ids) == 0 {
		return []*models.LinkedPatient{}, nil
	}

	byID, err := s.patients.GetByIDs(ctx, ids)
	if err != nil {
		return nil, fmt.Errorf("get patients by ids: %w", err)
	}

	result := make([]*models.LinkedPatient, 0, len(ids))
	for _, link := range userPatients {
		if link == nil {
			continue
		}
		patient, ok := byID[link.PatientID]
		if !ok || patient == nil {
			continue
		}
		result = append(result, &models.LinkedPatient{Patient: *patient, Link: *link})
	}

	return result, nil
}

func (s *PatientService) Update(
	ctx context.Context,
	actingUserID uint64,
	patient *models.Patient,
) (*models.Patient, error) {
	if err := ctx.Err(); err != nil {
		return nil, fmt.Errorf("update patient context: %w", err)
	}
	if actingUserID == 0 {
		return nil, fmt.Errorf("update patient without acting user: %w", apperrors.ErrInvalidPatientInput)
	}
	if patient == nil {
		return nil, apperrors.ErrNilPatientRequest
	}
	if patient.ID == 0 {
		return nil, fmt.Errorf("update patient without id: %w", apperrors.ErrInvalidPatientInput)
	}
	if err := validatePatientDemographics(patient); err != nil {
		return nil, err
	}

	link, err := s.userPatients.GetActiveByUserAndPatient(ctx, actingUserID, patient.ID)
	if err != nil {
		if errors.Is(err, apperrors.ErrUserPatientNotFound) {
			return nil, apperrors.ErrPatientNotFound
		}
		return nil, fmt.Errorf("authorize patient update: %w", err)
	}
	if link.AccessLevel != models.AccessLevelFull {
		return nil, apperrors.ErrPatientForbidden
	}

	updated, err := s.patients.Update(ctx, &models.Patient{
		ID:               patient.ID,
		Name:             patient.Name,
		Gender:           patient.Gender,
		DateOfBirth:      patient.DateOfBirth,
		BloodType:        patient.BloodType,
		EmergencyContact: patient.EmergencyContact,
	})
	if err != nil {
		if errors.Is(err, apperrors.ErrPatientNotFound) {
			return nil, err
		}
		return nil, fmt.Errorf("update patient record: %w", err)
	}

	s.logger.InfoContext(ctx, "patient updated", "patient_id", updated.ID)

	return updated, nil
}
