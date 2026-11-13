package handler_test

import (
	"appointment_system/internal/apps/appointment/adapters/payment/none"
	"appointment_system/internal/apps/appointment/apperrors"
	"appointment_system/internal/apps/appointment/controller"
	"appointment_system/internal/apps/appointment/handler"
	"appointment_system/internal/apps/appointment/models"
	"appointment_system/internal/apps/appointment/services"
	"appointment_system/middleware"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
)

func newAppointmentRouter(
	t *testing.T,
	doctors *fakeDoctorRepo,
	userPatients *fakeUserPatientRepo,
	appointments *fakeAppointmentRepo,
	rules []*models.AvailabilityRule,
) *gin.Engine {
	t.Helper()
	gin.SetMode(gin.TestMode)

	availabilityService, err := services.NewAvailabilityService(doctors, &fakeAvailabilityRepo{rules: rules}, appointments, &fakeTxManager{}, time.UTC, time.Now, discardLogger)
	if err != nil {
		t.Fatalf("NewAvailabilityService returned error: %v", err)
	}
	paymentService, err := services.NewPaymentService(&fakePaymentRepo{}, appointments, &fakeOutboxRepo{}, userPatients, doctors, none.New(), []models.PaymentMethod{models.PaymentMethodRazorpay}, &fakeFileStorage{}, &fakeTxManager{}, time.Now, discardLogger)
	if err != nil {
		t.Fatalf("NewPaymentService returned error: %v", err)
	}
	appointmentService, err := services.NewAppointmentService(appointments, &fakeOutboxRepo{}, userPatients, doctors, availabilityService, paymentService, &fakeCallRoomRepo{}, &fakeCallEventRepo{}, &fakeTxManager{}, discardLogger)
	if err != nil {
		t.Fatalf("NewAppointmentService returned error: %v", err)
	}
	appointmentController, err := controller.NewAppointmentController(appointmentService)
	if err != nil {
		t.Fatalf("NewAppointmentController returned error: %v", err)
	}
	appointmentHandler, err := handler.NewAppointmentHandler(appointmentController, &fakeRoleTokenVerifier{})
	if err != nil {
		t.Fatalf("NewAppointmentHandler returned error: %v", err)
	}

	router := gin.New()
	router.Use(middleware.AppContext())
	appointmentHandler.RegisterRoutes(router.Group("/api/v1"))

	return router
}

func TestAppointmentHandlerStatusCodes(t *testing.T) {
	nextWeek := time.Now().UTC().AddDate(0, 0, 7)
	date := nextWeek.Format(time.DateOnly)
	nineAM := time.Date(nextWeek.Year(), nextWeek.Month(), nextWeek.Day(), 9, 0, 0, 0, time.UTC)
	rules := []*models.AvailabilityRule{{DayOfWeek: nextWeek.Weekday(), StartMinute: 9 * 60, EndMinute: 10 * 60, Mode: models.ConsultationModeInPerson}}
	bookPayload := `{"patientId":3,"doctorId":5,"startsAt":"` + date + `T09:00:00Z","mode":"in_person","notes":"fever"}`

	verified := doctorWithStatus(models.VerificationStatusVerified)
	verified.ConsultationModes = []models.ConsultationMode{models.ConsultationModeInPerson}
	fullLink := &models.UserPatient{UserID: 7, PatientID: 3, PatientRelationship: models.RelationshipChild, AccessLevel: models.AccessLevelFull, Status: models.UserPatientStatusActive}
	readOnlyLink := &models.UserPatient{UserID: 7, PatientID: 3, PatientRelationship: models.RelationshipChild, AccessLevel: models.AccessLevelReadOnly, Status: models.UserPatientStatusActive}
	existing := func(status models.AppointmentStatus) *models.AppointmentDetails {
		return &models.AppointmentDetails{Appointment: models.Appointment{
			ID:            50,
			PatientID:     3,
			DoctorID:      5,
			StartsAt:      nineAM,
			EndsAt:        nineAM.Add(15 * time.Minute),
			Mode:          models.ConsultationModeInPerson,
			Status:        status,
			PaymentStatus: models.AppointmentPaymentPaid,
		}}
	}
	started := func() *models.AppointmentDetails {
		appointment := existing(models.AppointmentStatusScheduled)
		appointment.StartsAt = time.Now().Add(-time.Hour)
		appointment.EndsAt = appointment.StartsAt.Add(15 * time.Minute)
		return appointment
	}
	unpaid := func() *models.AppointmentDetails {
		appointment := started()
		appointment.PaymentStatus = models.AppointmentPaymentUnpaid
		return appointment
	}
	proposalPayload := `{"startsAt":"` + date + `T09:15:00Z","reason":"emergency","note":"surgery"}`
	pending := func() *models.RescheduleProposal {
		return &models.RescheduleProposal{
			ID: 80, AppointmentID: 50, PlaceholderAppointmentID: 50, Reason: models.ProposalReasonEmergency, Status: models.ProposalStatusPending,
			ExpiresAt: time.Now().Add(time.Hour), NewStartsAt: nineAM.Add(15 * time.Minute), NewEndsAt: nineAM.Add(30 * time.Minute), NewMode: models.ConsultationModeInPerson,
		}
	}
	unpaidBooking := func() *models.AppointmentDetails {
		appointment := existing(models.AppointmentStatusScheduled)
		appointment.PaymentStatus = models.AppointmentPaymentUnpaid
		return appointment
	}
	reschedulePayload := `{"startsAt":"` + date + `T09:15:00Z"}`

	tests := []struct {
		name       string
		method     string
		path       string
		body       string
		token      string
		link       *models.UserPatient
		byID       *models.AppointmentDetails
		createErr  error
		header     string
		proposal   *models.RescheduleProposal
		wantStatus int
		wantBody   string
	}{
		{name: "book without token", method: http.MethodPost, path: "/api/v1/appointments", body: bookPayload, wantStatus: http.StatusUnauthorized},
		{name: "doctor account cannot book", method: http.MethodPost, path: "/api/v1/appointments", body: bookPayload, token: "doctor-token", link: fullLink, wantStatus: http.StatusForbidden},
		{name: "patient books a free slot", method: http.MethodPost, path: "/api/v1/appointments", body: bookPayload, token: "patient-token", link: fullLink, wantStatus: http.StatusCreated, wantBody: `"startTime":"09:00"`},
		{name: "book reports the booker relationship", method: http.MethodPost, path: "/api/v1/appointments", body: bookPayload, token: "patient-token", link: fullLink, wantStatus: http.StatusCreated, wantBody: `"patientRelationship":"child"`},
		{name: "book without a patient", method: http.MethodPost, path: "/api/v1/appointments", body: `{"doctorId":5,"startsAt":"` + date + `T09:00:00Z","mode":"in_person"}`, token: "patient-token", wantStatus: http.StatusBadRequest, wantBody: `is required"`},
		{name: "book with a time without offset", method: http.MethodPost, path: "/api/v1/appointments", body: strings.Replace(bookPayload, "T09:00:00Z", "T09:00:00", 1), token: "patient-token", link: fullLink, wantStatus: http.StatusBadRequest, wantBody: `"details":"startsAt must be a time with its UTC offset`},
		{name: "book a time that is not a slot", method: http.MethodPost, path: "/api/v1/appointments", body: strings.Replace(bookPayload, "T09:00:00Z", "T09:05:00Z", 1), token: "patient-token", link: fullLink, wantStatus: http.StatusUnprocessableEntity},
		{name: "book a slot taken meanwhile", method: http.MethodPost, path: "/api/v1/appointments", body: bookPayload, token: "patient-token", link: fullLink, createErr: apperrors.ErrSlotTaken, wantStatus: http.StatusConflict},
		{name: "book for a patient without a link", method: http.MethodPost, path: "/api/v1/appointments", body: bookPayload, token: "patient-token", wantStatus: http.StatusNotFound},
		{name: "book with read_only access", method: http.MethodPost, path: "/api/v1/appointments", body: bookPayload, token: "patient-token", link: readOnlyLink, wantStatus: http.StatusForbidden},
		{name: "get an appointment of a linked patient", method: http.MethodGet, path: "/api/v1/appointments/50", token: "patient-token", link: readOnlyLink, byID: existing(models.AppointmentStatusScheduled), wantStatus: http.StatusOK, wantBody: `"status":"scheduled"`},
		{name: "get a missing appointment", method: http.MethodGet, path: "/api/v1/appointments/51", token: "patient-token", link: fullLink, wantStatus: http.StatusNotFound},
		{name: "get with an invalid id", method: http.MethodGet, path: "/api/v1/appointments/abc", token: "patient-token", wantStatus: http.StatusBadRequest},
		{name: "patient cancels a week ahead", method: http.MethodPost, path: "/api/v1/appointments/50/cancel", body: `{"reason":"feeling better"}`, token: "patient-token", link: fullLink, byID: existing(models.AppointmentStatusScheduled), wantStatus: http.StatusOK, wantBody: `"cancelReason":"patient_request"`},
		{name: "cancel without a body", method: http.MethodPost, path: "/api/v1/appointments/50/cancel", token: "patient-token", link: fullLink, byID: existing(models.AppointmentStatusScheduled), wantStatus: http.StatusOK},
		{name: "cancel a completed appointment", method: http.MethodPost, path: "/api/v1/appointments/50/cancel", token: "patient-token", link: fullLink, byID: existing(models.AppointmentStatusCompleted), wantStatus: http.StatusConflict},
		{name: "cancel with read_only access", method: http.MethodPost, path: "/api/v1/appointments/50/cancel", token: "patient-token", link: readOnlyLink, byID: existing(models.AppointmentStatusScheduled), wantStatus: http.StatusForbidden},
		{name: "list appointments", method: http.MethodGet, path: "/api/v1/appointments?status=scheduled", token: "patient-token", wantStatus: http.StatusOK, wantBody: `"appointments":[]`},
		{name: "list with an unknown status", method: http.MethodGet, path: "/api/v1/appointments?status=booked", token: "patient-token", wantStatus: http.StatusBadRequest, wantBody: `"details":"status must be one of`},
		{name: "list with a bad from", method: http.MethodGet, path: "/api/v1/appointments?from=tomorrow", token: "patient-token", wantStatus: http.StatusBadRequest},
		{name: "book with a key that is too long", method: http.MethodPost, path: "/api/v1/appointments", body: bookPayload, token: "patient-token", link: fullLink, header: strings.Repeat("k", 101), wantStatus: http.StatusBadRequest, wantBody: `"details":"Idempotency-Key must be at most 100 characters"`},
		{name: "book with a key", method: http.MethodPost, path: "/api/v1/appointments", body: bookPayload, token: "patient-token", link: fullLink, header: "retry-1", wantStatus: http.StatusCreated},
		{name: "patient reschedules to a free slot", method: http.MethodPost, path: "/api/v1/appointments/50/reschedule", body: reschedulePayload, token: "patient-token", link: fullLink, byID: existing(models.AppointmentStatusScheduled), wantStatus: http.StatusCreated, wantBody: `"rescheduledFromId":50`},
		{name: "reschedule without a time", method: http.MethodPost, path: "/api/v1/appointments/50/reschedule", body: `{}`, token: "patient-token", link: fullLink, byID: existing(models.AppointmentStatusScheduled), wantStatus: http.StatusBadRequest},
		{name: "reschedule to a time that is not a slot", method: http.MethodPost, path: "/api/v1/appointments/50/reschedule", body: strings.Replace(reschedulePayload, "09:15", "09:20", 1), token: "patient-token", link: fullLink, byID: existing(models.AppointmentStatusScheduled), wantStatus: http.StatusUnprocessableEntity},
		{name: "reschedule with read_only access", method: http.MethodPost, path: "/api/v1/appointments/50/reschedule", body: reschedulePayload, token: "patient-token", link: readOnlyLink, byID: existing(models.AppointmentStatusScheduled), wantStatus: http.StatusForbidden},
		{name: "doctor account cannot use the patient reschedule", method: http.MethodPost, path: "/api/v1/appointments/50/reschedule", body: reschedulePayload, token: "doctor-token", byID: existing(models.AppointmentStatusScheduled), wantStatus: http.StatusForbidden},
		{name: "reschedule a cancelled appointment", method: http.MethodPost, path: "/api/v1/appointments/50/reschedule", body: reschedulePayload, token: "patient-token", link: fullLink, byID: existing(models.AppointmentStatusCancelled), wantStatus: http.StatusConflict},
		{name: "doctor completes a started appointment", method: http.MethodPost, path: "/api/v1/doctors/me/appointments/50/complete", token: "doctor-token", byID: started(), wantStatus: http.StatusOK, wantBody: `"status":"completed"`},
		{name: "doctor marks a started appointment as a no-show", method: http.MethodPost, path: "/api/v1/doctors/me/appointments/50/no-show", token: "doctor-token", byID: started(), wantStatus: http.StatusOK, wantBody: `"status":"no_show"`},
		{name: "complete an unpaid appointment", method: http.MethodPost, path: "/api/v1/doctors/me/appointments/50/complete", token: "doctor-token", byID: unpaid(), wantStatus: http.StatusUnprocessableEntity, wantBody: `"error":"Payment is pending"`},
		{name: "complete before the start", method: http.MethodPost, path: "/api/v1/doctors/me/appointments/50/complete", token: "doctor-token", byID: existing(models.AppointmentStatusScheduled), wantStatus: http.StatusConflict},
		{name: "complete a missing appointment", method: http.MethodPost, path: "/api/v1/doctors/me/appointments/51/complete", token: "doctor-token", wantStatus: http.StatusNotFound},
		{name: "patient cannot complete", method: http.MethodPost, path: "/api/v1/doctors/me/appointments/50/complete", token: "patient-token", byID: started(), wantStatus: http.StatusForbidden},
		{name: "doctor proposes a free slot", method: http.MethodPost, path: "/api/v1/doctors/me/appointments/50/reschedule-proposals", body: proposalPayload, token: "doctor-token", byID: existing(models.AppointmentStatusScheduled), wantStatus: http.StatusCreated, wantBody: `"newStartTime":"09:15"`},
		{name: "proposal without a reason", method: http.MethodPost, path: "/api/v1/doctors/me/appointments/50/reschedule-proposals", body: `{"startsAt":"` + date + `T09:15:00Z"}`, token: "doctor-token", byID: existing(models.AppointmentStatusScheduled), wantStatus: http.StatusBadRequest},
		{name: "proposal with an unknown reason", method: http.MethodPost, path: "/api/v1/doctors/me/appointments/50/reschedule-proposals", body: strings.Replace(proposalPayload, "emergency", "bored", 1), token: "doctor-token", byID: existing(models.AppointmentStatusScheduled), wantStatus: http.StatusBadRequest, wantBody: `"details":"reason must be one of emergency, personal, clinic_closed, other"`},
		{name: "proposal for a booking awaiting payment", method: http.MethodPost, path: "/api/v1/doctors/me/appointments/50/reschedule-proposals", body: proposalPayload, token: "doctor-token", byID: unpaidBooking(), wantStatus: http.StatusConflict},
		{name: "patient cannot propose", method: http.MethodPost, path: "/api/v1/doctors/me/appointments/50/reschedule-proposals", body: proposalPayload, token: "patient-token", byID: existing(models.AppointmentStatusScheduled), wantStatus: http.StatusForbidden},
		{name: "patient sees the pending proposal", method: http.MethodGet, path: "/api/v1/appointments/50/reschedule-proposal", token: "patient-token", link: readOnlyLink, byID: existing(models.AppointmentStatusRescheduleProposed), proposal: pending(), wantStatus: http.StatusOK, wantBody: `"status":"pending"`},
		{name: "no proposal is waiting", method: http.MethodGet, path: "/api/v1/appointments/50/reschedule-proposal", token: "patient-token", link: fullLink, byID: existing(models.AppointmentStatusScheduled), wantStatus: http.StatusNotFound},
		{name: "patient accepts the new time", method: http.MethodPost, path: "/api/v1/appointments/50/reschedule-proposal/accept", token: "patient-token", link: fullLink, byID: existing(models.AppointmentStatusRescheduleProposed), proposal: pending(), wantStatus: http.StatusOK, wantBody: `"status":"scheduled"`},
		{name: "patient declines the new time", method: http.MethodPost, path: "/api/v1/appointments/50/reschedule-proposal/decline", token: "patient-token", link: fullLink, byID: existing(models.AppointmentStatusRescheduleProposed), proposal: pending(), wantStatus: http.StatusOK},
		{name: "read_only access cannot answer", method: http.MethodPost, path: "/api/v1/appointments/50/reschedule-proposal/accept", token: "patient-token", link: readOnlyLink, byID: existing(models.AppointmentStatusRescheduleProposed), proposal: pending(), wantStatus: http.StatusForbidden},
		{name: "doctor account cannot answer", method: http.MethodPost, path: "/api/v1/appointments/50/reschedule-proposal/accept", token: "doctor-token", byID: existing(models.AppointmentStatusRescheduleProposed), proposal: pending(), wantStatus: http.StatusForbidden},
		{name: "answer when nothing was proposed", method: http.MethodPost, path: "/api/v1/appointments/50/reschedule-proposal/decline", token: "patient-token", link: fullLink, byID: existing(models.AppointmentStatusScheduled), wantStatus: http.StatusNotFound},
		{name: "cancel while a proposal waits", method: http.MethodPost, path: "/api/v1/appointments/50/cancel", token: "patient-token", link: fullLink, byID: existing(models.AppointmentStatusRescheduleProposed), proposal: pending(), wantStatus: http.StatusConflict, wantBody: `accept or decline it first`},
		{name: "doctor lists a day", method: http.MethodGet, path: "/api/v1/doctors/me/appointments?date=" + date, token: "doctor-token", wantStatus: http.StatusOK, wantBody: `"timezone":"UTC"`},
		{name: "doctor lists today by default", method: http.MethodGet, path: "/api/v1/doctors/me/appointments", token: "doctor-token", wantStatus: http.StatusOK, wantBody: `"date":"` + time.Now().UTC().Format(time.DateOnly) + `"`},
		{name: "doctor day with a bad date", method: http.MethodGet, path: "/api/v1/doctors/me/appointments?date=26-10-2026", token: "doctor-token", wantStatus: http.StatusBadRequest},
		{name: "patient cannot list a doctor day", method: http.MethodGet, path: "/api/v1/doctors/me/appointments", token: "patient-token", wantStatus: http.StatusForbidden},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			userPatients := &fakeUserPatientRepo{userPatient: tt.link}
			if tt.link == nil {
				userPatients.getErr = apperrors.ErrUserPatientNotFound
			}
			router := newAppointmentRouter(t, &fakeDoctorRepo{doctor: verified}, userPatients, &fakeAppointmentRepo{byID: tt.byID, createErr: tt.createErr, proposal: tt.proposal}, rules)

			request := httptest.NewRequest(tt.method, tt.path, strings.NewReader(tt.body))
			if tt.body != "" {
				request.Header.Set("Content-Type", "application/json")
			}
			if tt.token != "" {
				request.Header.Set("Authorization", "Bearer "+tt.token)
			}
			if tt.header != "" {
				request.Header.Set("Idempotency-Key", tt.header)
			}
			recorder := httptest.NewRecorder()

			router.ServeHTTP(recorder, request)

			if recorder.Code != tt.wantStatus {
				t.Fatalf("status = %d, want %d, body = %s", recorder.Code, tt.wantStatus, recorder.Body.String())
			}
			if tt.wantBody != "" && !strings.Contains(recorder.Body.String(), tt.wantBody) {
				t.Fatalf("body = %s, want it to contain %s", recorder.Body.String(), tt.wantBody)
			}
		})
	}
}

func TestAppointmentHandlerListPatients(t *testing.T) {
	today := time.Now().UTC()
	lastVisit := today.AddDate(0, 0, -3)
	patients := []*models.DoctorPatient{
		{PatientID: 3, Name: "Asha Verma", Gender: models.GenderFemale, DateOfBirth: today.AddDate(-34, 0, 0), LastAppointmentAt: &lastVisit, Visits: 3},
		{PatientID: 4, Name: "Ravi Verma", Gender: models.GenderMale, DateOfBirth: today.AddDate(-34, 0, 1), Visits: 1},
	}

	tests := []struct {
		name       string
		path       string
		token      string
		wantStatus int
		wantBody   []string
	}{
		{name: "doctor lists their patients", path: "/api/v1/doctors/me/patients?q=verma", token: "doctor-token", wantStatus: http.StatusOK, wantBody: []string{`"id":3,"name":"Asha Verma","age":34,"gender":"Female","lastAppointmentAt":`, `"visits":3`, `"id":4,"name":"Ravi Verma","age":33`}},
		{name: "a birthday tomorrow is not a year yet", path: "/api/v1/doctors/me/patients", token: "doctor-token", wantStatus: http.StatusOK, wantBody: []string{`"age":33,"gender":"Male","visits":1`}},
		{name: "patient cannot list a doctor's patients", path: "/api/v1/doctors/me/patients", token: "patient-token", wantStatus: http.StatusForbidden},
		{name: "admin cannot list a doctor's patients", path: "/api/v1/doctors/me/patients", token: "admin-token", wantStatus: http.StatusForbidden},
		{name: "list without token", path: "/api/v1/doctors/me/patients", wantStatus: http.StatusUnauthorized},
		{name: "list with a bad limit", path: "/api/v1/doctors/me/patients?limit=many", token: "doctor-token", wantStatus: http.StatusBadRequest},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			router := newAppointmentRouter(t, &fakeDoctorRepo{doctor: doctorWithStatus(models.VerificationStatusVerified)}, &fakeUserPatientRepo{}, &fakeAppointmentRepo{patients: patients}, nil)

			request := httptest.NewRequest(http.MethodGet, tt.path, nil)
			if tt.token != "" {
				request.Header.Set("Authorization", "Bearer "+tt.token)
			}
			recorder := httptest.NewRecorder()

			router.ServeHTTP(recorder, request)

			if recorder.Code != tt.wantStatus {
				t.Fatalf("status = %d, want %d, body = %s", recorder.Code, tt.wantStatus, recorder.Body.String())
			}
			for _, want := range tt.wantBody {
				if !strings.Contains(recorder.Body.String(), want) {
					t.Fatalf("body = %s, want it to contain %s", recorder.Body.String(), want)
				}
			}
		})
	}
}
