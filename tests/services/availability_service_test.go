package services_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"appointment_system/internal/apps/appointment/apperrors"
	"appointment_system/internal/apps/appointment/constant"
	"appointment_system/internal/apps/appointment/models"
	"appointment_system/internal/apps/appointment/services"
)

func minutes(value int) *int {
	return &value
}

func clinicDoctor(status models.VerificationStatus) *models.Doctor {
	return &models.Doctor{
		ID:                  5,
		UserID:              11,
		SlotDurationMin:     30,
		VerificationStatus:  status,
		IsAcceptingPatients: true,
		ConsultationModes:   []models.ConsultationMode{models.ConsultationModeInPerson, models.ConsultationModeVideo},
	}
}

func newTestAvailabilityService(t *testing.T, doctor *models.Doctor, repo *fakeAvailabilityRepo, now time.Time) *services.AvailabilityService {
	t.Helper()

	location, err := time.LoadLocation(constant.DefaultTimezone)
	if err != nil {
		t.Fatalf("LoadLocation returned error: %v", err)
	}
	service, err := services.NewAvailabilityService(&fakeDoctorRepo{byID: doctor, byUserID: doctor}, repo, &fakeAppointmentRepo{}, &fakeTxManager{}, location, func() time.Time { return now })
	if err != nil {
		t.Fatalf("NewAvailabilityService returned error: %v", err)
	}

	return service
}

func TestAvailabilityServiceSlotComputation(t *testing.T) {
	location, err := time.LoadLocation(constant.DefaultTimezone)
	if err != nil {
		t.Fatalf("LoadLocation returned error: %v", err)
	}
	date := time.Date(2026, 10, 26, 0, 0, 0, 0, time.UTC)
	dayBefore := time.Date(2026, 10, 25, 12, 0, 0, 0, location)
	morning := &models.AvailabilityRule{DayOfWeek: time.Monday, StartMinute: 9 * 60, EndMinute: 11 * 60, Mode: models.ConsultationModeInPerson}

	tests := []struct {
		name      string
		rules     []*models.AvailabilityRule
		overrides []*models.AvailabilityOverride
		duration  int
		now       time.Time
		want      []string
	}{
		{
			name:     "rule cut into slots",
			rules:    []*models.AvailabilityRule{morning},
			duration: 30,
			now:      dayBefore,
			want:     []string{"09:00 in_person", "09:30 in_person", "10:00 in_person", "10:30 in_person"},
		},
		{
			name:     "partial slot at the end is dropped",
			rules:    []*models.AvailabilityRule{{DayOfWeek: time.Monday, StartMinute: 9 * 60, EndMinute: 10*60 + 20, Mode: models.ConsultationModeInPerson}},
			duration: 30,
			now:      dayBefore,
			want:     []string{"09:00 in_person", "09:30 in_person"},
		},
		{
			name:      "whole day off removes everything",
			rules:     []*models.AvailabilityRule{morning},
			overrides: []*models.AvailabilityOverride{{IsAvailable: false}},
			duration:  30,
			now:       dayBefore,
			want:      []string{},
		},
		{
			name:      "partial leave cuts the block",
			rules:     []*models.AvailabilityRule{morning},
			overrides: []*models.AvailabilityOverride{{IsAvailable: false, StartMinute: minutes(9*60 + 30), EndMinute: minutes(10 * 60)}},
			duration:  30,
			now:       dayBefore,
			want:      []string{"09:00 in_person", "10:00 in_person", "10:30 in_person"},
		},
		{
			name:      "extra session adds slots in its own mode",
			rules:     []*models.AvailabilityRule{morning},
			overrides: []*models.AvailabilityOverride{{IsAvailable: true, StartMinute: minutes(18 * 60), EndMinute: minutes(19 * 60), Mode: models.ConsultationModeVideo}},
			duration:  30,
			now:       dayBefore,
			want:      []string{"09:00 in_person", "09:30 in_person", "10:00 in_person", "10:30 in_person", "18:00 video", "18:30 video"},
		},
		{
			name:      "extra session overlapping a rule in the same mode does not duplicate slots",
			rules:     []*models.AvailabilityRule{morning},
			overrides: []*models.AvailabilityOverride{{IsAvailable: true, StartMinute: minutes(10 * 60), EndMinute: minutes(12 * 60), Mode: models.ConsultationModeInPerson}},
			duration:  60,
			now:       dayBefore,
			want:      []string{"09:00 in_person", "10:00 in_person", "11:00 in_person"},
		},
		{
			name:     "slots already started are dropped",
			rules:    []*models.AvailabilityRule{morning},
			duration: 30,
			now:      time.Date(2026, 10, 26, 10, 0, 0, 0, location),
			want:     []string{"10:00 in_person", "10:30 in_person"},
		},
		{
			name:     "no rules means no slots",
			duration: 30,
			now:      dayBefore,
			want:     []string{},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			doctor := clinicDoctor(models.VerificationStatusVerified)
			doctor.SlotDurationMin = tt.duration
			service := newTestAvailabilityService(t, doctor, &fakeAvailabilityRepo{rules: tt.rules, overrides: tt.overrides}, tt.now)

			slots, err := service.ListSlots(context.Background(), 5, date)
			if err != nil {
				t.Fatalf("ListSlots error = %v", err)
			}

			got := make([]string, 0, len(slots))
			for _, slot := range slots {
				if slot.StartsAt.Location().String() != location.String() {
					t.Fatalf("slot location = %s, want %s", slot.StartsAt.Location(), location)
				}
				if slot.EndsAt.Sub(slot.StartsAt) != time.Duration(tt.duration)*time.Minute {
					t.Fatalf("slot length = %s, want %d minutes", slot.EndsAt.Sub(slot.StartsAt), tt.duration)
				}
				got = append(got, slot.StartsAt.Format("15:04")+" "+string(slot.Mode))
			}
			if len(got) != len(tt.want) {
				t.Fatalf("slots = %v, want %v", got, tt.want)
			}
			for index := range got {
				if got[index] != tt.want[index] {
					t.Fatalf("slots = %v, want %v", got, tt.want)
				}
			}
		})
	}
}

func TestAvailabilityServiceReplace(t *testing.T) {
	tests := []struct {
		name    string
		role    models.Role
		doctor  *models.Doctor
		rules   []models.AvailabilityRule
		wantErr error
	}{
		{
			name:   "valid week",
			role:   models.RoleDoctor,
			doctor: clinicDoctor(models.VerificationStatusPending),
			rules: []models.AvailabilityRule{
				{DayOfWeek: time.Monday, StartMinute: 14 * 60, EndMinute: 17 * 60, Mode: models.ConsultationModeVideo},
				{DayOfWeek: time.Monday, StartMinute: 9 * 60, EndMinute: 13 * 60, Mode: models.ConsultationModeInPerson},
			},
		},
		{name: "empty list clears the schedule", role: models.RoleDoctor, doctor: clinicDoctor(models.VerificationStatusVerified), rules: []models.AvailabilityRule{}},
		{
			name:   "overlapping blocks on one day",
			role:   models.RoleDoctor,
			doctor: clinicDoctor(models.VerificationStatusVerified),
			rules: []models.AvailabilityRule{
				{DayOfWeek: time.Monday, StartMinute: 9 * 60, EndMinute: 13 * 60, Mode: models.ConsultationModeInPerson},
				{DayOfWeek: time.Monday, StartMinute: 12 * 60, EndMinute: 14 * 60, Mode: models.ConsultationModeVideo},
			},
			wantErr: apperrors.ErrAvailabilityOverlap,
		},
		{
			name:   "same hours on different days do not overlap",
			role:   models.RoleDoctor,
			doctor: clinicDoctor(models.VerificationStatusVerified),
			rules: []models.AvailabilityRule{
				{DayOfWeek: time.Monday, StartMinute: 9 * 60, EndMinute: 13 * 60, Mode: models.ConsultationModeInPerson},
				{DayOfWeek: time.Tuesday, StartMinute: 9 * 60, EndMinute: 13 * 60, Mode: models.ConsultationModeInPerson},
			},
		},
		{name: "end before start", role: models.RoleDoctor, doctor: clinicDoctor(models.VerificationStatusVerified), rules: []models.AvailabilityRule{{DayOfWeek: time.Monday, StartMinute: 13 * 60, EndMinute: 9 * 60, Mode: models.ConsultationModeInPerson}}, wantErr: apperrors.ErrInvalidAvailabilityInput},
		{name: "block shorter than a slot", role: models.RoleDoctor, doctor: clinicDoctor(models.VerificationStatusVerified), rules: []models.AvailabilityRule{{DayOfWeek: time.Monday, StartMinute: 9 * 60, EndMinute: 9*60 + 20, Mode: models.ConsultationModeInPerson}}, wantErr: apperrors.ErrInvalidAvailabilityInput},
		{name: "invalid day", role: models.RoleDoctor, doctor: clinicDoctor(models.VerificationStatusVerified), rules: []models.AvailabilityRule{{DayOfWeek: 7, StartMinute: 9 * 60, EndMinute: 13 * 60, Mode: models.ConsultationModeInPerson}}, wantErr: apperrors.ErrInvalidAvailabilityInput},
		{name: "mode the doctor does not offer", role: models.RoleDoctor, doctor: clinicDoctor(models.VerificationStatusVerified), rules: []models.AvailabilityRule{{DayOfWeek: time.Monday, StartMinute: 9 * 60, EndMinute: 13 * 60, Mode: models.ConsultationModePhone}}, wantErr: apperrors.ErrInvalidAvailabilityInput},
		{name: "patient cannot set availability", role: models.RolePatient, doctor: clinicDoctor(models.VerificationStatusVerified), rules: []models.AvailabilityRule{}, wantErr: apperrors.ErrDoctorRoleRequired},
		{name: "no profile yet", role: models.RoleDoctor, rules: []models.AvailabilityRule{}, wantErr: apperrors.ErrDoctorNotFound},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			repo := &fakeAvailabilityRepo{}
			service := newTestAvailabilityService(t, tt.doctor, repo, time.Now())

			replaced, err := service.Replace(context.Background(), 11, tt.role, tt.rules)
			if !errors.Is(err, tt.wantErr) {
				t.Fatalf("Replace error = %v, want %v", err, tt.wantErr)
			}
			if tt.wantErr != nil {
				if repo.replaceCalled {
					t.Fatalf("rules were written despite the error")
				}
				return
			}
			if len(replaced) != len(tt.rules) {
				t.Fatalf("replaced %d rules, want %d", len(replaced), len(tt.rules))
			}
			for index := 1; index < len(repo.replaceInput); index++ {
				previous, current := repo.replaceInput[index-1], repo.replaceInput[index]
				if previous.DayOfWeek == current.DayOfWeek && previous.StartMinute > current.StartMinute {
					t.Fatalf("rules not stored in day and time order: %+v", repo.replaceInput)
				}
			}
		})
	}
}

func TestAvailabilityServiceAddOverride(t *testing.T) {
	now := time.Date(2026, 10, 20, 12, 0, 0, 0, time.UTC)
	tomorrow := time.Date(2026, 10, 21, 0, 0, 0, 0, time.UTC)

	tests := []struct {
		name     string
		override models.AvailabilityOverride
		wantErr  error
	}{
		{name: "whole day off", override: models.AvailabilityOverride{Date: tomorrow, Reason: "  Conference  "}},
		{name: "partial leave", override: models.AvailabilityOverride{Date: tomorrow, StartMinute: minutes(14 * 60), EndMinute: minutes(15 * 60)}},
		{name: "extra session", override: models.AvailabilityOverride{Date: tomorrow, IsAvailable: true, StartMinute: minutes(18 * 60), EndMinute: minutes(19 * 60), Mode: models.ConsultationModeVideo}},
		{name: "date in the past", override: models.AvailabilityOverride{Date: tomorrow.AddDate(0, 0, -5)}, wantErr: apperrors.ErrInvalidAvailabilityInput},
		{name: "more than a year ahead", override: models.AvailabilityOverride{Date: tomorrow.AddDate(2, 0, 0)}, wantErr: apperrors.ErrInvalidAvailabilityInput},
		{name: "extra session without times", override: models.AvailabilityOverride{Date: tomorrow, IsAvailable: true, Mode: models.ConsultationModeVideo}, wantErr: apperrors.ErrInvalidAvailabilityInput},
		{name: "only a start time", override: models.AvailabilityOverride{Date: tomorrow, StartMinute: minutes(14 * 60)}, wantErr: apperrors.ErrInvalidAvailabilityInput},
		{name: "extra session in a mode not offered", override: models.AvailabilityOverride{Date: tomorrow, IsAvailable: true, StartMinute: minutes(18 * 60), EndMinute: minutes(19 * 60), Mode: models.ConsultationModePhone}, wantErr: apperrors.ErrInvalidAvailabilityInput},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			repo := &fakeAvailabilityRepo{}
			service := newTestAvailabilityService(t, clinicDoctor(models.VerificationStatusVerified), repo, now)

			override := tt.override
			created, err := service.AddOverride(context.Background(), 11, models.RoleDoctor, &override)
			if !errors.Is(err, tt.wantErr) {
				t.Fatalf("AddOverride error = %v, want %v", err, tt.wantErr)
			}
			if tt.wantErr != nil {
				return
			}
			if created.DoctorID != 5 || !created.Date.Equal(tomorrow) {
				t.Fatalf("created override = %+v", created)
			}
			if created.Reason != "" && created.Reason != "Conference" {
				t.Fatalf("reason not trimmed: %q", created.Reason)
			}
		})
	}
}

func TestAvailabilityServiceListSlots(t *testing.T) {
	now := time.Date(2026, 10, 20, 12, 0, 0, 0, time.UTC)
	monday := time.Date(2026, 10, 26, 0, 0, 0, 0, time.UTC)
	rules := []*models.AvailabilityRule{{DayOfWeek: time.Monday, StartMinute: 9 * 60, EndMinute: 10 * 60, Mode: models.ConsultationModeInPerson}}
	notAccepting := clinicDoctor(models.VerificationStatusVerified)
	notAccepting.IsAcceptingPatients = false

	tests := []struct {
		name      string
		doctor    *models.Doctor
		date      time.Time
		wantSlots int
		wantErr   error
	}{
		{name: "verified doctor on a working day", doctor: clinicDoctor(models.VerificationStatusVerified), date: monday, wantSlots: 2},
		{name: "verified doctor on a day off", doctor: clinicDoctor(models.VerificationStatusVerified), date: monday.AddDate(0, 0, 1), wantSlots: 0},
		{name: "not accepting patients", doctor: notAccepting, date: monday, wantSlots: 0},
		{name: "pending doctor is hidden", doctor: clinicDoctor(models.VerificationStatusPending), date: monday, wantErr: apperrors.ErrDoctorNotFound},
		{name: "missing doctor", date: monday, wantErr: apperrors.ErrDoctorNotFound},
		{name: "date in the past", doctor: clinicDoctor(models.VerificationStatusVerified), date: monday.AddDate(0, 0, -10), wantErr: apperrors.ErrInvalidAvailabilityInput},
		{name: "date too far ahead", doctor: clinicDoctor(models.VerificationStatusVerified), date: monday.AddDate(0, 0, 120), wantErr: apperrors.ErrInvalidAvailabilityInput},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			repo := &fakeAvailabilityRepo{rules: rules}
			service := newTestAvailabilityService(t, tt.doctor, repo, now)

			slots, err := service.ListSlots(context.Background(), 5, tt.date)
			if !errors.Is(err, tt.wantErr) {
				t.Fatalf("ListSlots error = %v, want %v", err, tt.wantErr)
			}
			if tt.wantErr == nil && len(slots) != tt.wantSlots {
				t.Fatalf("slots = %d, want %d", len(slots), tt.wantSlots)
			}
		})
	}
}
