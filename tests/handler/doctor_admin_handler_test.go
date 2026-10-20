package handler_test

import (
	"appointment_system/internal/apps/appointment/apperrors"
	"appointment_system/internal/apps/appointment/controller"
	"appointment_system/internal/apps/appointment/handler"
	"appointment_system/internal/apps/appointment/models"
	"appointment_system/internal/apps/appointment/services"
	"appointment_system/middleware"
	"errors"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
)

const doctorPayload = `{"fullName":"Dr. Asha Verma","registrationNumber":"DMC/R/12345","registrationCouncil":"Delhi Medical Council","qualification":"MBBS","consultationFee":60000,"consultationModes":["in_person"],"specialities":["general-medicine"],"tags":["Diabetes"],"clinic":{"city":"New Delhi"}}`

func newDoctorAdminRouter(t *testing.T, doctors *fakeDoctorRepo) *gin.Engine {
	t.Helper()

	return newDoctorAdminRouterWithAvailability(t, doctors, &fakeAvailabilityRepo{})
}

func newDoctorAdminRouterWithAvailability(t *testing.T, doctors *fakeDoctorRepo, availability *fakeAvailabilityRepo) *gin.Engine {
	t.Helper()
	gin.SetMode(gin.TestMode)

	doctorService, err := services.NewDoctorService(doctors, &fakeSpecialityRepo{}, &fakeDoctorVerificationRepo{}, &fakeTxManager{})
	if err != nil {
		t.Fatalf("NewDoctorService returned error: %v", err)
	}
	doctorVerificationService, err := services.NewDoctorVerificationService(doctors, &fakeDoctorVerificationRepo{}, &fakeTxManager{})
	if err != nil {
		t.Fatalf("NewDoctorVerificationService returned error: %v", err)
	}
	adminUserService, err := services.NewAdminUserService(&fakeUserRepo{})
	if err != nil {
		t.Fatalf("NewAdminUserService returned error: %v", err)
	}
	availabilityService, err := services.NewAvailabilityService(doctors, availability, &fakeTxManager{}, time.UTC, time.Now)
	if err != nil {
		t.Fatalf("NewAvailabilityService returned error: %v", err)
	}
	doctorController, err := controller.NewDoctorController(doctorService, availabilityService)
	if err != nil {
		t.Fatalf("NewDoctorController returned error: %v", err)
	}
	adminController, err := controller.NewAdminController(adminUserService, doctorVerificationService, doctorService)
	if err != nil {
		t.Fatalf("NewAdminController returned error: %v", err)
	}
	doctorHandler, err := handler.NewDoctorHandler(doctorController, &fakeRoleTokenVerifier{})
	if err != nil {
		t.Fatalf("NewDoctorHandler returned error: %v", err)
	}
	adminHandler, err := handler.NewAdminHandler(adminController, &fakeRoleTokenVerifier{})
	if err != nil {
		t.Fatalf("NewAdminHandler returned error: %v", err)
	}

	router := gin.New()
	router.Use(middleware.AppContext())
	api := router.Group("/api/v1")
	doctorHandler.RegisterRoutes(api)
	adminHandler.RegisterRoutes(api)

	return router
}

func doctorWithStatus(status models.VerificationStatus) *models.Doctor {
	return &models.Doctor{
		ID:                  5,
		UserID:              11,
		FullName:            "Dr. Asha Verma",
		RegistrationNumber:  "DMC/R/12345",
		RegistrationCouncil: "Delhi Medical Council",
		Qualification:       "MBBS",
		SlotDurationMin:     15,
		VerificationStatus:  status,
		IsAcceptingPatients: true,
	}
}

func TestDoctorAndAdminHandlerStatusCodes(t *testing.T) {
	tests := []struct {
		name       string
		method     string
		path       string
		body       string
		token      string
		doctor     *models.Doctor
		createErr  error
		wantStatus int
		wantBody   string
	}{
		{name: "create without token", method: http.MethodPost, path: "/api/v1/doctors", body: doctorPayload, wantStatus: http.StatusUnauthorized},
		{name: "patient cannot create a doctor profile", method: http.MethodPost, path: "/api/v1/doctors", body: doctorPayload, token: "patient-token", wantStatus: http.StatusForbidden},
		{name: "admin cannot create a doctor profile", method: http.MethodPost, path: "/api/v1/doctors", body: doctorPayload, token: "admin-token", wantStatus: http.StatusForbidden},
		{name: "doctor creates a pending profile", method: http.MethodPost, path: "/api/v1/doctors", body: doctorPayload, token: "doctor-token", wantStatus: http.StatusCreated, wantBody: `"verificationStatus":"pending"`},
		{name: "create with invalid payload", method: http.MethodPost, path: "/api/v1/doctors", body: `{"fullName":""}`, token: "doctor-token", wantStatus: http.StatusBadRequest},
		{name: "create with unknown speciality", method: http.MethodPost, path: "/api/v1/doctors", body: strings.Replace(doctorPayload, "general-medicine", "astrology", 1), token: "doctor-token", wantStatus: http.StatusUnprocessableEntity},
		{name: "create twice conflicts", method: http.MethodPost, path: "/api/v1/doctors", body: doctorPayload, token: "doctor-token", doctor: doctorWithStatus(models.VerificationStatusPending), wantStatus: http.StatusConflict},
		{name: "duplicate registration conflicts", method: http.MethodPost, path: "/api/v1/doctors", body: doctorPayload, token: "doctor-token", createErr: apperrors.ErrDoctorRegistrationExists, wantStatus: http.StatusConflict},
		{name: "doctor reads own profile", method: http.MethodGet, path: "/api/v1/doctors/me", token: "doctor-token", doctor: doctorWithStatus(models.VerificationStatusPending), wantStatus: http.StatusOK},
		{name: "doctor without profile", method: http.MethodGet, path: "/api/v1/doctors/me", token: "doctor-token", wantStatus: http.StatusNotFound},
		{name: "patient sees a verified doctor", method: http.MethodGet, path: "/api/v1/doctors/5", token: "patient-token", doctor: doctorWithStatus(models.VerificationStatusVerified), wantStatus: http.StatusOK},
		{name: "patient cannot see a pending doctor", method: http.MethodGet, path: "/api/v1/doctors/5", token: "patient-token", doctor: doctorWithStatus(models.VerificationStatusPending), wantStatus: http.StatusNotFound},
		{name: "search returns verified doctors", method: http.MethodGet, path: "/api/v1/doctors?speciality=general-medicine&city=new+delhi", token: "patient-token", doctor: doctorWithStatus(models.VerificationStatusVerified), wantStatus: http.StatusOK, wantBody: `"id":5`},
		{name: "search by date is not supported yet", method: http.MethodGet, path: "/api/v1/doctors?date=2026-10-20", token: "patient-token", wantStatus: http.StatusBadRequest},
		{name: "search with invalid mode", method: http.MethodGet, path: "/api/v1/doctors?mode=teleport", token: "patient-token", wantStatus: http.StatusBadRequest},
		{name: "list specialities", method: http.MethodGet, path: "/api/v1/specialities", token: "patient-token", wantStatus: http.StatusOK, wantBody: "general-medicine"},
		{name: "admin routes need a token", method: http.MethodGet, path: "/api/v1/admin/doctors", wantStatus: http.StatusUnauthorized},
		{name: "patient cannot use admin routes", method: http.MethodGet, path: "/api/v1/admin/doctors", token: "patient-token", wantStatus: http.StatusForbidden},
		{name: "doctor cannot use admin routes", method: http.MethodGet, path: "/api/v1/admin/doctors", token: "doctor-token", wantStatus: http.StatusForbidden},
		{name: "admin lists the pending queue", method: http.MethodGet, path: "/api/v1/admin/doctors?status=pending", token: "admin-token", doctor: doctorWithStatus(models.VerificationStatusPending), wantStatus: http.StatusOK, wantBody: `"userId":11`},
		{name: "admin list with unknown status", method: http.MethodGet, path: "/api/v1/admin/doctors?status=approved", token: "admin-token", wantStatus: http.StatusBadRequest},
		{name: "admin gets a pending doctor", method: http.MethodGet, path: "/api/v1/admin/doctors/5", token: "admin-token", doctor: doctorWithStatus(models.VerificationStatusPending), wantStatus: http.StatusOK},
		{name: "admin verifies a pending doctor", method: http.MethodPost, path: "/api/v1/admin/doctors/5/verify", token: "admin-token", doctor: doctorWithStatus(models.VerificationStatusPending), wantStatus: http.StatusOK, wantBody: `"verificationStatus":"verified"`},
		{name: "verify an already verified doctor", method: http.MethodPost, path: "/api/v1/admin/doctors/5/verify", token: "admin-token", doctor: doctorWithStatus(models.VerificationStatusVerified), wantStatus: http.StatusConflict},
		{name: "verify a missing doctor", method: http.MethodPost, path: "/api/v1/admin/doctors/9/verify", token: "admin-token", wantStatus: http.StatusNotFound},
		{name: "reject without a reason", method: http.MethodPost, path: "/api/v1/admin/doctors/5/reject", body: `{}`, token: "admin-token", doctor: doctorWithStatus(models.VerificationStatusPending), wantStatus: http.StatusBadRequest},
		{name: "reject with a reason", method: http.MethodPost, path: "/api/v1/admin/doctors/5/reject", body: `{"reason":"number not on register"}`, token: "admin-token", doctor: doctorWithStatus(models.VerificationStatusPending), wantStatus: http.StatusOK, wantBody: `"verificationStatus":"rejected"`},
		{name: "suspend a verified doctor", method: http.MethodPost, path: "/api/v1/admin/doctors/5/suspend", body: `{"reason":"complaint"}`, token: "admin-token", doctor: doctorWithStatus(models.VerificationStatusVerified), wantStatus: http.StatusOK},
		{name: "reinstate a suspended doctor", method: http.MethodPost, path: "/api/v1/admin/doctors/5/reinstate", token: "admin-token", doctor: doctorWithStatus(models.VerificationStatusSuspended), wantStatus: http.StatusOK},
		{name: "review with invalid id", method: http.MethodPost, path: "/api/v1/admin/doctors/abc/verify", token: "admin-token", wantStatus: http.StatusBadRequest},
		{name: "admin creates another admin", method: http.MethodPost, path: "/api/v1/admin/users", body: `{"name":"Ops","email":"ops@example.com","password":"a-long-password","mobileNo":"999"}`, token: "admin-token", wantStatus: http.StatusCreated, wantBody: `"role":"admin"`},
		{name: "admin password too short", method: http.MethodPost, path: "/api/v1/admin/users", body: `{"name":"Ops","email":"ops@example.com","password":"short","mobileNo":"999"}`, token: "admin-token", wantStatus: http.StatusBadRequest},
		{name: "admin adds a speciality", method: http.MethodPost, path: "/api/v1/admin/specialities", body: `{"slug":"nephrology","name":"Nephrology"}`, token: "admin-token", wantStatus: http.StatusCreated},
		{name: "speciality slug with spaces", method: http.MethodPost, path: "/api/v1/admin/specialities", body: `{"slug":"kidney care","name":"Kidney"}`, token: "admin-token", wantStatus: http.StatusBadRequest},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			router := newDoctorAdminRouter(t, &fakeDoctorRepo{doctor: tt.doctor, createErr: tt.createErr})

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

func TestAvailabilityHandlerStatusCodes(t *testing.T) {
	nextWeek := time.Now().UTC().AddDate(0, 0, 7)
	date := nextWeek.Format(time.DateOnly)
	weekday := strconv.Itoa(int(nextWeek.Weekday()))
	weekPayload := `{"rules":[{"dayOfWeek":` + weekday + `,"startTime":"09:00","endTime":"10:00","mode":"in_person"}]}`

	verified := doctorWithStatus(models.VerificationStatusVerified)
	verified.ConsultationModes = []models.ConsultationMode{models.ConsultationModeInPerson}

	tests := []struct {
		name       string
		method     string
		path       string
		body       string
		token      string
		doctor     *models.Doctor
		setWeek    bool
		wantStatus int
		wantBody   string
	}{
		{name: "patient cannot set availability", method: http.MethodPut, path: "/api/v1/doctors/me/availability", body: weekPayload, token: "patient-token", doctor: verified, wantStatus: http.StatusForbidden},
		{name: "doctor sets the week", method: http.MethodPut, path: "/api/v1/doctors/me/availability", body: weekPayload, token: "doctor-token", doctor: verified, wantStatus: http.StatusOK, wantBody: `"startTime":"09:00"`},
		{name: "invalid time format", method: http.MethodPut, path: "/api/v1/doctors/me/availability", body: strings.Replace(weekPayload, "09:00", "9am", 1), token: "doctor-token", doctor: verified, wantStatus: http.StatusBadRequest},
		{name: "overlapping blocks", method: http.MethodPut, path: "/api/v1/doctors/me/availability", body: `{"rules":[{"dayOfWeek":1,"startTime":"09:00","endTime":"12:00","mode":"in_person"},{"dayOfWeek":1,"startTime":"11:00","endTime":"13:00","mode":"in_person"}]}`, token: "doctor-token", doctor: verified, wantStatus: http.StatusConflict},
		{name: "availability without a profile", method: http.MethodGet, path: "/api/v1/doctors/me/availability", token: "doctor-token", wantStatus: http.StatusNotFound},
		{name: "doctor reads availability", method: http.MethodGet, path: "/api/v1/doctors/me/availability", token: "doctor-token", doctor: verified, setWeek: true, wantStatus: http.StatusOK, wantBody: `"timezone":"UTC"`},
		{name: "doctor adds a day off", method: http.MethodPost, path: "/api/v1/doctors/me/overrides", body: `{"date":"` + date + `","reason":"Conference"}`, token: "doctor-token", doctor: verified, wantStatus: http.StatusCreated},
		{name: "override in the past", method: http.MethodPost, path: "/api/v1/doctors/me/overrides", body: `{"date":"2000-01-01"}`, token: "doctor-token", doctor: verified, wantStatus: http.StatusBadRequest},
		{name: "override with a bad date", method: http.MethodPost, path: "/api/v1/doctors/me/overrides", body: `{"date":"26/10/2026"}`, token: "doctor-token", doctor: verified, wantStatus: http.StatusBadRequest},
		{name: "slots for a working day", method: http.MethodGet, path: "/api/v1/doctors/5/slots?date=" + date, token: "patient-token", doctor: verified, setWeek: true, wantStatus: http.StatusOK, wantBody: `"startTime":"09:45"`},
		{name: "slots need a date", method: http.MethodGet, path: "/api/v1/doctors/5/slots", token: "patient-token", doctor: verified, wantStatus: http.StatusBadRequest},
		{name: "slots for a past date", method: http.MethodGet, path: "/api/v1/doctors/5/slots?date=2000-01-01", token: "patient-token", doctor: verified, wantStatus: http.StatusBadRequest},
		{name: "slots of a pending doctor", method: http.MethodGet, path: "/api/v1/doctors/5/slots?date=" + date, token: "patient-token", doctor: doctorWithStatus(models.VerificationStatusPending), wantStatus: http.StatusNotFound},
		{name: "slots without a token", method: http.MethodGet, path: "/api/v1/doctors/5/slots?date=" + date, doctor: verified, wantStatus: http.StatusUnauthorized},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			availability := &fakeAvailabilityRepo{}
			if tt.setWeek {
				availability.rules = []*models.AvailabilityRule{{DayOfWeek: nextWeek.Weekday(), StartMinute: 9 * 60, EndMinute: 10 * 60, Mode: models.ConsultationModeInPerson}}
			}
			var doctor *models.Doctor
			if tt.doctor != nil {
				copied := *tt.doctor
				doctor = &copied
			}
			router := newDoctorAdminRouterWithAvailability(t, &fakeDoctorRepo{doctor: doctor}, availability)

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

func TestNewDoctorAndAdminHandlerValidation(t *testing.T) {
	if _, err := handler.NewDoctorHandler(nil, &fakeRoleTokenVerifier{}); !errors.Is(err, apperrors.ErrNilDoctorController) {
		t.Fatalf("nil doctor controller error = %v, want %v", err, apperrors.ErrNilDoctorController)
	}
	if _, err := handler.NewAdminHandler(nil, &fakeRoleTokenVerifier{}); !errors.Is(err, apperrors.ErrNilAdminController) {
		t.Fatalf("nil admin controller error = %v, want %v", err, apperrors.ErrNilAdminController)
	}
}
