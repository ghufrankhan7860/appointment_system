package services_test

import (
	"bytes"
	"context"
	"errors"
	"io"
	"slices"
	"strings"
	"testing"
	"time"

	"appointment_system/internal/apps/appointment/apperrors"
	"appointment_system/internal/apps/appointment/constant"
	"appointment_system/internal/apps/appointment/models"
)

var (
	upiAndClinic = []models.PaymentMethod{models.PaymentMethodUPI, models.PaymentMethodClinic}
	pngImage     = []byte("\x89PNG\r\n\x1a\n\x00\x00\x00\rIHDR")
)

func upiDoctor() *models.Doctor {
	doctor := clinicDoctor(models.VerificationStatusVerified)
	doctor.FullName = "Asha Rao"
	doctor.UPIID = "asha@okbank"
	return doctor
}

func upiPayment(status models.PaymentStatus) *models.Payment {
	return &models.Payment{ID: 70, AppointmentID: 50, PayerUserID: 7, Amount: 40000, Currency: constant.PaymentCurrency, Provider: models.ProviderUPIDirect, Status: status}
}

func pendingAt(startsAt time.Time) *models.AppointmentDetails {
	appointment := unpaidAt(startsAt, startsAt)
	appointment.PaymentStatus = models.AppointmentPaymentPendingVerification
	return appointment
}

func TestPaymentServiceOptions(t *testing.T) {
	location := clinicLocation(t)
	now := time.Date(2026, 10, 25, 12, 0, 0, 0, location)
	nineAM := time.Date(2026, 10, 26, 9, 0, 0, 0, location)

	tests := []struct {
		name        string
		methods     []models.PaymentMethod
		upiID       string
		wantMethods []models.PaymentMethod
		wantUPI     bool
	}{
		{name: "razorpay alone", methods: []models.PaymentMethod{models.PaymentMethodRazorpay}, upiID: "asha@okbank", wantMethods: []models.PaymentMethod{models.PaymentMethodRazorpay}},
		{name: "every method with a doctor's upi id", methods: []models.PaymentMethod{models.PaymentMethodRazorpay, models.PaymentMethodUPI, models.PaymentMethodClinic}, upiID: "asha@okbank", wantMethods: []models.PaymentMethod{models.PaymentMethodRazorpay, models.PaymentMethodUPI, models.PaymentMethodClinic}, wantUPI: true},
		{name: "upi alone with a doctor's upi id", methods: []models.PaymentMethod{models.PaymentMethodUPI}, upiID: "asha@okbank", wantMethods: []models.PaymentMethod{models.PaymentMethodUPI}, wantUPI: true},
		{name: "upi alone without a upi id falls back to the clinic", methods: []models.PaymentMethod{models.PaymentMethodUPI}, wantMethods: []models.PaymentMethod{models.PaymentMethodClinic}},
		{name: "none mode is the clinic", methods: []models.PaymentMethod{}, upiID: "asha@okbank", wantMethods: []models.PaymentMethod{models.PaymentMethodClinic}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			doctor := upiDoctor()
			doctor.UPIID = tt.upiID
			_, service := newTestServicesWithMethods(t, tt.methods, &fakeFileStorage{}, &fakeDoctorRepo{byID: doctor}, &fakeAppointmentRepo{byID: unpaidAt(nineAM, now.Add(5*time.Minute))}, &fakeOutboxRepo{}, &fakeUserPatientRepo{activeResult: patientLink(models.AccessLevelFull)}, &fakePaymentRepo{}, &fakeGateway{}, now)

			options, err := service.Options(context.Background(), 7, 50)
			if err != nil {
				t.Fatalf("Options returned error: %v", err)
			}
			if !slices.Equal(options.Methods, tt.wantMethods) {
				t.Fatalf("Options methods = %v, want %v", options.Methods, tt.wantMethods)
			}
			if tt.wantUPI != (options.UPI != nil) || tt.wantUPI && (options.UPI.UPIID != "asha@okbank" || options.UPI.PayeeName != "Asha Rao") {
				t.Fatalf("Options upi = %+v, want present %v with the doctor's details", options.UPI, tt.wantUPI)
			}
		})
	}
}

func TestAppointmentServiceBookPaymentByMethod(t *testing.T) {
	location := clinicLocation(t)
	now := time.Date(2026, 10, 25, 12, 0, 0, 0, location)
	nineAM := time.Date(2026, 10, 26, 9, 0, 0, 0, location)

	tests := []struct {
		name       string
		methods    []models.PaymentMethod
		upiID      string
		wantStatus models.AppointmentPaymentStatus
		wantDue    bool
	}{
		{name: "a doctor with a upi id holds the slot for payment", methods: upiAndClinic, upiID: "asha@okbank", wantStatus: models.AppointmentPaymentUnpaid, wantDue: true},
		{name: "upi without the doctor's upi id is due at the clinic", methods: []models.PaymentMethod{models.PaymentMethodUPI}, wantStatus: models.AppointmentPaymentDueAtClinic},
		{name: "clinic alone is due at the clinic", methods: []models.PaymentMethod{models.PaymentMethodClinic}, upiID: "asha@okbank", wantStatus: models.AppointmentPaymentDueAtClinic},
		{name: "none mode is due at the clinic", methods: []models.PaymentMethod{}, wantStatus: models.AppointmentPaymentDueAtClinic},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			doctor := upiDoctor()
			doctor.ConsultationFee = 40000
			doctor.UPIID = tt.upiID
			appointments := &fakeAppointmentRepo{}
			service, _ := newTestServicesWithMethods(t, tt.methods, &fakeFileStorage{}, &fakeDoctorRepo{byID: doctor}, appointments, &fakeOutboxRepo{}, &fakeUserPatientRepo{activeResult: patientLink(models.AccessLevelFull)}, &fakePaymentRepo{}, &fakeGateway{}, now)

			if _, err := service.Book(context.Background(), 7, &models.Appointment{PatientID: 3, DoctorID: 5, StartsAt: nineAM, Mode: models.ConsultationModeInPerson}); err != nil {
				t.Fatalf("Book returned error: %v", err)
			}
			created := appointments.createInput
			if created.PaymentStatus != tt.wantStatus || tt.wantDue != (created.PaymentDueAt != nil) {
				t.Fatalf("Book payment = %q due %v, want %q with a hold %v", created.PaymentStatus, created.PaymentDueAt, tt.wantStatus, tt.wantDue)
			}
		})
	}
}

func TestPaymentServiceStartUPI(t *testing.T) {
	location := clinicLocation(t)
	now := time.Date(2026, 10, 25, 12, 0, 0, 0, location)
	nineAM := time.Date(2026, 10, 26, 9, 0, 0, 0, location)

	tests := []struct {
		name         string
		methods      []models.PaymentMethod
		upiID        string
		existing     *models.Payment
		wantErr      error
		wantExpired  bool
		wantExisting bool
	}{
		{name: "opens a direct upi payment", methods: upiAndClinic, upiID: "asha@okbank"},
		{name: "an open gateway order is expired first", methods: []models.PaymentMethod{models.PaymentMethodRazorpay, models.PaymentMethodUPI}, upiID: "asha@okbank", existing: &models.Payment{ID: 69, AppointmentID: 50, Provider: "fake", Status: models.PaymentStatusCreated}, wantExpired: true},
		{name: "asking again returns the open upi payment", methods: upiAndClinic, upiID: "asha@okbank", existing: upiPayment(models.PaymentStatusCreated), wantExisting: true},
		{name: "a doctor without a upi id", methods: upiAndClinic, wantErr: apperrors.ErrPaymentMethodUnavailable},
		{name: "upi turned off", methods: []models.PaymentMethod{models.PaymentMethodRazorpay}, upiID: "asha@okbank", wantErr: apperrors.ErrPaymentMethodUnavailable},
		{name: "a claim already made", methods: upiAndClinic, upiID: "asha@okbank", existing: upiPayment(models.PaymentStatusClaimed), wantErr: apperrors.ErrPaymentNotRequired},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			doctor := upiDoctor()
			doctor.UPIID = tt.upiID
			payments := &fakePaymentRepo{payment: tt.existing}
			_, service := newTestServicesWithMethods(t, tt.methods, &fakeFileStorage{}, &fakeDoctorRepo{byID: doctor}, &fakeAppointmentRepo{byID: unpaidAt(nineAM, now.Add(5*time.Minute))}, &fakeOutboxRepo{}, &fakeUserPatientRepo{activeResult: patientLink(models.AccessLevelFull)}, payments, &fakeGateway{}, now)

			payment, payload, err := service.StartUPI(context.Background(), 7, 50)
			if tt.wantErr != nil {
				if !errors.Is(err, tt.wantErr) {
					t.Fatalf("StartUPI error = %v, want %v", err, tt.wantErr)
				}
				return
			}
			if err != nil {
				t.Fatalf("StartUPI returned error: %v", err)
			}
			if payment.Provider != models.ProviderUPIDirect || payment.Status != models.PaymentStatusCreated || payment.Amount != 40000 {
				t.Fatalf("StartUPI payment = %+v, want a created upi_direct payment of 40000", payment)
			}
			link, _ := payload["upiLink"].(string)
			if payload["upiId"] != "asha@okbank" || payload["amount"] != "400.00" || !strings.HasPrefix(link, "upi://pay?") || !strings.Contains(link, "pa=asha%40okbank") || !strings.Contains(link, "am=400.00") {
				t.Fatalf("StartUPI payload = %v, want the doctor's upi id, 400.00 and a upi link", payload)
			}
			if tt.wantExisting && len(payments.changes) != 0 {
				t.Fatalf("StartUPI wrote %d history rows, want the open payment returned as is", len(payments.changes))
			}
			expired := slices.ContainsFunc(payments.changes, func(change *models.PaymentStatusChange) bool {
				return change.PaymentID == 69 && change.ToStatus == models.PaymentStatusExpired
			})
			if expired != tt.wantExpired {
				t.Fatalf("StartUPI expired the old order %v, want %v", expired, tt.wantExpired)
			}
		})
	}
}

func TestPaymentServicePayAtClinic(t *testing.T) {
	location := clinicLocation(t)
	now := time.Date(2026, 10, 25, 12, 0, 0, 0, location)
	nineAM := time.Date(2026, 10, 26, 9, 0, 0, 0, location)

	tests := []struct {
		name    string
		methods []models.PaymentMethod
		wantErr error
	}{
		{name: "clinic on sets the booking due at the clinic", methods: upiAndClinic},
		{name: "clinic off", methods: []models.PaymentMethod{models.PaymentMethodRazorpay, models.PaymentMethodUPI}, wantErr: apperrors.ErrPaymentMethodUnavailable},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			appointments := &fakeAppointmentRepo{byID: unpaidAt(nineAM, now.Add(5*time.Minute))}
			payments := &fakePaymentRepo{payment: upiPayment(models.PaymentStatusCreated)}
			_, service := newTestServicesWithMethods(t, tt.methods, &fakeFileStorage{}, &fakeDoctorRepo{byID: upiDoctor()}, appointments, &fakeOutboxRepo{}, &fakeUserPatientRepo{activeResult: patientLink(models.AccessLevelFull)}, payments, &fakeGateway{}, now)

			err := service.PayAtClinic(context.Background(), 7, 50)
			if tt.wantErr != nil {
				if !errors.Is(err, tt.wantErr) {
					t.Fatalf("PayAtClinic error = %v, want %v", err, tt.wantErr)
				}
				return
			}
			if err != nil {
				t.Fatalf("PayAtClinic returned error: %v", err)
			}
			if appointments.byID.PaymentStatus != models.AppointmentPaymentDueAtClinic || appointments.byID.PaymentDueAt != nil {
				t.Fatalf("PayAtClinic left the booking %q due %v, want due_at_clinic with no hold", appointments.byID.PaymentStatus, appointments.byID.PaymentDueAt)
			}
			if payments.payment.Status != models.PaymentStatusExpired {
				t.Fatalf("PayAtClinic left the open upi payment %q, want expired", payments.payment.Status)
			}
		})
	}
}

func TestPaymentServiceClaim(t *testing.T) {
	location := clinicLocation(t)
	now := time.Date(2026, 10, 25, 12, 0, 0, 0, location)
	nineAM := time.Date(2026, 10, 26, 9, 0, 0, 0, location)
	gatewayPayment := upiPayment(models.PaymentStatusCreated)
	gatewayPayment.Provider = "fake"

	tests := []struct {
		name        string
		appointment *models.AppointmentDetails
		payment     *models.Payment
		reference   string
		proof       []byte
		wantErr     error
	}{
		{name: "a reference and a screenshot hold the slot until the start", appointment: unpaidAt(nineAM, now.Add(5*time.Minute)), payment: upiPayment(models.PaymentStatusCreated), reference: "4123 5678 9012", proof: pngImage},
		{name: "a reference alone is enough", appointment: unpaidAt(nineAM, now.Add(5*time.Minute)), payment: upiPayment(models.PaymentStatusCreated), reference: "412356789012"},
		{name: "a short reference", appointment: unpaidAt(nineAM, now.Add(5*time.Minute)), payment: upiPayment(models.PaymentStatusCreated), reference: "12345", wantErr: apperrors.ErrInvalidPaymentInput},
		{name: "a reference with symbols", appointment: unpaidAt(nineAM, now.Add(5*time.Minute)), payment: upiPayment(models.PaymentStatusCreated), reference: "4123-5678", wantErr: apperrors.ErrInvalidPaymentInput},
		{name: "a screenshot that is not an image", appointment: unpaidAt(nineAM, now.Add(5*time.Minute)), payment: upiPayment(models.PaymentStatusCreated), reference: "412356789012", proof: []byte("%PDF-1.7"), wantErr: apperrors.ErrInvalidPaymentInput},
		{name: "after the payment window", appointment: unpaidAt(nineAM, now.Add(-time.Minute)), payment: upiPayment(models.PaymentStatusCreated), reference: "412356789012", wantErr: apperrors.ErrPaymentWindowClosed},
		{name: "a claim made twice", appointment: pendingAt(nineAM), payment: upiPayment(models.PaymentStatusClaimed), reference: "412356789012", wantErr: apperrors.ErrPaymentNotModifiable},
		{name: "a gateway payment cannot be claimed", appointment: unpaidAt(nineAM, now.Add(5*time.Minute)), payment: gatewayPayment, reference: "412356789012", wantErr: apperrors.ErrPaymentNotFound},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			appointments := &fakeAppointmentRepo{byID: tt.appointment}
			payments := &fakePaymentRepo{payment: tt.payment}
			storage := &fakeFileStorage{}
			outbox := &fakeOutboxRepo{}
			_, service := newTestServicesWithMethods(t, upiAndClinic, storage, &fakeDoctorRepo{byID: upiDoctor()}, appointments, outbox, &fakeUserPatientRepo{activeResult: patientLink(models.AccessLevelFull)}, payments, &fakeGateway{}, now)

			var proof io.Reader
			if tt.proof != nil {
				proof = bytes.NewReader(tt.proof)
			}
			claimed, err := service.Claim(context.Background(), 7, 70, tt.reference, proof)
			if tt.wantErr != nil {
				if !errors.Is(err, tt.wantErr) {
					t.Fatalf("Claim error = %v, want %v", err, tt.wantErr)
				}
				if len(storage.files) != 0 || len(outbox.events) != 0 {
					t.Fatalf("Claim kept %d files and %d events after an error", len(storage.files), len(outbox.events))
				}
				return
			}
			if err != nil {
				t.Fatalf("Claim returned error: %v", err)
			}
			if claimed.Status != models.PaymentStatusClaimed || claimed.PayerReference != "412356789012" || (tt.proof != nil) != (claimed.ProofStorageKey != nil) {
				t.Fatalf("Claim payment = %+v, want claimed with reference 412356789012", claimed)
			}
			if appointments.byID.PaymentStatus != models.AppointmentPaymentPendingVerification || appointments.byID.PaymentDueAt == nil || !appointments.byID.PaymentDueAt.Equal(nineAM) {
				t.Fatalf("Claim left the booking %q due %v, want pending_verification until %v", appointments.byID.PaymentStatus, appointments.byID.PaymentDueAt, nineAM)
			}
			if len(outbox.events) != 1 || outbox.events[0].EventType != models.OutboxEventPaymentClaimed {
				t.Fatalf("Claim outbox = %+v, want one %q event", outbox.events, models.OutboxEventPaymentClaimed)
			}
			if tt.proof != nil && !bytes.Equal(storage.files[*claimed.ProofStorageKey], tt.proof) {
				t.Fatalf("Claim stored %d bytes, want the screenshot", len(storage.files[*claimed.ProofStorageKey]))
			}
		})
	}
}

func TestPaymentServiceReviewClaim(t *testing.T) {
	location := clinicLocation(t)
	now := time.Date(2026, 10, 25, 12, 0, 0, 0, location)
	nineAM := time.Date(2026, 10, 26, 9, 0, 0, 0, location)
	otherDoctor := upiDoctor()
	otherDoctor.ID = 6

	tests := []struct {
		name            string
		reject          bool
		role            models.Role
		doctor          *models.Doctor
		payment         *models.Payment
		reason          string
		wantErr         error
		wantStatus      models.PaymentStatus
		wantAppointment models.AppointmentPaymentStatus
		wantEvents      []string
	}{
		{name: "the doctor verifies a claim", role: models.RoleDoctor, doctor: upiDoctor(), payment: upiPayment(models.PaymentStatusClaimed), wantStatus: models.PaymentStatusCaptured, wantAppointment: models.AppointmentPaymentPaid, wantEvents: []string{models.OutboxEventPaymentCaptured}},
		{name: "an admin verifies a claim", role: models.RoleAdmin, payment: upiPayment(models.PaymentStatusClaimed), wantStatus: models.PaymentStatusCaptured, wantAppointment: models.AppointmentPaymentPaid, wantEvents: []string{models.OutboxEventPaymentCaptured}},
		{name: "another doctor cannot see the claim", role: models.RoleDoctor, doctor: otherDoctor, payment: upiPayment(models.PaymentStatusClaimed), wantErr: apperrors.ErrPaymentNotFound},
		{name: "a patient cannot verify", role: models.RolePatient, payment: upiPayment(models.PaymentStatusClaimed), wantErr: apperrors.ErrPaymentNotFound},
		{name: "a payment not claimed yet", role: models.RoleDoctor, doctor: upiDoctor(), payment: upiPayment(models.PaymentStatusCreated), wantErr: apperrors.ErrPaymentNotModifiable},
		{name: "the doctor rejects a claim and the booking is cancelled", reject: true, role: models.RoleDoctor, doctor: upiDoctor(), payment: upiPayment(models.PaymentStatusClaimed), reason: "no such payment in my account", wantStatus: models.PaymentStatusRejected, wantAppointment: models.AppointmentPaymentUnpaid, wantEvents: []string{models.OutboxEventPaymentRejected, models.OutboxEventAppointmentCancelled}},
		{name: "a rejection needs a reason", reject: true, role: models.RoleDoctor, doctor: upiDoctor(), payment: upiPayment(models.PaymentStatusClaimed), reason: " ", wantErr: apperrors.ErrInvalidPaymentInput},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			appointments := &fakeAppointmentRepo{byID: pendingAt(nineAM)}
			payments := &fakePaymentRepo{payment: tt.payment}
			outbox := &fakeOutboxRepo{}
			_, service := newTestServicesWithMethods(t, upiAndClinic, &fakeFileStorage{}, &fakeDoctorRepo{byUserID: tt.doctor}, appointments, outbox, &fakeUserPatientRepo{}, payments, &fakeGateway{}, now)

			var err error
			if tt.reject {
				_, err = service.RejectClaim(context.Background(), 11, tt.role, 70, tt.reason)
			} else {
				_, err = service.VerifyClaim(context.Background(), 11, tt.role, 70, "seen in my bank app")
			}
			if tt.wantErr != nil {
				if !errors.Is(err, tt.wantErr) {
					t.Fatalf("review error = %v, want %v", err, tt.wantErr)
				}
				if len(outbox.events) != 0 {
					t.Fatalf("review wrote %d events after an error", len(outbox.events))
				}
				return
			}
			if err != nil {
				t.Fatalf("review returned error: %v", err)
			}
			if payments.payment.Status != tt.wantStatus || payments.payment.ReviewedByUserID != 11 || appointments.byID.PaymentStatus != tt.wantAppointment {
				t.Fatalf("review left the payment %q by %d and the booking %q, want %q by 11 and %q", payments.payment.Status, payments.payment.ReviewedByUserID, appointments.byID.PaymentStatus, tt.wantStatus, tt.wantAppointment)
			}
			if tt.reject && (appointments.byID.Status != models.AppointmentStatusCancelled || appointments.cancelReason != models.CancelReasonPaymentRejected || payments.payment.ReviewNote != tt.reason) {
				t.Fatalf("RejectClaim left the booking %q for %q with note %q, want cancelled for payment_rejected", appointments.byID.Status, appointments.cancelReason, payments.payment.ReviewNote)
			}
			events := []string{}
			for _, event := range outbox.events {
				events = append(events, event.EventType)
			}
			if !slices.Equal(events, tt.wantEvents) {
				t.Fatalf("review outbox = %v, want %v", events, tt.wantEvents)
			}
		})
	}
}

func TestPaymentServiceListClaimsScope(t *testing.T) {
	tests := []struct {
		name         string
		role         models.Role
		doctor       *models.Doctor
		wantDoctorID uint64
		wantErr      error
	}{
		{name: "a doctor sees their own claims", role: models.RoleDoctor, doctor: upiDoctor(), wantDoctorID: 5},
		{name: "an admin sees every claim", role: models.RoleAdmin, wantDoctorID: 0},
		{name: "a patient cannot list claims", role: models.RolePatient, wantErr: apperrors.ErrDoctorRoleRequired},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			payments := &fakePaymentRepo{payment: upiPayment(models.PaymentStatusClaimed), listedDoctorID: 99}
			_, service := newTestServicesWithMethods(t, upiAndClinic, &fakeFileStorage{}, &fakeDoctorRepo{byUserID: tt.doctor}, &fakeAppointmentRepo{byID: pendingAt(time.Now().Add(time.Hour))}, &fakeOutboxRepo{}, &fakeUserPatientRepo{}, payments, &fakeGateway{}, time.Now())

			claims, err := service.ListClaims(context.Background(), 11, tt.role, 20, 0)
			if tt.wantErr != nil {
				if !errors.Is(err, tt.wantErr) {
					t.Fatalf("ListClaims error = %v, want %v", err, tt.wantErr)
				}
				return
			}
			if err != nil {
				t.Fatalf("ListClaims returned error: %v", err)
			}
			if payments.listedDoctorID != tt.wantDoctorID || len(claims) != 1 || claims[0].Appointment.ID != 50 {
				t.Fatalf("ListClaims listed doctor %d with %d claims, want doctor %d with the claim and its booking", payments.listedDoctorID, len(claims), tt.wantDoctorID)
			}
		})
	}
}

func TestPaymentServiceMarkCollected(t *testing.T) {
	location := clinicLocation(t)
	nineAM := time.Date(2026, 10, 26, 9, 0, 0, 0, location)
	otherDoctor := upiDoctor()
	otherDoctor.ID = 6
	withPayment := func(appointmentStatus models.AppointmentStatus, paymentStatus models.AppointmentPaymentStatus) *models.AppointmentDetails {
		appointment := scheduledAt(nineAM, appointmentStatus)
		appointment.PaymentStatus = paymentStatus
		return appointment
	}

	tests := []struct {
		name        string
		doctor      *models.Doctor
		appointment *models.AppointmentDetails
		wantErr     error
	}{
		{name: "the doctor collects the fee of a scheduled visit", doctor: upiDoctor(), appointment: withPayment(models.AppointmentStatusScheduled, models.AppointmentPaymentDueAtClinic)},
		{name: "the doctor collects the fee after the visit", doctor: upiDoctor(), appointment: withPayment(models.AppointmentStatusCompleted, models.AppointmentPaymentDueAtClinic)},
		{name: "a booking already paid", doctor: upiDoctor(), appointment: withPayment(models.AppointmentStatusScheduled, models.AppointmentPaymentPaid), wantErr: apperrors.ErrPaymentNotRequired},
		{name: "a cancelled booking", doctor: upiDoctor(), appointment: withPayment(models.AppointmentStatusCancelled, models.AppointmentPaymentDueAtClinic), wantErr: apperrors.ErrAppointmentNotModifiable},
		{name: "another doctor's booking", doctor: otherDoctor, appointment: withPayment(models.AppointmentStatusScheduled, models.AppointmentPaymentDueAtClinic), wantErr: apperrors.ErrAppointmentNotFound},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			appointments := &fakeAppointmentRepo{byID: tt.appointment}
			payments := &fakePaymentRepo{}
			outbox := &fakeOutboxRepo{}
			_, service := newTestServicesWithMethods(t, []models.PaymentMethod{models.PaymentMethodClinic}, &fakeFileStorage{}, &fakeDoctorRepo{byUserID: tt.doctor}, appointments, outbox, &fakeUserPatientRepo{}, payments, &fakeGateway{}, nineAM)

			collected, err := service.MarkCollected(context.Background(), 11, models.RoleDoctor, 50, "cash")
			if tt.wantErr != nil {
				if !errors.Is(err, tt.wantErr) {
					t.Fatalf("MarkCollected error = %v, want %v", err, tt.wantErr)
				}
				if payments.payment != nil {
					t.Fatalf("MarkCollected recorded %+v after an error", payments.payment)
				}
				return
			}
			if err != nil {
				t.Fatalf("MarkCollected returned error: %v", err)
			}
			if collected.Provider != models.ProviderClinic || collected.Status != models.PaymentStatusCaptured || collected.Amount != 40000 || collected.ReviewNote != "cash" || collected.CapturedAt == nil {
				t.Fatalf("MarkCollected payment = %+v, want a captured clinic payment of 40000 noted cash", collected)
			}
			if appointments.paymentStatusTo != models.AppointmentPaymentPaid || len(outbox.events) != 1 || outbox.events[0].EventType != models.OutboxEventPaymentCaptured {
				t.Fatalf("MarkCollected set the booking %q with outbox %+v, want paid and one captured event", appointments.paymentStatusTo, outbox.events)
			}
		})
	}
}

func TestPaymentServiceCloseManualRefund(t *testing.T) {
	location := clinicLocation(t)
	nineAM := time.Date(2026, 10, 26, 9, 0, 0, 0, location)
	otherDoctor := upiDoctor()
	otherDoctor.ID = 6
	gatewayPayment := upiPayment(models.PaymentStatusCaptured)
	gatewayPayment.Provider = "fake"
	requested := func() []*models.Refund {
		return []*models.Refund{{ID: 90, PaymentID: 70, Amount: 40000, Reason: models.RefundReasonPatientCancelled, Status: models.RefundStatusRequested}}
	}
	closed := requested()
	closed[0].Status = models.RefundStatusSucceeded

	tests := []struct {
		name            string
		role            models.Role
		doctor          *models.Doctor
		payment         *models.Payment
		refunds         []*models.Refund
		refunded        bool
		wantErr         error
		wantRefund      models.RefundStatus
		wantPayment     models.PaymentStatus
		wantAppointment models.AppointmentPaymentStatus
	}{
		{name: "the doctor returns a verified payment", role: models.RoleDoctor, doctor: upiDoctor(), payment: upiPayment(models.PaymentStatusCaptured), refunds: requested(), refunded: true, wantRefund: models.RefundStatusSucceeded, wantPayment: models.PaymentStatusRefunded, wantAppointment: models.AppointmentPaymentRefunded},
		{name: "an admin closes an unchecked claim as returned", role: models.RoleAdmin, payment: upiPayment(models.PaymentStatusClaimed), refunds: requested(), refunded: true, wantRefund: models.RefundStatusSucceeded, wantPayment: models.PaymentStatusRefunded, wantAppointment: models.AppointmentPaymentRefunded},
		{name: "an unchecked claim that never arrived", role: models.RoleDoctor, doctor: upiDoctor(), payment: upiPayment(models.PaymentStatusClaimed), refunds: requested(), wantRefund: models.RefundStatusFailed, wantPayment: models.PaymentStatusRejected, wantAppointment: models.AppointmentPaymentUnpaid},
		{name: "a verified payment cannot be closed as never received", role: models.RoleDoctor, doctor: upiDoctor(), payment: upiPayment(models.PaymentStatusCaptured), refunds: requested(), wantErr: apperrors.ErrPaymentNotModifiable},
		{name: "a refund already closed", role: models.RoleDoctor, doctor: upiDoctor(), payment: upiPayment(models.PaymentStatusRefunded), refunds: closed, refunded: true, wantErr: apperrors.ErrPaymentNotModifiable},
		{name: "another doctor's refund", role: models.RoleDoctor, doctor: otherDoctor, payment: upiPayment(models.PaymentStatusCaptured), refunds: requested(), refunded: true, wantErr: apperrors.ErrRefundNotFound},
		{name: "a gateway refund is not closed by hand", role: models.RoleAdmin, payment: gatewayPayment, refunds: requested(), refunded: true, wantErr: apperrors.ErrRefundNotFound},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			appointments := &fakeAppointmentRepo{byID: scheduledAt(nineAM, models.AppointmentStatusCancelled)}
			payments := &fakePaymentRepo{payment: tt.payment, refunds: tt.refunds}
			_, service := newTestServicesWithMethods(t, upiAndClinic, &fakeFileStorage{}, &fakeDoctorRepo{byUserID: tt.doctor}, appointments, &fakeOutboxRepo{}, &fakeUserPatientRepo{}, payments, &fakeGateway{}, nineAM)

			err := service.CloseManualRefund(context.Background(), 11, tt.role, 90, tt.refunded, "sent back on GPay")
			if tt.wantErr != nil {
				if !errors.Is(err, tt.wantErr) {
					t.Fatalf("CloseManualRefund error = %v, want %v", err, tt.wantErr)
				}
				return
			}
			if err != nil {
				t.Fatalf("CloseManualRefund returned error: %v", err)
			}
			if payments.refunds[0].Status != tt.wantRefund || payments.payment.Status != tt.wantPayment || appointments.paymentStatusTo != tt.wantAppointment {
				t.Fatalf("CloseManualRefund left refund %q payment %q booking %q, want %q %q %q", payments.refunds[0].Status, payments.payment.Status, appointments.paymentStatusTo, tt.wantRefund, tt.wantPayment, tt.wantAppointment)
			}
		})
	}
}

func TestPaymentServiceManualRefundsDue(t *testing.T) {
	location := clinicLocation(t)
	nineAM := time.Date(2026, 10, 26, 9, 0, 0, 0, location)
	dayBefore := nineAM.Add(-21 * time.Hour)

	t.Run("a claim unchecked at the start is cancelled and owed back", func(t *testing.T) {
		appointment := pendingAt(nineAM)
		appointments := &fakeAppointmentRepo{unpaidDue: []*models.AppointmentDetails{appointment}}
		payments := &fakePaymentRepo{payment: upiPayment(models.PaymentStatusClaimed)}
		outbox := &fakeOutboxRepo{}
		gateway := &fakeGateway{}
		_, service := newTestServicesWithMethods(t, upiAndClinic, &fakeFileStorage{}, &fakeDoctorRepo{}, appointments, outbox, &fakeUserPatientRepo{}, payments, gateway, nineAM)

		expired, err := service.ExpireUnpaid(context.Background(), 10)
		if err != nil || expired != 1 {
			t.Fatalf("ExpireUnpaid = %d, %v; want 1", expired, err)
		}
		if len(outbox.events) != 1 || outbox.events[0].Payload["cancelReason"] != models.CancelReasonPaymentUnverified {
			t.Fatalf("ExpireUnpaid outbox = %+v, want one cancelled event with reason payment_unverified", outbox.events)
		}
		if len(payments.refunds) != 1 || payments.refunds[0].Status != models.RefundStatusRequested || gateway.refunded != 0 || payments.payment.Status != models.PaymentStatusClaimed {
			t.Fatalf("ExpireUnpaid refunds %+v with %d sent to the gateway, want one refund requested from the doctor", payments.refunds, gateway.refunded)
		}
	})

	t.Run("cancelling a claimed booking leaves a refund due from the doctor", func(t *testing.T) {
		payments := &fakePaymentRepo{payment: upiPayment(models.PaymentStatusClaimed)}
		gateway := &fakeGateway{}
		service, _ := newTestServicesWithMethods(t, upiAndClinic, &fakeFileStorage{}, &fakeDoctorRepo{}, &fakeAppointmentRepo{byID: pendingAt(nineAM)}, &fakeOutboxRepo{}, &fakeUserPatientRepo{activeResult: patientLink(models.AccessLevelFull)}, payments, gateway, dayBefore)

		if _, err := service.Cancel(context.Background(), 7, models.RolePatient, 50, ""); err != nil {
			t.Fatalf("Cancel returned error: %v", err)
		}
		if len(payments.refunds) != 1 || payments.refunds[0].Status != models.RefundStatusRequested || payments.refunds[0].Reason != models.RefundReasonPatientCancelled || gateway.refunded != 0 {
			t.Fatalf("Cancel refunds %+v with %d sent to the gateway, want one patient refund requested from the doctor", payments.refunds, gateway.refunded)
		}
	})

	t.Run("reconciling leaves manual refunds alone", func(t *testing.T) {
		payments := &fakePaymentRepo{payment: upiPayment(models.PaymentStatusClaimed), refunds: []*models.Refund{{ID: 90, PaymentID: 70, Status: models.RefundStatusRequested}}}
		gateway := &fakeGateway{}
		_, service := newTestServicesWithMethods(t, upiAndClinic, &fakeFileStorage{}, &fakeDoctorRepo{}, &fakeAppointmentRepo{}, &fakeOutboxRepo{}, &fakeUserPatientRepo{}, payments, gateway, nineAM)

		settled, err := service.ReconcileRefunds(context.Background(), 10)
		if err != nil || settled != 0 || gateway.asked != 0 || payments.refunds[0].Status != models.RefundStatusRequested {
			t.Fatalf("ReconcileRefunds = %d, %v asking the gateway %d times; want the manual refund left requested", settled, err, gateway.asked)
		}
	})
}

func TestPaymentServiceDoctorUPI(t *testing.T) {
	t.Run("the upi id", func(t *testing.T) {
		tests := []struct {
			name    string
			role    models.Role
			upiID   string
			wantID  string
			wantErr error
		}{
			{name: "a valid upi id", role: models.RoleDoctor, upiID: " asha.rao@okicici ", wantID: "asha.rao@okicici"},
			{name: "an empty id turns direct upi off", role: models.RoleDoctor, upiID: "", wantID: ""},
			{name: "a phone number is not a upi id", role: models.RoleDoctor, upiID: "9876543210", wantErr: apperrors.ErrInvalidDoctorInput},
			{name: "a bank part with digits", role: models.RoleDoctor, upiID: "asha@ok1", wantErr: apperrors.ErrInvalidDoctorInput},
			{name: "only a doctor sets it", role: models.RolePatient, upiID: "asha@okbank", wantErr: apperrors.ErrDoctorRoleRequired},
		}

		for _, tt := range tests {
			t.Run(tt.name, func(t *testing.T) {
				qrKey := "doctors/5/upi-qr-old"
				doctor := upiDoctor()
				doctor.UPIQRStorageKey = &qrKey
				doctors := &fakeDoctorRepo{byUserID: doctor, byID: doctor}
				storage := &fakeFileStorage{files: map[string][]byte{qrKey: pngImage}}
				_, service := newTestServicesWithMethods(t, upiAndClinic, storage, doctors, &fakeAppointmentRepo{}, &fakeOutboxRepo{}, &fakeUserPatientRepo{}, &fakePaymentRepo{}, &fakeGateway{}, time.Now())

				updated, err := service.SetDoctorUPI(context.Background(), 11, tt.role, tt.upiID)
				if tt.wantErr != nil {
					if !errors.Is(err, tt.wantErr) {
						t.Fatalf("SetDoctorUPI error = %v, want %v", err, tt.wantErr)
					}
					return
				}
				if err != nil {
					t.Fatalf("SetDoctorUPI returned error: %v", err)
				}
				if updated.UPIID != tt.wantID || doctors.upiID != tt.wantID {
					t.Fatalf("SetDoctorUPI saved %q, want %q", doctors.upiID, tt.wantID)
				}
				if tt.wantID == "" && (doctors.upiQRKey != nil || len(storage.files) != 0) {
					t.Fatalf("SetDoctorUPI kept the qr %v with %d files, want it removed with the upi id", doctors.upiQRKey, len(storage.files))
				}
				if tt.wantID != "" && (doctors.upiQRKey == nil || *doctors.upiQRKey != qrKey) {
					t.Fatalf("SetDoctorUPI changed the qr to %v, want it kept", doctors.upiQRKey)
				}
			})
		}
	})

	t.Run("the qr image", func(t *testing.T) {
		tests := []struct {
			name    string
			upiID   string
			content []byte
			wantErr error
		}{
			{name: "a png replaces the old qr", upiID: "asha@okbank", content: pngImage},
			{name: "a jpeg", upiID: "asha@okbank", content: []byte("\xff\xd8\xff\xe0\x00\x10JFIF\x00")},
			{name: "not an image", upiID: "asha@okbank", content: []byte("<svg></svg>"), wantErr: apperrors.ErrInvalidPaymentInput},
			{name: "too large", upiID: "asha@okbank", content: append(append([]byte{}, pngImage...), make([]byte, constant.MaxPaymentImageBytes)...), wantErr: apperrors.ErrInvalidPaymentInput},
			{name: "the upi id comes first", content: pngImage, wantErr: apperrors.ErrInvalidDoctorInput},
		}

		for _, tt := range tests {
			t.Run(tt.name, func(t *testing.T) {
				oldKey := "doctors/5/upi-qr-old"
				doctor := upiDoctor()
				doctor.UPIID = tt.upiID
				doctor.UPIQRStorageKey = &oldKey
				doctors := &fakeDoctorRepo{byUserID: doctor, byID: doctor}
				storage := &fakeFileStorage{files: map[string][]byte{oldKey: pngImage}}
				_, service := newTestServicesWithMethods(t, upiAndClinic, storage, doctors, &fakeAppointmentRepo{}, &fakeOutboxRepo{}, &fakeUserPatientRepo{}, &fakePaymentRepo{}, &fakeGateway{}, time.Now())

				updated, err := service.SetDoctorUPIQR(context.Background(), 11, models.RoleDoctor, bytes.NewReader(tt.content))
				if tt.wantErr != nil {
					if !errors.Is(err, tt.wantErr) {
						t.Fatalf("SetDoctorUPIQR error = %v, want %v", err, tt.wantErr)
					}
					if len(storage.files) != 1 || storage.files[oldKey] == nil {
						t.Fatalf("SetDoctorUPIQR left %d files, want only the old qr", len(storage.files))
					}
					return
				}
				if err != nil {
					t.Fatalf("SetDoctorUPIQR returned error: %v", err)
				}
				if updated.UPIQRStorageKey == nil || *updated.UPIQRStorageKey == oldKey || !bytes.Equal(storage.files[*updated.UPIQRStorageKey], tt.content) || storage.files[oldKey] != nil {
					t.Fatalf("SetDoctorUPIQR saved %v with files %d, want a new key holding the image and the old one deleted", updated.UPIQRStorageKey, len(storage.files))
				}
			})
		}
	})
}
