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

func pendingProposal(original *models.AppointmentDetails, expiresAt time.Time) *models.RescheduleProposal {
	return &models.RescheduleProposal{
		ID:                       80,
		AppointmentID:            original.ID,
		PlaceholderAppointmentID: 60,
		ProposedByUserID:         11,
		Reason:                   models.ProposalReasonEmergency,
		Status:                   models.ProposalStatusPending,
		ExpiresAt:                expiresAt,
		NewStartsAt:              original.StartsAt.Add(time.Hour),
		NewEndsAt:                original.StartsAt.Add(90 * time.Minute),
		NewMode:                  original.Mode,
	}
}

func TestAppointmentServiceProposeReschedule(t *testing.T) {
	location := clinicLocation(t)
	nineAM := time.Date(2026, 10, 26, 9, 0, 0, 0, location)
	dayBefore := nineAM.Add(-21 * time.Hour)
	ownDoctor := clinicDoctor(models.VerificationStatusVerified)
	otherDoctor := clinicDoctor(models.VerificationStatusVerified)
	otherDoctor.ID = 6
	unpaid := func() *models.AppointmentDetails {
		return unpaidAt(nineAM, dayBefore.Add(5*time.Minute))
	}
	proposed := func() *models.AppointmentDetails {
		return scheduledAt(nineAM, models.AppointmentStatusRescheduleProposed)
	}

	tests := []struct {
		name        string
		role        models.Role
		doctor      *models.Doctor
		appointment func() *models.AppointmentDetails
		startsAt    time.Time
		reason      models.ProposalReason
		wantErr     error
	}{
		{name: "doctor offers a free slot", role: models.RoleDoctor, doctor: ownDoctor},
		{name: "patient account", role: models.RolePatient, wantErr: apperrors.ErrDoctorRoleRequired},
		{name: "doctor of another appointment", role: models.RoleDoctor, doctor: otherDoctor, wantErr: apperrors.ErrAppointmentNotFound},
		{name: "unknown reason", role: models.RoleDoctor, doctor: ownDoctor, reason: "bored", wantErr: apperrors.ErrInvalidAppointmentInput},
		{name: "booking still awaiting payment", role: models.RoleDoctor, doctor: ownDoctor, appointment: unpaid, wantErr: apperrors.ErrAppointmentNotModifiable},
		{name: "a proposal is already waiting", role: models.RoleDoctor, doctor: ownDoctor, appointment: proposed, wantErr: apperrors.ErrAppointmentNotModifiable},
		{name: "its own current time is not free", role: models.RoleDoctor, doctor: ownDoctor, startsAt: nineAM, wantErr: apperrors.ErrSlotNotAvailable},
		{name: "time between slots", role: models.RoleDoctor, doctor: ownDoctor, startsAt: nineAM.Add(70 * time.Minute), wantErr: apperrors.ErrSlotNotAvailable},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if tt.appointment == nil {
				tt.appointment = func() *models.AppointmentDetails { return scheduledAt(nineAM, models.AppointmentStatusScheduled) }
			}
			if tt.startsAt.IsZero() {
				tt.startsAt = nineAM.Add(time.Hour)
			}
			if tt.reason == "" {
				tt.reason = models.ProposalReasonEmergency
			}
			original := tt.appointment()
			appointments := &fakeAppointmentRepo{byID: original, booked: []*models.AppointmentDetails{original}}
			outbox := &fakeOutboxRepo{}
			service := newTestAppointmentService(t, &fakeDoctorRepo{byID: ownDoctor, byUserID: tt.doctor}, appointments, outbox, &fakeUserPatientRepo{activeErr: apperrors.ErrUserPatientNotFound}, dayBefore)

			proposal, err := service.ProposeReschedule(context.Background(), 11, tt.role, 50, tt.startsAt, "", tt.reason, " clinic flooded ")
			if tt.wantErr != nil {
				if !errors.Is(err, tt.wantErr) {
					t.Fatalf("ProposeReschedule error = %v, want %v", err, tt.wantErr)
				}
				if len(appointments.created) != 0 || len(outbox.events) != 0 {
					t.Fatalf("ProposeReschedule changed appointments after an error")
				}
				return
			}
			if err != nil {
				t.Fatalf("ProposeReschedule returned error: %v", err)
			}

			if len(appointments.statusUpdates) != 1 || appointments.statusUpdates[0] != "50:scheduled->reschedule_proposed" {
				t.Fatalf("ProposeReschedule status updates = %v, want the original held as reschedule_proposed", appointments.statusUpdates)
			}
			placeholder := appointments.created[0]
			if placeholder.Status != models.AppointmentStatusRescheduleProposed || !placeholder.StartsAt.Equal(tt.startsAt) ||
				placeholder.BookedByUserID != 7 || placeholder.RescheduledFromID == nil || *placeholder.RescheduledFromID != 50 ||
				placeholder.PaymentStatus != models.AppointmentPaymentPaid || placeholder.ConsultationFee != 40000 {
				t.Fatalf("ProposeReschedule placeholder = %+v, want the offered time held for the original booker with the original payment", placeholder)
			}
			created := appointments.createdProposal
			if created.PlaceholderAppointmentID != placeholder.ID || created.Note != "clinic flooded" || !created.ExpiresAt.Equal(nineAM) {
				t.Fatalf("ProposeReschedule proposal = %+v, want it to point at the placeholder and expire at the original start", created)
			}
			if len(outbox.events) != 1 || outbox.events[0].EventType != models.OutboxEventRescheduleProposed || outbox.events[0].AggregateID != 50 {
				t.Fatalf("ProposeReschedule outbox = %+v, want one reschedule_proposed event for 50", outbox.events)
			}
			if proposal.ID != 80 {
				t.Fatalf("ProposeReschedule returned proposal %d, want 80", proposal.ID)
			}
		})
	}
}

func TestAppointmentServiceProposalExpiresAfterTTL(t *testing.T) {
	location := clinicLocation(t)
	nineAM := time.Date(2026, 10, 30, 9, 0, 0, 0, location)
	now := time.Date(2026, 10, 25, 12, 0, 0, 0, location)
	doctor := clinicDoctor(models.VerificationStatusVerified)
	doctor.SlotDurationMin = 30
	original := scheduledAt(nineAM, models.AppointmentStatusScheduled)
	appointments := &fakeAppointmentRepo{byID: original}
	availability := &fakeAvailabilityRepo{rules: []*models.AvailabilityRule{{DayOfWeek: time.Friday, StartMinute: 9 * 60, EndMinute: 11 * 60, Mode: models.ConsultationModeInPerson}}}
	availabilityService, err := services.NewAvailabilityService(&fakeDoctorRepo{byID: doctor, byUserID: doctor}, availability, appointments, &fakeTxManager{}, location, func() time.Time { return now }, discardLogger)
	if err != nil {
		t.Fatalf("NewAvailabilityService returned error: %v", err)
	}
	payments, err := services.NewPaymentService(&fakePaymentRepo{}, appointments, &fakeOutboxRepo{}, &fakeUserPatientRepo{}, &fakeDoctorRepo{}, &fakeGateway{}, &fakeTxManager{}, func() time.Time { return now }, discardLogger)
	if err != nil {
		t.Fatalf("NewPaymentService returned error: %v", err)
	}
	service, err := services.NewAppointmentService(appointments, &fakeOutboxRepo{}, &fakeUserPatientRepo{activeErr: apperrors.ErrUserPatientNotFound}, &fakeDoctorRepo{byID: doctor, byUserID: doctor}, availabilityService, payments, &fakeTxManager{}, discardLogger)
	if err != nil {
		t.Fatalf("NewAppointmentService returned error: %v", err)
	}

	if _, err := service.ProposeReschedule(context.Background(), 11, models.RoleDoctor, 50, nineAM.Add(time.Hour), "", models.ProposalReasonPersonal, ""); err != nil {
		t.Fatalf("ProposeReschedule returned error: %v", err)
	}
	if want := now.Add(constant.RescheduleProposalTTL); !appointments.createdProposal.ExpiresAt.Equal(want) {
		t.Fatalf("ProposeReschedule expires at %v, want %v", appointments.createdProposal.ExpiresAt, want)
	}
}

func TestAppointmentServiceAnswerProposal(t *testing.T) {
	location := clinicLocation(t)
	nineAM := time.Date(2026, 10, 26, 9, 0, 0, 0, location)
	dayBefore := nineAM.Add(-21 * time.Hour)
	ownDoctor := clinicDoctor(models.VerificationStatusVerified)

	tests := []struct {
		name        string
		accept      bool
		role        models.Role
		link        *models.UserPatient
		status      models.AppointmentStatus
		expiresAt   time.Time
		noProposal  bool
		closeErr    error
		wantErr     error
		wantClosed  models.ProposalStatus
		wantCancel  uint64
		wantReason  models.CancelReason
		wantUpdates []string
		wantEvent   string
	}{
		{
			name: "accept moves the appointment to the offered time", accept: true, role: models.RolePatient, link: patientLink(models.AccessLevelAppointmentsOnly),
			wantClosed: models.ProposalStatusAccepted, wantCancel: 50, wantReason: models.CancelReasonRescheduled,
			wantUpdates: []string{"60:reschedule_proposed->scheduled"}, wantEvent: models.OutboxEventAppointmentRescheduled,
		},
		{
			name: "decline keeps the original time", role: models.RolePatient, link: patientLink(models.AccessLevelFull),
			wantClosed: models.ProposalStatusDeclined, wantCancel: 60, wantReason: models.CancelReasonProposalDeclined,
			wantUpdates: []string{"50:reschedule_proposed->scheduled"}, wantEvent: models.OutboxEventRescheduleDeclined,
		},
		{name: "read_only access cannot answer", accept: true, role: models.RolePatient, link: patientLink(models.AccessLevelReadOnly), wantErr: apperrors.ErrRescheduleForbidden},
		{name: "the doctor cannot answer their own proposal", role: models.RoleDoctor, wantErr: apperrors.ErrRescheduleForbidden},
		{name: "no proposal is waiting", accept: true, role: models.RolePatient, link: patientLink(models.AccessLevelFull), status: models.AppointmentStatusScheduled, noProposal: true, wantErr: apperrors.ErrProposalNotFound},
		{name: "the proposal has expired", accept: true, role: models.RolePatient, link: patientLink(models.AccessLevelFull), expiresAt: dayBefore.Add(-time.Minute), wantErr: apperrors.ErrAppointmentNotModifiable},
		{name: "answered meanwhile", role: models.RolePatient, link: patientLink(models.AccessLevelFull), closeErr: apperrors.ErrAppointmentNotModifiable, wantErr: apperrors.ErrAppointmentNotModifiable},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if tt.status == "" {
				tt.status = models.AppointmentStatusRescheduleProposed
			}
			if tt.expiresAt.IsZero() {
				tt.expiresAt = nineAM
			}
			original := scheduledAt(nineAM, tt.status)
			appointments := &fakeAppointmentRepo{byID: original, closeErr: tt.closeErr}
			if !tt.noProposal {
				appointments.proposal = pendingProposal(original, tt.expiresAt)
			}
			userPatients := &fakeUserPatientRepo{activeResult: tt.link}
			if tt.link == nil {
				userPatients.activeErr = apperrors.ErrUserPatientNotFound
			}
			payments := &fakePaymentRepo{}
			outbox := &fakeOutboxRepo{}
			service, _ := newTestAppointmentAndPaymentServices(t, &fakeDoctorRepo{byID: ownDoctor, byUserID: ownDoctor}, appointments, outbox, userPatients, payments, &fakeGateway{}, dayBefore)

			answer := service.DeclineReschedule
			if tt.accept {
				answer = service.AcceptReschedule
			}
			_, err := answer(context.Background(), 7, tt.role, 50)
			if tt.wantErr != nil {
				if !errors.Is(err, tt.wantErr) {
					t.Fatalf("answer error = %v, want %v", err, tt.wantErr)
				}
				if len(outbox.events) != 0 {
					t.Fatalf("answer wrote outbox events after an error")
				}
				return
			}
			if err != nil {
				t.Fatalf("answer returned error: %v", err)
			}
			if appointments.closedTo != tt.wantClosed || appointments.closedBy != 7 {
				t.Fatalf("answer closed the proposal as %q by %d, want %q by 7", appointments.closedTo, appointments.closedBy, tt.wantClosed)
			}
			if len(appointments.cancelledIDs) != 1 || appointments.cancelledIDs[0] != tt.wantCancel || appointments.cancelReason != tt.wantReason {
				t.Fatalf("answer cancelled %v for %q, want %d for %q", appointments.cancelledIDs, appointments.cancelReason, tt.wantCancel, tt.wantReason)
			}
			if len(appointments.statusUpdates) != len(tt.wantUpdates) || appointments.statusUpdates[0] != tt.wantUpdates[0] {
				t.Fatalf("answer status updates = %v, want %v", appointments.statusUpdates, tt.wantUpdates)
			}
			if len(outbox.events) != 1 || outbox.events[0].EventType != tt.wantEvent {
				t.Fatalf("answer outbox = %+v, want one %s event", outbox.events, tt.wantEvent)
			}
			if tt.accept && (payments.movedFrom != 50 || payments.movedTo != 60) {
				t.Fatalf("accept moved payments %d -> %d, want 50 -> 60", payments.movedFrom, payments.movedTo)
			}
		})
	}
}

func TestAppointmentServiceExpireProposals(t *testing.T) {
	location := clinicLocation(t)
	nineAM := time.Date(2026, 10, 26, 9, 0, 0, 0, location)
	now := nineAM.Add(-time.Minute)
	original := scheduledAt(nineAM, models.AppointmentStatusRescheduleProposed)

	t.Run("an unanswered proposal is withdrawn by the system", func(t *testing.T) {
		appointments := &fakeAppointmentRepo{byID: original, expiredProposals: []*models.RescheduleProposal{pendingProposal(original, now.Add(-time.Hour))}}
		outbox := &fakeOutboxRepo{}
		service := newTestAppointmentService(t, &fakeDoctorRepo{}, appointments, outbox, &fakeUserPatientRepo{}, now)

		expired, err := service.ExpireProposals(context.Background(), 10)
		if err != nil || expired != 1 {
			t.Fatalf("ExpireProposals = %d, %v; want 1", expired, err)
		}
		if appointments.closedTo != models.ProposalStatusExpired || appointments.closedBy != 0 || appointments.cancelReason != models.CancelReasonProposalExpired {
			t.Fatalf("ExpireProposals closed as %q by %d with cancel reason %q, want expired by the system", appointments.closedTo, appointments.closedBy, appointments.cancelReason)
		}
		if len(outbox.events) != 1 || outbox.events[0].EventType != models.OutboxEventRescheduleExpired {
			t.Fatalf("ExpireProposals outbox = %+v, want one reschedule_expired event", outbox.events)
		}
	})

	t.Run("a proposal answered meanwhile is skipped", func(t *testing.T) {
		appointments := &fakeAppointmentRepo{byID: original, closeErr: apperrors.ErrAppointmentNotModifiable, expiredProposals: []*models.RescheduleProposal{pendingProposal(original, now.Add(-time.Hour))}}
		service := newTestAppointmentService(t, &fakeDoctorRepo{}, appointments, &fakeOutboxRepo{}, &fakeUserPatientRepo{}, now)

		expired, err := service.ExpireProposals(context.Background(), 10)
		if err != nil || expired != 0 {
			t.Fatalf("ExpireProposals = %d, %v; want 0 without error", expired, err)
		}
	})
}

func TestAppointmentServiceHeldByProposal(t *testing.T) {
	location := clinicLocation(t)
	nineAM := time.Date(2026, 10, 26, 9, 0, 0, 0, location)
	dayBefore := nineAM.Add(-21 * time.Hour)
	held := func() *fakeAppointmentRepo {
		return &fakeAppointmentRepo{byID: scheduledAt(nineAM, models.AppointmentStatusRescheduleProposed)}
	}

	t.Run("the patient side cannot cancel or reschedule until it answers", func(t *testing.T) {
		service := newTestAppointmentService(t, &fakeDoctorRepo{byID: clinicDoctor(models.VerificationStatusVerified)}, held(), &fakeOutboxRepo{}, &fakeUserPatientRepo{activeResult: patientLink(models.AccessLevelFull)}, dayBefore)

		if _, err := service.Cancel(context.Background(), 7, models.RolePatient, 50, ""); !errors.Is(err, apperrors.ErrAppointmentNotModifiable) || apperrors.Reason(err) != "the doctor proposed a new time; accept or decline it first" {
			t.Fatalf("Cancel error = %v, want the answer-first reason", err)
		}
		if _, err := service.Reschedule(context.Background(), 7, models.RolePatient, 50, nineAM.Add(time.Hour), "", ""); !errors.Is(err, apperrors.ErrAppointmentNotModifiable) {
			t.Fatalf("Reschedule error = %v, want %v", err, apperrors.ErrAppointmentNotModifiable)
		}
	})

	t.Run("the placeholder holds its slot", func(t *testing.T) {
		doctor := clinicDoctor(models.VerificationStatusVerified)
		placeholder := scheduledAt(nineAM.Add(time.Hour), models.AppointmentStatusRescheduleProposed)
		appointments := &fakeAppointmentRepo{booked: []*models.AppointmentDetails{placeholder}}
		availability, err := services.NewAvailabilityService(
			&fakeDoctorRepo{byID: doctor},
			&fakeAvailabilityRepo{rules: []*models.AvailabilityRule{{DayOfWeek: time.Monday, StartMinute: 9 * 60, EndMinute: 11 * 60, Mode: models.ConsultationModeInPerson}}},
			appointments,
			&fakeTxManager{},
			location,
			func() time.Time { return dayBefore },
			discardLogger,
		)
		if err != nil {
			t.Fatalf("NewAvailabilityService returned error: %v", err)
		}

		slots, err := availability.ListSlots(context.Background(), 5, time.Date(2026, 10, 26, 0, 0, 0, 0, time.UTC))
		if err != nil {
			t.Fatalf("ListSlots returned error: %v", err)
		}
		for _, slot := range slots {
			if slot.StartsAt.Equal(placeholder.StartsAt) {
				t.Fatalf("ListSlots offers %v, which a reschedule proposal holds", slot.StartsAt)
			}
		}
	})
}
