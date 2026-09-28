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

const (
	defaultPatientListLimit = 20
	maxPatientListLimit     = 100
)

// PatientService owns user-scoped patient use cases, authorization, and
// cross-repository orchestration. It never deletes a canonical patient;
// access removal is a link revoke owned elsewhere.
type PatientService struct {
	patients PatientRepository
	links    UserPatientRepository
	tx       TransactionManager
}

func NewPatientService(
	patients PatientRepository,
	links UserPatientRepository,
	tx TransactionManager,
) (*PatientService, error) {
	if patients == nil {
		return nil, apperrors.ErrNilPatientRepository
	}
	if links == nil {
		return nil, apperrors.ErrNilUserPatientRepository
	}
	if tx == nil {
		return nil, apperrors.ErrNilTransactionManager
	}
	return &PatientService{patients: patients, links: links, tx: tx}, nil
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
		return nil, fmt.Errorf("create patient with relationship %q: %w", relationship, apperrors.ErrInvalidPatientInput)
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
		if _, err := s.links.Create(txCtx, link); err != nil {
			if errors.Is(err, apperrors.ErrUserPatientAlreadyExists) {
				return err
			}
			return fmt.Errorf("create user-patient link: %w", err)
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

	return created, nil
}

func (s *PatientService) GetByID(
	ctx context.Context,
	actingUserID uint64,
	patientID uint64,
) (*models.Patient, error) {
	if err := ctx.Err(); err != nil {
		return nil, fmt.Errorf("get patient context: %w", err)
	}
	if actingUserID == 0 || patientID == 0 {
		return nil, fmt.Errorf("get patient with missing ids: %w", apperrors.ErrInvalidPatientInput)
	}

	if _, err := s.links.GetActiveByUserAndPatient(ctx, actingUserID, patientID); err != nil {
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

	return patient, nil
}

func (s *PatientService) List(
	ctx context.Context,
	actingUserID uint64,
	limit int,
	offset int,
) ([]*models.Patient, error) {
	if err := ctx.Err(); err != nil {
		return nil, fmt.Errorf("list patients context: %w", err)
	}
	if actingUserID == 0 {
		return nil, fmt.Errorf("list patients without acting user: %w", apperrors.ErrInvalidPatientInput)
	}

	limit, offset = clampPatientPagination(limit, offset)

	links, err := s.links.ListByUser(ctx, actingUserID, limit, offset)
	if err != nil {
		return nil, fmt.Errorf("list user-patient links: %w", err)
	}
	if len(links) == 0 {
		return []*models.Patient{}, nil
	}

	ids := make([]uint64, 0, len(links))
	for _, link := range links {
		if link == nil {
			continue
		}
		if link.Status != models.UserPatientStatusActive {
			continue
		}
		ids = append(ids, link.PatientID)
	}
	if len(ids) == 0 {
		return []*models.Patient{}, nil
	}

	byID, err := s.patients.GetByIDs(ctx, ids)
	if err != nil {
		return nil, fmt.Errorf("get patients by ids: %w", err)
	}

	result := make([]*models.Patient, 0, len(ids))
	for _, id := range ids {
		patient, ok := byID[id]
		if !ok || patient == nil {
			continue
		}
		result = append(result, patient)
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

	link, err := s.links.GetActiveByUserAndPatient(ctx, actingUserID, patient.ID)
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

	return updated, nil
}

// ------------------------------- Helpers ------------------------------------
func validatePatientDemographics(patient *models.Patient) error {
	if strings.TrimSpace(patient.Name) == "" {
		return fmt.Errorf("patient name is required: %w", apperrors.ErrInvalidPatientInput)
	}
	if patient.DateOfBirth.IsZero() {
		return fmt.Errorf("patient date of birth is required: %w", apperrors.ErrInvalidPatientInput)
	}
	if patient.DateOfBirth.After(time.Now().UTC()) {
		return fmt.Errorf("patient date of birth is in the future: %w", apperrors.ErrInvalidPatientInput)
	}
	if !validGender(patient.Gender) {
		return fmt.Errorf("patient gender %q is invalid: %w", patient.Gender, apperrors.ErrInvalidPatientInput)
	}
	if !validBloodType(patient.BloodType) {
		return fmt.Errorf("patient blood type %q is invalid: %w", patient.BloodType, apperrors.ErrInvalidPatientInput)
	}
	return nil
}

func validGender(gender models.Gender) bool {
	switch gender {
	case models.GenderMale, models.GenderFemale, models.GenderOther, models.GenderUnknown:
		return true
	default:
		return false
	}
}

func validBloodType(bloodType models.BloodType) bool {
	switch bloodType {
	case models.BloodTypeAPlus, models.BloodTypeAMinus,
		models.BloodTypeBPlus, models.BloodTypeBMinus,
		models.BloodTypeABPlus, models.BloodTypeABMinus,
		models.BloodTypeOPlus, models.BloodTypeOMinus:
		return true
	default:
		return false
	}
}

func validRelationship(relationship models.Relationship) bool {
	switch relationship {
	case models.RelationshipSelf, models.RelationshipFather,
		models.RelationshipMother, models.RelationshipSpouse,
		models.RelationshipChild, models.RelationshipSibling,
		models.RelationshipGuardian, models.RelationshipCaregiver,
		models.RelationshipOther:
		return true
	default:
		return false
	}
}

func clampPatientPagination(limit int, offset int) (int, int) {
	if limit <= 0 {
		limit = defaultPatientListLimit
	}
	if limit > maxPatientListLimit {
		limit = maxPatientListLimit
	}
	if offset < 0 {
		offset = 0
	}
	return limit, offset
}
