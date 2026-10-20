package services

import (
	"context"
	"errors"
	"fmt"
	"regexp"
	"strings"

	"appointment_system/internal/apps/appointment/apperrors"
	"appointment_system/internal/apps/appointment/models"
	"appointment_system/internal/apps/appointment/utils"
)

var specialitySlugPattern = regexp.MustCompile(`^[a-z0-9]+(-[a-z0-9]+)*$`)

// DoctorService owns doctor profiles: a doctor account creates and edits its
// own profile, and everyone else sees only verified doctors. Verification
// decisions belong to DoctorVerificationService.
type DoctorService struct {
	doctors       DoctorRepository
	specialities  SpecialityRepository
	verifications DoctorVerificationRepository
	tx            TransactionManager
}

func NewDoctorService(
	doctors DoctorRepository,
	specialities SpecialityRepository,
	verifications DoctorVerificationRepository,
	tx TransactionManager,
) (*DoctorService, error) {
	if doctors == nil {
		return nil, apperrors.ErrNilDoctorRepository
	}
	if specialities == nil {
		return nil, apperrors.ErrNilSpecialityRepository
	}
	if verifications == nil {
		return nil, apperrors.ErrNilDoctorVerificationRepository
	}
	if tx == nil {
		return nil, apperrors.ErrNilTransactionManager
	}

	return &DoctorService{
		doctors:       doctors,
		specialities:  specialities,
		verifications: verifications,
		tx:            tx,
	}, nil
}

// Create stores the acting doctor's profile in pending state. specialitySlugs
// are resolved against the lookup table; the first one is the primary.
func (ds *DoctorService) Create(
	ctx context.Context,
	actingUserID uint64,
	actingRole models.Role,
	doctor *models.Doctor,
	specialitySlugs []string,
) (*models.Doctor, error) {
	if err := ctx.Err(); err != nil {
		return nil, fmt.Errorf("create doctor context: %w", err)
	}
	if err := validateDoctorActor(actingUserID, actingRole); err != nil {
		return nil, err
	}
	if doctor == nil {
		return nil, apperrors.ErrNilDoctorRequest
	}
	if err := normalizeDoctorProfile(doctor); err != nil {
		return nil, err
	}

	_, err := ds.doctors.GetByUserID(ctx, actingUserID)
	if err == nil {
		return nil, apperrors.ErrDoctorAlreadyExists
	}
	if !errors.Is(err, apperrors.ErrDoctorNotFound) {
		return nil, fmt.Errorf("check existing doctor profile: %w", err)
	}

	specialities, err := ds.resolveSpecialities(ctx, specialitySlugs)
	if err != nil {
		return nil, err
	}

	doctor.ID = 0
	doctor.UserID = actingUserID
	doctor.Specialities = specialities
	doctor.VerificationStatus = models.VerificationStatusPending
	doctor.VerifiedAt = nil
	doctor.IsAcceptingPatients = true

	var created *models.Doctor
	if err := ds.tx.WithinTransaction(ctx, func(txCtx context.Context) error {
		next, err := ds.doctors.Create(txCtx, doctor)
		if err != nil {
			return err
		}
		created = next
		return nil
	}); err != nil {
		return nil, fmt.Errorf("create doctor profile: %w", err)
	}

	return created, nil
}

func (ds *DoctorService) GetMe(
	ctx context.Context,
	actingUserID uint64,
	actingRole models.Role,
) (*models.Doctor, error) {
	if err := validateDoctorActor(actingUserID, actingRole); err != nil {
		return nil, err
	}

	doctor, err := ds.doctors.GetByUserID(ctx, actingUserID)
	if err != nil {
		if errors.Is(err, apperrors.ErrDoctorNotFound) {
			return nil, err
		}
		return nil, fmt.Errorf("get own doctor profile: %w", err)
	}

	return doctor, nil
}

// Update replaces the acting doctor's profile. A rejected profile goes back to
// pending (the doctor's resubmission), and a verified doctor who changes the
// registration details goes back to pending for a new review. Each such change
// is recorded with the doctor as the actor. A nil isAcceptingPatients keeps the
// current value.
func (ds *DoctorService) Update(
	ctx context.Context,
	actingUserID uint64,
	actingRole models.Role,
	doctor *models.Doctor,
	specialitySlugs []string,
	isAcceptingPatients *bool,
) (*models.Doctor, error) {
	if err := ctx.Err(); err != nil {
		return nil, fmt.Errorf("update doctor context: %w", err)
	}
	if err := validateDoctorActor(actingUserID, actingRole); err != nil {
		return nil, err
	}
	if doctor == nil {
		return nil, apperrors.ErrNilDoctorRequest
	}
	if err := normalizeDoctorProfile(doctor); err != nil {
		return nil, err
	}

	existing, err := ds.doctors.GetByUserID(ctx, actingUserID)
	if err != nil {
		if errors.Is(err, apperrors.ErrDoctorNotFound) {
			return nil, err
		}
		return nil, fmt.Errorf("get doctor profile for update: %w", err)
	}

	specialities, err := ds.resolveSpecialities(ctx, specialitySlugs)
	if err != nil {
		return nil, err
	}

	doctor.ID = existing.ID
	doctor.UserID = existing.UserID
	doctor.Specialities = specialities
	doctor.VerificationStatus = existing.VerificationStatus
	doctor.VerifiedAt = existing.VerifiedAt
	doctor.IsAcceptingPatients = existing.IsAcceptingPatients
	if isAcceptingPatients != nil {
		doctor.IsAcceptingPatients = *isAcceptingPatients
	}

	registrationChanged := doctor.RegistrationNumber != existing.RegistrationNumber ||
		doctor.RegistrationCouncil != existing.RegistrationCouncil
	resubmit := existing.VerificationStatus == models.VerificationStatusRejected ||
		(existing.VerificationStatus == models.VerificationStatusVerified && registrationChanged)
	if resubmit {
		doctor.VerificationStatus = models.VerificationStatusPending
		doctor.VerifiedAt = nil
	}

	var updated *models.Doctor
	if err := ds.tx.WithinTransaction(ctx, func(txCtx context.Context) error {
		next, err := ds.doctors.Update(txCtx, doctor)
		if err != nil {
			return err
		}
		if resubmit {
			if err := ds.verifications.Create(txCtx, &models.DoctorVerification{
				DoctorID:         existing.ID,
				ReviewedByUserID: actingUserID,
				FromStatus:       existing.VerificationStatus,
				ToStatus:         models.VerificationStatusPending,
				Reason:           "profile resubmitted by doctor",
			}); err != nil {
				return err
			}
		}
		updated = next
		return nil
	}); err != nil {
		return nil, fmt.Errorf("update doctor profile: %w", err)
	}

	return updated, nil
}

// GetByID returns a verified doctor's public profile. Any other status is
// reported as not found.
func (ds *DoctorService) GetByID(ctx context.Context, doctorID uint64) (*models.Doctor, error) {
	if doctorID == 0 {
		return nil, fmt.Errorf("get doctor without id: %w", apperrors.ErrInvalidDoctorInput)
	}

	doctor, err := ds.doctors.GetByID(ctx, doctorID)
	if err != nil {
		if errors.Is(err, apperrors.ErrDoctorNotFound) {
			return nil, err
		}
		return nil, fmt.Errorf("get doctor by id: %w", err)
	}
	if doctor.VerificationStatus != models.VerificationStatusVerified {
		return nil, apperrors.ErrDoctorNotFound
	}

	return doctor, nil
}

func (ds *DoctorService) List(ctx context.Context, search models.DoctorSearch) ([]*models.Doctor, error) {
	if err := ctx.Err(); err != nil {
		return nil, fmt.Errorf("list doctors context: %w", err)
	}

	search.Query = strings.TrimSpace(search.Query)
	search.City = strings.TrimSpace(search.City)
	search.Specialities = utils.NormalizeValues(search.Specialities)
	search.Tags = utils.NormalizeValues(search.Tags)
	if search.Mode != "" && !validConsultationMode(search.Mode) {
		return nil, apperrors.Invalid(apperrors.ErrInvalidDoctorInput, "mode", "must be one of in_person, video, phone")
	}
	if search.FeeMax < 0 {
		return nil, apperrors.Invalid(apperrors.ErrInvalidDoctorInput, "feeMax", "cannot be negative")
	}
	search.Limit, search.Offset = clampPagination(search.Limit, search.Offset)

	doctors, err := ds.doctors.Search(ctx, search)
	if err != nil {
		return nil, fmt.Errorf("search doctors: %w", err)
	}

	return doctors, nil
}

func (ds *DoctorService) ListSpecialities(ctx context.Context) ([]*models.Speciality, error) {
	specialities, err := ds.specialities.List(ctx)
	if err != nil {
		return nil, fmt.Errorf("list specialities: %w", err)
	}

	return specialities, nil
}

func (ds *DoctorService) CreateSpeciality(
	ctx context.Context,
	slug string,
	name string,
) (*models.Speciality, error) {
	slug = strings.ToLower(strings.TrimSpace(slug))
	name = strings.TrimSpace(name)
	if !specialitySlugPattern.MatchString(slug) || name == "" {
		return nil, apperrors.Invalid(apperrors.ErrInvalidDoctorInput, "", "slug must be lowercase words joined by hyphens, and name is required")
	}

	created, err := ds.specialities.Create(ctx, &models.Speciality{Slug: slug, Name: name})
	if err != nil {
		if errors.Is(err, apperrors.ErrSpecialityAlreadyExists) {
			return nil, err
		}
		return nil, fmt.Errorf("create speciality: %w", err)
	}

	return created, nil
}

func (ds *DoctorService) resolveSpecialities(ctx context.Context, slugs []string) ([]models.Speciality, error) {
	slugs = utils.NormalizeValues(slugs)
	if len(slugs) == 0 {
		return nil, apperrors.Invalid(apperrors.ErrInvalidDoctorInput, "specialities", "must have at least one speciality")
	}

	found, err := ds.specialities.GetBySlugs(ctx, slugs)
	if err != nil {
		return nil, fmt.Errorf("resolve specialities: %w", err)
	}

	specialities := make([]models.Speciality, 0, len(slugs))
	for index, slug := range slugs {
		speciality, ok := found[slug]
		if !ok {
			return nil, apperrors.Invalid(apperrors.ErrSpecialityNotFound, "", fmt.Sprintf("speciality %s does not exist", slug))
		}
		next := *speciality
		next.IsPrimary = index == 0
		specialities = append(specialities, next)
	}

	return specialities, nil
}
