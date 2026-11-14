package handler_test

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"appointment_system/internal/apps/appointment/adapters/payment/none"
	"appointment_system/internal/apps/appointment/apperrors"
	"appointment_system/internal/apps/appointment/controller"
	"appointment_system/internal/apps/appointment/handler"
	"appointment_system/internal/apps/appointment/models"
	"appointment_system/internal/apps/appointment/services"
	"appointment_system/internal/apps/appointment/utils"
	"appointment_system/middleware"

	"github.com/gin-gonic/gin"
)

type staffDeskFixture struct {
	doctors        *fakeDoctorRepo
	staff          *fakeStaffRepo
	invites        *fakeStaffInviteRepo
	clinicPatients *fakeClinicPatientRepo
	passes         *fakePatientPassRepo
	userPatients   *fakeUserPatientRepo
	appointments   *fakeAppointmentRepo
	sessions       *fakeSessionRepo
	rules          []*models.AvailabilityRule
}

func newStaffDeskFixture(rules []*models.AvailabilityRule) *staffDeskFixture {
	doctor := doctorWithStatus(models.VerificationStatusVerified)
	doctor.ConsultationModes = []models.ConsultationMode{models.ConsultationModeInPerson}
	doctor.ConsultationFee = 50000
	return &staffDeskFixture{
		doctors:        &fakeDoctorRepo{doctor: doctor},
		staff:          &fakeStaffRepo{active: map[uint64]*models.DoctorStaff{40: {DoctorID: 5, UserID: 40, CreatedAt: time.Now()}}},
		invites:        &fakeStaffInviteRepo{},
		clinicPatients: &fakeClinicPatientRepo{},
		passes:         &fakePatientPassRepo{},
		userPatients:   &fakeUserPatientRepo{userPatient: &models.UserPatient{UserID: 7, PatientID: 3, AccessLevel: models.AccessLevelFull, Status: models.UserPatientStatusActive}},
		appointments:   &fakeAppointmentRepo{},
		sessions:       &fakeSessionRepo{},
		rules:          rules,
	}
}

// router serves the staff, desk and appointment routes over the fixture.
func (f *staffDeskFixture) router(t *testing.T) *gin.Engine {
	t.Helper()
	gin.SetMode(gin.TestMode)

	clinic, err := services.NewClinicActor(f.doctors, f.staff, f.clinicPatients)
	if err != nil {
		t.Fatalf("NewClinicActor returned error: %v", err)
	}
	availabilityService, err := services.NewAvailabilityService(f.doctors, &fakeAvailabilityRepo{rules: f.rules}, f.appointments, &fakeTxManager{}, time.UTC, time.Now, discardLogger)
	if err != nil {
		t.Fatalf("NewAvailabilityService returned error: %v", err)
	}
	paymentService, err := services.NewPaymentService(&fakePaymentRepo{}, f.appointments, &fakeOutboxRepo{}, f.userPatients, f.doctors, clinic, none.New(), []models.PaymentMethod{models.PaymentMethodRazorpay}, &fakeFileStorage{}, &fakeTxManager{}, time.Now, discardLogger)
	if err != nil {
		t.Fatalf("NewPaymentService returned error: %v", err)
	}
	appointmentService, err := services.NewAppointmentService(f.appointments, &fakeOutboxRepo{}, f.userPatients, f.doctors, clinic, availabilityService, paymentService, &fakeCallRoomRepo{}, &fakeCallEventRepo{}, &fakeTxManager{}, discardLogger)
	if err != nil {
		t.Fatalf("NewAppointmentService returned error: %v", err)
	}
	staffService, err := services.NewStaffService(&fakeUserRepo{}, f.doctors, f.staff, f.invites, &fakeStaffInviteAttemptRepo{}, f.sessions, &fakeTxManager{}, time.Now, discardLogger)
	if err != nil {
		t.Fatalf("NewStaffService returned error: %v", err)
	}
	patients := &fakePatientRepo{patient: &models.Patient{ID: 3, Name: "Asha Verma", Gender: models.GenderFemale, BloodType: models.BloodTypeOPlus, DateOfBirth: time.Date(1990, 1, 2, 0, 0, 0, 0, time.UTC)}}
	inviteService, err := services.NewPatientInviteService(&fakePatientInviteRepo{}, &fakeInviteAttemptRepo{}, f.userPatients, &fakeUserPatientHistoryRepo{}, patients, &fakeOutboxRepo{}, &fakeTxManager{}, time.Now, discardLogger)
	if err != nil {
		t.Fatalf("NewPatientInviteService returned error: %v", err)
	}
	deskService, err := services.NewDeskService(clinic, patients, f.userPatients, f.passes, &fakePatientPassAttemptRepo{}, f.clinicPatients, inviteService, &fakeTxManager{}, time.Now, discardLogger)
	if err != nil {
		t.Fatalf("NewDeskService returned error: %v", err)
	}

	staffController, err := controller.NewStaffController(staffService)
	if err != nil {
		t.Fatalf("NewStaffController returned error: %v", err)
	}
	deskController, err := controller.NewDeskController(deskService, appointmentService)
	if err != nil {
		t.Fatalf("NewDeskController returned error: %v", err)
	}
	appointmentController, err := controller.NewAppointmentController(appointmentService)
	if err != nil {
		t.Fatalf("NewAppointmentController returned error: %v", err)
	}
	staffHandler, err := handler.NewStaffHandler(staffController, &fakeRoleTokenVerifier{})
	if err != nil {
		t.Fatalf("NewStaffHandler returned error: %v", err)
	}
	deskHandler, err := handler.NewDeskHandler(deskController, &fakeRoleTokenVerifier{})
	if err != nil {
		t.Fatalf("NewDeskHandler returned error: %v", err)
	}
	appointmentHandler, err := handler.NewAppointmentHandler(appointmentController, &fakeRoleTokenVerifier{})
	if err != nil {
		t.Fatalf("NewAppointmentHandler returned error: %v", err)
	}

	router := gin.New()
	router.Use(middleware.AppContext())
	group := router.Group("/api/v1")
	staffHandler.RegisterRoutes(group)
	deskHandler.RegisterRoutes(group)
	appointmentHandler.RegisterRoutes(group)

	return router
}

func TestStaffAndDeskHandlerStatusCodes(t *testing.T) {
	nextWeek := time.Now().UTC().AddDate(0, 0, 7)
	date := nextWeek.Format(time.DateOnly)
	rules := []*models.AvailabilityRule{{DayOfWeek: nextWeek.Weekday(), StartMinute: 9 * 60, EndMinute: 10 * 60, Mode: models.ConsultationModeInPerson}}
	bookPayload := `{"patientId":3,"startsAt":"` + date + `T09:00:00Z","mode":"in_person"}`
	walkInPayload := `{"name":"Walk In","gender":"Female","dateOfBirth":"1980-01-02","bloodType":"O+","contact":"9876543210"}`
	inviteCode := "K7Q2M9XD"
	withInvite := func(f *staffDeskFixture) {
		f.invites.invites = append(f.invites.invites, &models.StaffInvite{ID: 1, DoctorID: 5, CodeHash: utils.HashToken(inviteCode), Status: models.StaffInviteStatusPending, ExpiresAt: time.Now().Add(time.Hour)})
	}
	seenAtDesk := func(source models.ClinicPatientSource) func(f *staffDeskFixture) {
		return func(f *staffDeskFixture) {
			_ = f.clinicPatients.Add(context.Background(), &models.ClinicPatient{DoctorID: 5, PatientID: 3, Source: source})
		}
	}
	withPass := func(f *staffDeskFixture) {
		f.passes.passes = map[uint64]*models.PatientPass{3: {PatientID: 3, TokenHash: utils.HashToken("m2Rk9vQp0bW3xYzA1cDe4f"), CodeHash: utils.HashToken("ABCD2345"), ExpiresAt: time.Now().Add(5 * time.Minute)}}
	}

	tests := []struct {
		name       string
		method     string
		path       string
		body       string
		token      string
		setup      func(f *staffDeskFixture)
		wantStatus int
		wantBody   string
	}{
		{name: "staff sign up with an invite", method: http.MethodPost, path: "/api/v1/auth/register-staff", body: `{"name":"Ravi","email":"ravi@example.com","password":"staff-password","mobileNo":"9876543210","inviteCode":"k7q2-m9xd"}`, setup: withInvite, wantStatus: http.StatusCreated, wantBody: `"role":"staff"`},
		{name: "staff sign up with a wrong code", method: http.MethodPost, path: "/api/v1/auth/register-staff", body: `{"name":"Ravi","email":"ravi@example.com","password":"staff-password","mobileNo":"9876543210","inviteCode":"ZZZZZZZZ"}`, wantStatus: http.StatusNotFound},
		{name: "staff sign up without a code", method: http.MethodPost, path: "/api/v1/auth/register-staff", body: `{"name":"Ravi","email":"ravi@example.com","password":"staff-password","mobileNo":"9876543210"}`, wantStatus: http.StatusBadRequest},
		{name: "staff see their doctor", method: http.MethodGet, path: "/api/v1/staff/me", token: "staff-token", wantStatus: http.StatusOK, wantBody: `"fullName":"Dr. Asha Verma"`},
		{name: "unlinked staff have no doctor", method: http.MethodGet, path: "/api/v1/staff/me", token: "unlinked-staff-token", wantStatus: http.StatusNotFound},
		{name: "unlinked staff join with a code", method: http.MethodPost, path: "/api/v1/staff/me/join", body: `{"inviteCode":"K7Q2M9XD"}`, token: "unlinked-staff-token", setup: withInvite, wantStatus: http.StatusOK},
		{name: "doctor invites staff", method: http.MethodPost, path: "/api/v1/doctors/me/staff/invites", token: "doctor-token", wantStatus: http.StatusCreated, wantBody: `"code":"`},
		{name: "staff cannot invite staff", method: http.MethodPost, path: "/api/v1/doctors/me/staff/invites", token: "staff-token", wantStatus: http.StatusForbidden},
		{name: "doctor lists staff", method: http.MethodGet, path: "/api/v1/doctors/me/staff", token: "doctor-token", wantStatus: http.StatusOK},
		{name: "doctor removes staff", method: http.MethodDelete, path: "/api/v1/doctors/me/staff/40", token: "doctor-token", wantStatus: http.StatusOK},
		{name: "remove someone who is not staff", method: http.MethodDelete, path: "/api/v1/doctors/me/staff/99", token: "doctor-token", wantStatus: http.StatusNotFound},
		{name: "patient gets a clinic pass", method: http.MethodPost, path: "/api/v1/patients/3/pass", token: "patient-token", wantStatus: http.StatusCreated, wantBody: `"code":"`},
		{name: "staff cannot get a patient pass", method: http.MethodPost, path: "/api/v1/patients/3/pass", token: "staff-token", wantStatus: http.StatusForbidden},
		{name: "staff look up a pass token", method: http.MethodPost, path: "/api/v1/doctors/me/desk/lookup", body: `{"code":"m2Rk9vQp0bW3xYzA1cDe4f"}`, token: "staff-token", setup: withPass, wantStatus: http.StatusOK, wantBody: `"name":"Asha Verma"`},
		{name: "doctor types a pass code", method: http.MethodPost, path: "/api/v1/doctors/me/desk/lookup", body: `{"code":"abcd-2345"}`, token: "doctor-token", setup: withPass, wantStatus: http.StatusOK},
		{name: "a wrong pass code", method: http.MethodPost, path: "/api/v1/doctors/me/desk/lookup", body: `{"code":"ZZZZZZZZ"}`, token: "staff-token", setup: withPass, wantStatus: http.StatusNotFound},
		{name: "a patient cannot use the desk", method: http.MethodPost, path: "/api/v1/doctors/me/desk/lookup", body: `{"code":"m2Rk9vQp0bW3xYzA1cDe4f"}`, token: "patient-token", setup: withPass, wantStatus: http.StatusForbidden},
		{name: "unlinked staff cannot use the desk", method: http.MethodPost, path: "/api/v1/doctors/me/desk/lookup", body: `{"code":"m2Rk9vQp0bW3xYzA1cDe4f"}`, token: "unlinked-staff-token", setup: withPass, wantStatus: http.StatusForbidden},
		{name: "staff add a walk-in", method: http.MethodPost, path: "/api/v1/doctors/me/desk/patients", body: walkInPayload, token: "staff-token", wantStatus: http.StatusCreated},
		{name: "a walk-in without a blood group", method: http.MethodPost, path: "/api/v1/doctors/me/desk/patients", body: `{"name":"Walk In","gender":"Female","dateOfBirth":"1980-01-02"}`, token: "staff-token", wantStatus: http.StatusBadRequest},
		{name: "a claim code for the desk's walk-in", method: http.MethodPost, path: "/api/v1/doctors/me/desk/patients/3/claim-code", body: `{"relationship":"self"}`, token: "staff-token", setup: func(f *staffDeskFixture) {
			seenAtDesk(models.ClinicPatientSourceWalkIn)(f)
			f.userPatients = &fakeUserPatientRepo{}
		}, wantStatus: http.StatusCreated, wantBody: `"relationship":"self"`},
		{name: "a claim code for a patient with the app", method: http.MethodPost, path: "/api/v1/doctors/me/desk/patients/3/claim-code", body: `{"relationship":"self"}`, token: "staff-token", setup: seenAtDesk(models.ClinicPatientSourcePass), wantStatus: http.StatusConflict},
		{name: "staff book at the desk, paid at the clinic", method: http.MethodPost, path: "/api/v1/doctors/me/desk/appointments", body: bookPayload, token: "staff-token", setup: seenAtDesk(models.ClinicPatientSourcePass), wantStatus: http.StatusCreated, wantBody: `"paymentStatus":"due_at_clinic","bookedVia":"desk"`},
		{name: "staff book a patient the desk has never seen", method: http.MethodPost, path: "/api/v1/doctors/me/desk/appointments", body: bookPayload, token: "staff-token", wantStatus: http.StatusNotFound},
		{name: "a patient cannot book at the desk", method: http.MethodPost, path: "/api/v1/doctors/me/desk/appointments", body: bookPayload, token: "patient-token", setup: seenAtDesk(models.ClinicPatientSourcePass), wantStatus: http.StatusForbidden},
		{name: "staff see the doctor's day", method: http.MethodGet, path: "/api/v1/doctors/me/appointments?date=" + date, token: "staff-token", wantStatus: http.StatusOK},
		{name: "staff see the doctor's patients", method: http.MethodGet, path: "/api/v1/doctors/me/patients", token: "staff-token", wantStatus: http.StatusOK},
		{name: "staff cannot complete a visit", method: http.MethodPost, path: "/api/v1/doctors/me/appointments/50/complete", token: "staff-token", wantStatus: http.StatusForbidden},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			fixture := newStaffDeskFixture(rules)
			if tt.setup != nil {
				tt.setup(fixture)
			}
			router := fixture.router(t)

			request := httptest.NewRequest(tt.method, tt.path, strings.NewReader(tt.body))
			if tt.body != "" {
				request.Header.Set("Content-Type", "application/json")
			}
			if tt.token != "" {
				request.Header.Set("Authorization", "Bearer "+tt.token)
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

func TestStaffRemovalRevokesSessions(t *testing.T) {
	fixture := newStaffDeskFixture(nil)
	router := fixture.router(t)

	request := httptest.NewRequest(http.MethodDelete, "/api/v1/doctors/me/staff/40", nil)
	request.Header.Set("Authorization", "Bearer doctor-token")
	recorder := httptest.NewRecorder()
	router.ServeHTTP(recorder, request)

	if recorder.Code != http.StatusOK || fixture.sessions.revokedUserID != 40 {
		t.Fatalf("remove status = %d revoking sessions of %d, want 200 and staff 40 signed out", recorder.Code, fixture.sessions.revokedUserID)
	}
	if _, err := fixture.staff.GetActiveByUser(context.Background(), 40); err != apperrors.ErrStaffNotFound {
		t.Fatalf("staff 40 after removal: %v, want %v", err, apperrors.ErrStaffNotFound)
	}
}
