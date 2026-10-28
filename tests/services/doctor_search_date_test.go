package services_test

import (
	"context"
	"errors"
	"slices"
	"testing"
	"time"

	"appointment_system/internal/apps/appointment/apperrors"
	"appointment_system/internal/apps/appointment/constant"
	"appointment_system/internal/apps/appointment/models"
	"appointment_system/internal/apps/appointment/services"
)

func newTestDoctorSearch(t *testing.T, doctors *fakeDoctorRepo, rules []*models.AvailabilityRule, booked []*models.AppointmentDetails, now time.Time) *services.DoctorService {
	t.Helper()

	availability, err := services.NewAvailabilityService(doctors, &fakeAvailabilityRepo{rules: rules}, &fakeAppointmentRepo{booked: booked}, &fakeTxManager{}, clinicLocation(t), func() time.Time { return now })
	if err != nil {
		t.Fatalf("NewAvailabilityService returned error: %v", err)
	}
	service, err := services.NewDoctorService(doctors, knownSpecialities(), &fakeDoctorVerificationRepo{}, availability, &fakeTxManager{})
	if err != nil {
		t.Fatalf("NewDoctorService returned error: %v", err)
	}

	return service
}

func TestDoctorServiceListByDate(t *testing.T) {
	location := clinicLocation(t)
	now := time.Date(2026, 10, 25, 12, 0, 0, 0, location)
	monday := time.Date(2026, 10, 26, 0, 0, 0, 0, time.UTC)
	doctorWithID := func(id uint64) *models.Doctor {
		doctor := clinicDoctor(models.VerificationStatusVerified)
		doctor.ID = id
		return doctor
	}
	morning := func(doctorID uint64, endMinute int, mode models.ConsultationMode) *models.AvailabilityRule {
		return &models.AvailabilityRule{DoctorID: doctorID, DayOfWeek: time.Monday, StartMinute: 9 * 60, EndMinute: endMinute, Mode: mode}
	}
	rules := []*models.AvailabilityRule{
		morning(5, 10*60, models.ConsultationModeInPerson),
		morning(6, 10*60, models.ConsultationModeVideo),
		morning(8, 9*60+30, models.ConsultationModeInPerson),
	}
	fullyBooked := scheduledAt(time.Date(2026, 10, 26, 9, 0, 0, 0, location), models.AppointmentStatusScheduled)
	fullyBooked.DoctorID = 8
	past := now.AddDate(0, 0, -1)
	beyond := now.AddDate(0, 0, constant.SlotHorizonDays+1)

	tests := []struct {
		name    string
		search  models.DoctorSearch
		wantIDs []uint64
		wantErr error
	}{
		{name: "doctors with a free slot that day", search: models.DoctorSearch{Date: &monday}, wantIDs: []uint64{5, 6}},
		{name: "a free slot in the asked mode", search: models.DoctorSearch{Date: &monday, Mode: models.ConsultationModeVideo}, wantIDs: []uint64{6}},
		{name: "in person only", search: models.DoctorSearch{Date: &monday, Mode: models.ConsultationModeInPerson}, wantIDs: []uint64{5}},
		{name: "offset counts bookable doctors", search: models.DoctorSearch{Date: &monday, Offset: 1}, wantIDs: []uint64{6}},
		{name: "limit counts bookable doctors", search: models.DoctorSearch{Date: &monday, Limit: 1}, wantIDs: []uint64{5}},
		{name: "a past date", search: models.DoctorSearch{Date: &past}, wantErr: apperrors.ErrInvalidAvailabilityInput},
		{name: "a date past the booking horizon", search: models.DoctorSearch{Date: &beyond}, wantErr: apperrors.ErrInvalidAvailabilityInput},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			doctors := &fakeDoctorRepo{searchResult: []*models.Doctor{doctorWithID(5), doctorWithID(6), doctorWithID(7), doctorWithID(8)}}
			service := newTestDoctorSearch(t, doctors, rules, []*models.AppointmentDetails{fullyBooked}, now)

			listed, err := service.List(context.Background(), tt.search)
			if tt.wantErr != nil {
				if !errors.Is(err, tt.wantErr) {
					t.Fatalf("List error = %v, want %v", err, tt.wantErr)
				}
				return
			}
			if err != nil {
				t.Fatalf("List returned error: %v", err)
			}
			ids := make([]uint64, 0, len(listed))
			for _, doctor := range listed {
				ids = append(ids, doctor.ID)
			}
			if !slices.Equal(ids, tt.wantIDs) {
				t.Fatalf("List = %v, want %v", ids, tt.wantIDs)
			}
		})
	}

	t.Run("candidates are searched in batches until enough are bookable", func(t *testing.T) {
		candidates := make([]*models.Doctor, 0, constant.MaxListLimit+5)
		for id := uint64(100); len(candidates) < constant.MaxListLimit+5; id++ {
			candidates = append(candidates, doctorWithID(id))
		}
		lastID := candidates[len(candidates)-1].ID
		doctors := &fakeDoctorRepo{searchResult: candidates}
		service := newTestDoctorSearch(t, doctors, []*models.AvailabilityRule{morning(lastID, 10*60, models.ConsultationModeInPerson)}, nil, now)

		listed, err := service.List(context.Background(), models.DoctorSearch{Date: &monday})
		if err != nil || len(listed) != 1 || listed[0].ID != lastID {
			t.Fatalf("List = %d doctors, %v; want only doctor %d", len(listed), err, lastID)
		}
		if doctors.searchCalls != 2 {
			t.Fatalf("List searched %d batches, want 2", doctors.searchCalls)
		}
	})
}
