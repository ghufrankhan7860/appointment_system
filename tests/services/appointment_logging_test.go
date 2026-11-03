package services_test

import (
	"bytes"
	"context"
	"encoding/json"
	"strings"
	"testing"
	"time"

	"appointment_system/internal/apps/appointment/models"
	"appointment_system/internal/apps/appointment/services"
	"appointment_system/internal/platform/logger"
)

func TestAppointmentServiceBookLogsBooking(t *testing.T) {
	location := clinicLocation(t)
	now := time.Date(2026, 10, 25, 12, 0, 0, 0, location)
	nineAM := time.Date(2026, 10, 26, 9, 0, 0, 0, location)

	var output bytes.Buffer
	jsonLogger, err := logger.New(logger.Config{Level: "debug", Format: logger.FormatJSON}, &output)
	if err != nil {
		t.Fatalf("logger.New returned error: %v", err)
	}

	doctor := clinicDoctor(models.VerificationStatusVerified)
	doctor.ConsultationFee = 50000
	doctors := &fakeDoctorRepo{byID: doctor, byUserID: doctor}
	appointments := &fakeAppointmentRepo{}
	outbox := &fakeOutboxRepo{}
	userPatients := &fakeUserPatientRepo{activeResult: patientLink(models.AccessLevelFull)}
	clock := func() time.Time { return now }
	mondayMorning := &models.AvailabilityRule{DayOfWeek: time.Monday, StartMinute: 9 * 60, EndMinute: 11 * 60, Mode: models.ConsultationModeInPerson}

	availability, err := services.NewAvailabilityService(doctors, &fakeAvailabilityRepo{rules: []*models.AvailabilityRule{mondayMorning}}, appointments, &fakeTxManager{}, location, clock, jsonLogger)
	if err != nil {
		t.Fatalf("NewAvailabilityService returned error: %v", err)
	}
	payments, err := services.NewPaymentService(&fakePaymentRepo{}, appointments, outbox, userPatients, doctors, &fakeGateway{}, &fakeTxManager{}, clock, jsonLogger)
	if err != nil {
		t.Fatalf("NewPaymentService returned error: %v", err)
	}
	service, err := services.NewAppointmentService(appointments, outbox, userPatients, doctors, availability, payments, &fakeTxManager{}, jsonLogger)
	if err != nil {
		t.Fatalf("NewAppointmentService returned error: %v", err)
	}

	booked, err := service.Book(context.Background(), 7, &models.Appointment{
		PatientID:    3,
		DoctorID:     doctor.ID,
		StartsAt:     nineAM,
		Mode:         models.ConsultationModeInPerson,
		PatientNotes: "fever since Monday",
	})
	if err != nil {
		t.Fatalf("Book returned error: %v", err)
	}

	var bookedLine map[string]any
	for _, line := range strings.Split(strings.TrimSpace(output.String()), "\n") {
		var record map[string]any
		if err := json.Unmarshal([]byte(line), &record); err != nil {
			t.Fatalf("log line %q is not JSON: %v", line, err)
		}
		if record["msg"] == "appointment booked" {
			bookedLine = record
		}
	}
	if bookedLine == nil {
		t.Fatalf("Book logged %q, want an appointment booked line", output.String())
	}
	if bookedLine["level"] != "INFO" || bookedLine["appointment_id"] != float64(booked.ID) ||
		bookedLine["patient_id"] != float64(3) || bookedLine["payment_status"] != string(models.AppointmentPaymentUnpaid) {
		t.Fatalf("appointment booked line = %v, want INFO with the appointment, patient and unpaid status", bookedLine)
	}
	if strings.Contains(output.String(), "fever") {
		t.Fatalf("Book logged the patient notes: %s", output.String())
	}
}
