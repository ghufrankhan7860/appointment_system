package services

import (
	"context"
	"errors"
	"fmt"

	"appointment_system/internal/apps/appointment/apperrors"
	"appointment_system/internal/apps/appointment/models"
)

// ClinicActor answers which doctor the acting user runs the clinic desk for:
// a doctor runs their own, a staff account the one doctor it works for. Every
// desk action (the doctor's day, booking at the desk, cancelling, payments to
// check) resolves the doctor here, so staff never reach records, calls or the
// doctor's profile, which keep asking for a doctor account.
type ClinicActor struct {
	doctors        DoctorRepository
	staff          StaffRepository
	clinicPatients ClinicPatientRepository
}

func NewClinicActor(doctors DoctorRepository, staff StaffRepository, clinicPatients ClinicPatientRepository) (*ClinicActor, error) {
	if doctors == nil {
		return nil, apperrors.ErrNilDoctorRepository
	}
	if staff == nil {
		return nil, apperrors.ErrNilStaffRepository
	}
	if clinicPatients == nil {
		return nil, apperrors.ErrNilClinicPatientRepository
	}

	return &ClinicActor{doctors: doctors, staff: staff, clinicPatients: clinicPatients}, nil
}

// DeskDoctor returns the doctor whose desk the acting user runs. Any role but
// doctor and staff is ErrDoctorRoleRequired; a staff account no doctor has now
// is ErrStaffNotLinked, which also matches ErrDoctorRoleRequired.
func (ca *ClinicActor) DeskDoctor(ctx context.Context, actingUserID uint64, actingRole models.Role) (*models.Doctor, error) {
	if actingUserID == 0 {
		return nil, fmt.Errorf("desk action without acting user: %w", apperrors.ErrInvalidDeskInput)
	}

	switch actingRole {
	case models.RoleDoctor:
		doctor, err := ca.doctors.GetByUserID(ctx, actingUserID)
		if err != nil {
			if errors.Is(err, apperrors.ErrDoctorNotFound) {
				return nil, err
			}
			return nil, fmt.Errorf("get own doctor profile: %w", err)
		}
		return doctor, nil
	case models.RoleStaff:
		link, err := ca.staff.GetActiveByUser(ctx, actingUserID)
		if err != nil {
			if errors.Is(err, apperrors.ErrStaffNotFound) {
				return nil, fmt.Errorf("%w: %w", apperrors.ErrStaffNotLinked, apperrors.ErrDoctorRoleRequired)
			}
			return nil, fmt.Errorf("get staff link: %w", err)
		}
		doctor, err := ca.doctors.GetByID(ctx, link.DoctorID)
		if err != nil {
			return nil, fmt.Errorf("get staff doctor: %w", err)
		}
		return doctor, nil
	default:
		return nil, apperrors.ErrDoctorRoleRequired
	}
}

// IsDeskOf reports whether the acting user runs doctorID's desk. Other users,
// and staff no doctor has, are simply not.
func (ca *ClinicActor) IsDeskOf(ctx context.Context, actingUserID uint64, actingRole models.Role, doctorID uint64) (bool, error) {
	if actingRole != models.RoleDoctor && actingRole != models.RoleStaff {
		return false, nil
	}

	doctor, err := ca.DeskDoctor(ctx, actingUserID, actingRole)
	if err != nil {
		if errors.Is(err, apperrors.ErrDoctorNotFound) || errors.Is(err, apperrors.ErrDoctorRoleRequired) {
			return false, nil
		}
		return false, err
	}

	return doctor.ID == doctorID, nil
}

// HasPatient reports whether the doctor's desk has seen the patient: shown
// their pass, or added them as a walk-in.
func (ca *ClinicActor) HasPatient(ctx context.Context, doctorID uint64, patientID uint64) (bool, error) {
	if _, err := ca.clinicPatients.Get(ctx, doctorID, patientID); err != nil {
		if errors.Is(err, apperrors.ErrPatientNotAtClinic) {
			return false, nil
		}
		return false, fmt.Errorf("get clinic patient: %w", err)
	}

	return true, nil
}

// validateDeskActor accepts a doctor or a staff account, as validateDoctorActor
// accepts only a doctor.
func validateDeskActor(actingUserID uint64, actingRole models.Role) error {
	if actingUserID == 0 {
		return fmt.Errorf("desk action without acting user: %w", apperrors.ErrInvalidDeskInput)
	}
	if actingRole != models.RoleDoctor && actingRole != models.RoleStaff {
		return apperrors.ErrDoctorRoleRequired
	}

	return nil
}
