package services_test

import (
	"errors"
	"testing"
	"time"

	"appointment_system/internal/apps/appointment/apperrors"
	"appointment_system/internal/apps/appointment/models"
	"appointment_system/internal/apps/appointment/services"
)

func TestNewServicesRequireLogger(t *testing.T) {
	availability := newTestAvailability(t)
	payments, err := services.NewPaymentService(&fakePaymentRepo{}, &fakeAppointmentRepo{}, &fakeOutboxRepo{}, &fakeUserPatientRepo{}, &fakeDoctorRepo{}, &fakeGateway{}, []models.PaymentMethod{models.PaymentMethodRazorpay}, &fakeFileStorage{}, &fakeTxManager{}, time.Now, discardLogger)
	if err != nil {
		t.Fatalf("NewPaymentService returned error: %v", err)
	}
	appointments, err := services.NewAppointmentService(&fakeAppointmentRepo{}, &fakeOutboxRepo{}, &fakeUserPatientRepo{}, &fakeDoctorRepo{}, availability, payments, &fakeCallRoomRepo{}, &fakeCallEventRepo{}, &fakeTxManager{}, discardLogger)
	if err != nil {
		t.Fatalf("NewAppointmentService returned error: %v", err)
	}

	tests := []struct {
		name string
		new  func() error
	}{
		{name: "auth", new: func() error {
			_, err := services.NewAuthService(&fakeUserRepo{}, &fakeSessionRepo{}, &fakeTokenGen{}, &fakeUserAgentParser{}, nil)
			return err
		}},
		{name: "patient", new: func() error {
			_, err := services.NewPatientService(&fakePatientRepo{}, &fakeUserPatientRepo{}, &fakeUserPatientHistoryRepo{}, &fakeTxManager{}, nil)
			return err
		}},
		{name: "admin user", new: func() error {
			_, err := services.NewAdminUserService(&fakeAdminUserRepo{}, nil)
			return err
		}},
		{name: "availability", new: func() error {
			_, err := services.NewAvailabilityService(&fakeDoctorRepo{}, &fakeAvailabilityRepo{}, &fakeAppointmentRepo{}, &fakeTxManager{}, time.UTC, time.Now, nil)
			return err
		}},
		{name: "doctor", new: func() error {
			_, err := services.NewDoctorService(&fakeDoctorRepo{}, &fakeSpecialityRepo{}, &fakeDoctorVerificationRepo{}, availability, &fakeTxManager{}, nil)
			return err
		}},
		{name: "payment", new: func() error {
			_, err := services.NewPaymentService(&fakePaymentRepo{}, &fakeAppointmentRepo{}, &fakeOutboxRepo{}, &fakeUserPatientRepo{}, &fakeDoctorRepo{}, &fakeGateway{}, []models.PaymentMethod{models.PaymentMethodRazorpay}, &fakeFileStorage{}, &fakeTxManager{}, time.Now, nil)
			return err
		}},
		{name: "appointment", new: func() error {
			_, err := services.NewAppointmentService(&fakeAppointmentRepo{}, &fakeOutboxRepo{}, &fakeUserPatientRepo{}, &fakeDoctorRepo{}, availability, payments, &fakeCallRoomRepo{}, &fakeCallEventRepo{}, &fakeTxManager{}, nil)
			return err
		}},
		{name: "doctor verification", new: func() error {
			_, err := services.NewDoctorVerificationService(&fakeDoctorRepo{}, &fakeDoctorVerificationRepo{}, appointments, &fakeTxManager{}, nil)
			return err
		}},
		{name: "medical record", new: func() error {
			_, err := services.NewMedicalRecordService(&fakeMedicalRecordRepo{}, &fakeAppointmentRepo{}, &fakeUserPatientRepo{}, &fakeDoctorRepo{}, &fakeFileStorage{}, &fakeOutboxRepo{}, &fakeTxManager{}, time.Now, nil)
			return err
		}},
		{name: "outbox", new: func() error {
			_, err := services.NewOutboxService(&fakeOutboxRepo{}, &fakeTxManager{}, nil)
			return err
		}},
		{name: "reminder", new: func() error {
			_, err := services.NewReminderService(&fakeAppointmentRepo{}, &fakeOutboxRepo{}, &fakeTxManager{}, time.UTC, time.Now, nil)
			return err
		}},
		{name: "session", new: func() error {
			_, err := services.NewSessionService(&fakeSessionRepo{}, &fakeUserRepo{}, time.Now, nil)
			return err
		}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if err := tt.new(); !errors.Is(err, apperrors.ErrNilLogger) {
				t.Fatalf("constructor error = %v, want %v", err, apperrors.ErrNilLogger)
			}
		})
	}
}
