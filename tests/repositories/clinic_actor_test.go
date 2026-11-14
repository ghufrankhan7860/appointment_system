package repositories_test

import (
	"testing"

	"appointment_system/internal/apps/appointment/repositories"
	"appointment_system/internal/apps/appointment/services"

	"gorm.io/gorm"
)

// newTestClinicActor resolves desk users against the database.
func newTestClinicActor(t *testing.T, db *gorm.DB, doctors services.DoctorRepository) *services.ClinicActor {
	t.Helper()

	staff, err := repositories.NewStaffRepository(db)
	if err != nil {
		t.Fatalf("NewStaffRepository returned error: %v", err)
	}
	clinicPatients, err := repositories.NewClinicPatientRepository(db)
	if err != nil {
		t.Fatalf("NewClinicPatientRepository returned error: %v", err)
	}
	actor, err := services.NewClinicActor(doctors, staff, clinicPatients)
	if err != nil {
		t.Fatalf("NewClinicActor returned error: %v", err)
	}

	return actor
}
