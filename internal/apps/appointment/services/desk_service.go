package services

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"appointment_system/internal/apps/appointment/apperrors"
	"appointment_system/internal/apps/appointment/constant"
	"appointment_system/internal/apps/appointment/models"
	"appointment_system/internal/apps/appointment/utils"
)

// DeskService is the clinic desk's side of a visit: the pass a patient shows
// at the desk (a QR token and a short code that work for
// constant.PatientPassTTL), finding the patient from it, and adding a walk-in
// who has no app. A patient the desk has seen either way can be booked there
// (AppointmentService.BookAtDesk). Only hashes of the token and code are kept.
type DeskService struct {
	clinic         *ClinicActor
	patients       PatientRepository
	userPatients   UserPatientRepository
	passes         PatientPassRepository
	attempts       PatientPassAttemptRepository
	clinicPatients ClinicPatientRepository
	invites        *PatientInviteService
	tx             TransactionManager
	now            func() time.Time
	logger         Logger
}

func NewDeskService(
	clinic *ClinicActor,
	patients PatientRepository,
	userPatients UserPatientRepository,
	passes PatientPassRepository,
	attempts PatientPassAttemptRepository,
	clinicPatients ClinicPatientRepository,
	invites *PatientInviteService,
	tx TransactionManager,
	now func() time.Time,
	logger Logger,
) (*DeskService, error) {
	if clinic == nil {
		return nil, apperrors.ErrNilClinicActor
	}
	if patients == nil {
		return nil, apperrors.ErrNilPatientRepository
	}
	if userPatients == nil {
		return nil, apperrors.ErrNilUserPatientRepository
	}
	if passes == nil {
		return nil, apperrors.ErrNilPatientPassRepository
	}
	if attempts == nil {
		return nil, apperrors.ErrNilPatientPassAttemptRepository
	}
	if clinicPatients == nil {
		return nil, apperrors.ErrNilClinicPatientRepository
	}
	if invites == nil {
		return nil, apperrors.ErrNilPatientInviteService
	}
	if tx == nil {
		return nil, apperrors.ErrNilTransactionManager
	}
	if now == nil {
		return nil, apperrors.ErrNilClock
	}
	if logger == nil {
		return nil, apperrors.ErrNilLogger
	}

	return &DeskService{
		clinic:         clinic,
		patients:       patients,
		userPatients:   userPatients,
		passes:         passes,
		attempts:       attempts,
		clinicPatients: clinicPatients,
		invites:        invites,
		tx:             tx,
		now:            now,
		logger:         logger,
	}, nil
}

// IssuePass gives the patient side a new pass for a patient they can book
// for, replacing the patient's old one so its QR and code stop working. It
// returns the pass with its token and code, which exist only in this answer.
func (ds *DeskService) IssuePass(
	ctx context.Context,
	actingUserID uint64,
	patientID uint64,
) (*models.PatientPass, string, string, error) {
	if actingUserID == 0 || patientID == 0 {
		return nil, "", "", fmt.Errorf("issue pass with missing ids: %w", apperrors.ErrInvalidDeskInput)
	}

	link, err := ds.userPatients.GetActiveByUserAndPatient(ctx, actingUserID, patientID)
	if err != nil {
		if errors.Is(err, apperrors.ErrUserPatientNotFound) {
			return nil, "", "", apperrors.ErrPatientNotFound
		}
		return nil, "", "", fmt.Errorf("authorize patient pass: %w", err)
	}
	if !canBook(link) {
		return nil, "", "", apperrors.ErrPatientForbidden
	}

	for try := 1; ; try++ {
		token, err := utils.NewURLToken(constant.PatientPassTokenBytes)
		if err != nil {
			return nil, "", "", fmt.Errorf("create pass token: %w", err)
		}
		code, err := utils.NewCode(constant.PatientInviteCodeAlphabet, constant.PatientInviteCodeLength)
		if err != nil {
			return nil, "", "", fmt.Errorf("create pass code: %w", err)
		}

		pass, err := ds.passes.Replace(ctx, &models.PatientPass{
			PatientID:       patientID,
			CreatedByUserID: actingUserID,
			TokenHash:       utils.HashToken(token),
			CodeHash:        utils.HashToken(code),
			ExpiresAt:       ds.now().Add(constant.PatientPassTTL),
		})
		if errors.Is(err, apperrors.ErrPatientPassCodeExists) && try < constant.MaxPatientPassCodeTries {
			continue
		}
		if err != nil {
			return nil, "", "", fmt.Errorf("replace patient pass: %w", err)
		}

		return pass, token, code, nil
	}
}

// LookupPass finds the patient behind a pass the desk scanned (its token) or
// typed (its code), and lets the desk book for them from now on. A wrong or
// expired value is one ErrPatientPassInvalid and counts against the desk user;
// past constant.MaxFailedPatientPassAttempts within
// constant.PatientPassAttemptWindow it is ErrPatientPassAttemptsExceeded.
func (ds *DeskService) LookupPass(
	ctx context.Context,
	actingUserID uint64,
	actingRole models.Role,
	value string,
) (*models.Patient, error) {
	value = strings.TrimSpace(value)
	if value == "" {
		return nil, apperrors.Invalid(apperrors.ErrInvalidDeskInput, "code", "is required")
	}
	doctor, err := ds.clinic.DeskDoctor(ctx, actingUserID, actingRole)
	if err != nil {
		return nil, err
	}

	now := ds.now()
	failed, err := ds.attempts.CountSince(ctx, actingUserID, now.Add(-constant.PatientPassAttemptWindow))
	if err != nil {
		return nil, fmt.Errorf("count patient pass attempts: %w", err)
	}
	if failed >= constant.MaxFailedPatientPassAttempts {
		return nil, apperrors.ErrPatientPassAttemptsExceeded
	}

	var pass *models.PatientPass
	if code := normalizeInviteCode(value); len(code) == constant.PatientInviteCodeLength {
		pass, err = ds.passes.GetByCodeHash(ctx, utils.HashToken(code))
	} else {
		pass, err = ds.passes.GetByTokenHash(ctx, utils.HashToken(value))
	}
	if err == nil && !now.Before(pass.ExpiresAt) {
		err = apperrors.ErrPatientPassInvalid
	}
	if err != nil {
		if errors.Is(err, apperrors.ErrPatientPassInvalid) {
			ds.recordFailedAttempt(ctx, actingUserID, now)
			return nil, err
		}
		return nil, fmt.Errorf("get patient pass: %w", err)
	}

	if err := ds.clinicPatients.Add(ctx, &models.ClinicPatient{
		DoctorID:        doctor.ID,
		PatientID:       pass.PatientID,
		Source:          models.ClinicPatientSourcePass,
		CreatedByUserID: actingUserID,
	}); err != nil {
		return nil, fmt.Errorf("add clinic patient: %w", err)
	}
	patient, err := ds.patients.GetByID(ctx, pass.PatientID)
	if err != nil {
		return nil, fmt.Errorf("get pass patient: %w", err)
	}
	ds.logger.InfoContext(ctx, "patient pass shown at desk", "patient_id", patient.ID, "doctor_id", doctor.ID, "role", actingRole)

	return patient, nil
}

// CreateWalkIn adds a patient who has no app, for the desk to book. Nobody
// holds the patient until someone redeems a claim code (WalkInClaimCode).
// EmergencyContact carries the contact number the desk took.
func (ds *DeskService) CreateWalkIn(
	ctx context.Context,
	actingUserID uint64,
	actingRole models.Role,
	patient *models.Patient,
) (*models.Patient, error) {
	if patient == nil {
		return nil, apperrors.ErrNilPatientRequest
	}
	patient.Name = strings.TrimSpace(patient.Name)
	patient.EmergencyContact = strings.TrimSpace(patient.EmergencyContact)
	if err := validatePatientDemographics(patient); err != nil {
		return nil, err
	}
	doctor, err := ds.clinic.DeskDoctor(ctx, actingUserID, actingRole)
	if err != nil {
		return nil, err
	}

	var created *models.Patient
	err = ds.tx.WithinTransaction(ctx, func(txCtx context.Context) error {
		created, err = ds.patients.Create(txCtx, patient)
		if err != nil {
			return err
		}

		return ds.clinicPatients.Add(txCtx, &models.ClinicPatient{
			DoctorID:        doctor.ID,
			PatientID:       created.ID,
			Source:          models.ClinicPatientSourceWalkIn,
			CreatedByUserID: actingUserID,
		})
	})
	if err != nil {
		return nil, fmt.Errorf("create walk-in patient: %w", err)
	}
	ds.logger.InfoContext(ctx, "walk-in patient added", "patient_id", created.ID, "doctor_id", doctor.ID, "role", actingRole)

	return created, nil
}

// WalkInClaimCode issues a one-time code with which the walk-in, or whoever
// looks after them (relationship), takes the patient over in the app. Only the
// desk that added the walk-in can, and only while nobody holds the patient.
func (ds *DeskService) WalkInClaimCode(
	ctx context.Context,
	actingUserID uint64,
	actingRole models.Role,
	patientID uint64,
	relationship models.Relationship,
) (*models.PatientInvite, string, error) {
	if patientID == 0 {
		return nil, "", fmt.Errorf("claim code without patient id: %w", apperrors.ErrInvalidDeskInput)
	}
	doctor, err := ds.clinic.DeskDoctor(ctx, actingUserID, actingRole)
	if err != nil {
		return nil, "", err
	}

	seen, err := ds.clinicPatients.Get(ctx, doctor.ID, patientID)
	if err != nil {
		if errors.Is(err, apperrors.ErrPatientNotAtClinic) {
			return nil, "", apperrors.ErrPatientNotFound
		}
		return nil, "", fmt.Errorf("get clinic patient: %w", err)
	}
	if seen.Source != models.ClinicPatientSourceWalkIn {
		return nil, "", apperrors.Invalid(apperrors.ErrPatientAlreadyClaimed, "", "this patient already uses the app")
	}

	invite, code, err := ds.invites.CreateUnclaimed(ctx, actingUserID, patientID, relationship)
	if err != nil {
		if errors.Is(err, apperrors.ErrPatientAlreadyClaimed) {
			return nil, "", apperrors.Invalid(apperrors.ErrPatientAlreadyClaimed, "", "someone has already taken this patient over in the app")
		}
		return nil, "", err
	}

	return invite, code, nil
}

func (ds *DeskService) recordFailedAttempt(ctx context.Context, actingUserID uint64, now time.Time) {
	if err := ds.attempts.Create(ctx, actingUserID, now); err != nil {
		ds.logger.WarnContext(ctx, "record patient pass attempt failed", "error", err)
	}
	if err := ds.attempts.DeleteBefore(ctx, actingUserID, now.Add(-constant.PatientPassAttemptWindow)); err != nil {
		ds.logger.WarnContext(ctx, "prune patient pass attempts failed", "error", err)
	}
}
