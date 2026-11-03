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
	"strings"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
)

const patientPayload = `{"name":"Asha","gender":"Female","dateOfBirth":"1990-01-02","bloodType":"O+","emergencyContact":"1234567890","relationship":"self"}`

func newPatientRouter(t *testing.T, patients *fakePatientRepo, userPatients *fakeUserPatientRepo) *gin.Engine {
	t.Helper()
	gin.SetMode(gin.TestMode)

	service, err := services.NewPatientService(patients, userPatients, &fakeTxManager{}, discardLogger)
	if err != nil {
		t.Fatalf("NewPatientService returned error: %v", err)
	}
	patientController, err := controller.NewPatientController(service)
	if err != nil {
		t.Fatalf("NewPatientController returned error: %v", err)
	}
	patientHandler, err := handler.NewPatientHandler(patientController, &fakeTokenVerifier{})
	if err != nil {
		t.Fatalf("NewPatientHandler returned error: %v", err)
	}

	router := gin.New()
	router.Use(middleware.AppContext())
	patientHandler.RegisterRoutes(router.Group("/api/v1"))

	return router
}

func TestNewPatientHandlerValidation(t *testing.T) {
	if _, err := handler.NewPatientHandler(nil, &fakeTokenVerifier{}); !errors.Is(err, apperrors.ErrNilPatientController) {
		t.Fatalf("nil controller error = %v, want %v", err, apperrors.ErrNilPatientController)
	}

	service, err := services.NewPatientService(&fakePatientRepo{}, &fakeUserPatientRepo{}, &fakeTxManager{}, discardLogger)
	if err != nil {
		t.Fatalf("NewPatientService returned error: %v", err)
	}
	patientController, err := controller.NewPatientController(service)
	if err != nil {
		t.Fatalf("NewPatientController returned error: %v", err)
	}
	if _, err := handler.NewPatientHandler(patientController, nil); !errors.Is(err, apperrors.ErrNilTokenVerifier) {
		t.Fatalf("nil verifier error = %v, want %v", err, apperrors.ErrNilTokenVerifier)
	}
}

func TestPatientHandlerStatusCodes(t *testing.T) {
	savedPatient := &models.Patient{
		ID:          3,
		Name:        "Asha",
		Gender:      models.GenderFemale,
		DateOfBirth: time.Date(1990, 1, 2, 0, 0, 0, 0, time.UTC),
		BloodType:   models.BloodTypeOPlus,
	}
	fullAccess := &models.UserPatient{
		UserID:      7,
		PatientID:   3,
		AccessLevel: models.AccessLevelFull,
		Status:      models.UserPatientStatusActive,
	}
	readOnly := &models.UserPatient{
		UserID:      7,
		PatientID:   3,
		AccessLevel: models.AccessLevelReadOnly,
		Status:      models.UserPatientStatusActive,
	}

	tests := []struct {
		name         string
		method       string
		path         string
		body         string
		token        string
		patients     *fakePatientRepo
		userPatients *fakeUserPatientRepo
		wantStatus   int
	}{
		{
			name:         "missing token is unauthorized",
			method:       http.MethodGet,
			path:         "/api/v1/patients",
			patients:     &fakePatientRepo{},
			userPatients: &fakeUserPatientRepo{},
			wantStatus:   http.StatusUnauthorized,
		},
		{
			name:         "create with invalid payload",
			method:       http.MethodPost,
			path:         "/api/v1/patients",
			body:         `{"name":""}`,
			token:        "good-token",
			patients:     &fakePatientRepo{},
			userPatients: &fakeUserPatientRepo{},
			wantStatus:   http.StatusBadRequest,
		},
		{
			name:         "create with malformed date of birth",
			method:       http.MethodPost,
			path:         "/api/v1/patients",
			body:         strings.Replace(patientPayload, "1990-01-02", "02/01/1990", 1),
			token:        "good-token",
			patients:     &fakePatientRepo{},
			userPatients: &fakeUserPatientRepo{},
			wantStatus:   http.StatusBadRequest,
		},
		{
			name:         "create succeeds",
			method:       http.MethodPost,
			path:         "/api/v1/patients",
			body:         patientPayload,
			token:        "good-token",
			patients:     &fakePatientRepo{patient: savedPatient},
			userPatients: &fakeUserPatientRepo{},
			wantStatus:   http.StatusCreated,
		},
		{
			name:         "create conflicts with existing link",
			method:       http.MethodPost,
			path:         "/api/v1/patients",
			body:         patientPayload,
			token:        "good-token",
			patients:     &fakePatientRepo{patient: savedPatient},
			userPatients: &fakeUserPatientRepo{createErr: apperrors.ErrUserPatientAlreadyExists},
			wantStatus:   http.StatusConflict,
		},
		{
			name:         "get with invalid id",
			method:       http.MethodGet,
			path:         "/api/v1/patients/abc",
			token:        "good-token",
			patients:     &fakePatientRepo{},
			userPatients: &fakeUserPatientRepo{},
			wantStatus:   http.StatusBadRequest,
		},
		{
			name:         "get without access hides the patient",
			method:       http.MethodGet,
			path:         "/api/v1/patients/3",
			token:        "good-token",
			patients:     &fakePatientRepo{patient: savedPatient},
			userPatients: &fakeUserPatientRepo{getErr: apperrors.ErrUserPatientNotFound},
			wantStatus:   http.StatusNotFound,
		},
		{
			name:         "get succeeds",
			method:       http.MethodGet,
			path:         "/api/v1/patients/3",
			token:        "good-token",
			patients:     &fakePatientRepo{patient: savedPatient},
			userPatients: &fakeUserPatientRepo{userPatient: fullAccess},
			wantStatus:   http.StatusOK,
		},
		{
			name:         "list succeeds",
			method:       http.MethodGet,
			path:         "/api/v1/patients?limit=10&offset=0",
			token:        "good-token",
			patients:     &fakePatientRepo{patient: savedPatient},
			userPatients: &fakeUserPatientRepo{userPatient: fullAccess},
			wantStatus:   http.StatusOK,
		},
		{
			name:         "list with invalid limit",
			method:       http.MethodGet,
			path:         "/api/v1/patients?limit=abc",
			token:        "good-token",
			patients:     &fakePatientRepo{},
			userPatients: &fakeUserPatientRepo{},
			wantStatus:   http.StatusBadRequest,
		},
		{
			name:         "update without full access is forbidden",
			method:       http.MethodPut,
			path:         "/api/v1/patients/3",
			body:         patientPayload,
			token:        "good-token",
			patients:     &fakePatientRepo{patient: savedPatient},
			userPatients: &fakeUserPatientRepo{userPatient: readOnly},
			wantStatus:   http.StatusForbidden,
		},
		{
			name:         "update succeeds",
			method:       http.MethodPut,
			path:         "/api/v1/patients/3",
			body:         patientPayload,
			token:        "good-token",
			patients:     &fakePatientRepo{patient: savedPatient},
			userPatients: &fakeUserPatientRepo{userPatient: fullAccess},
			wantStatus:   http.StatusOK,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			router := newPatientRouter(t, tt.patients, tt.userPatients)

			request := httptest.NewRequest(tt.method, tt.path, strings.NewReader(tt.body))
			request.Header.Set("Content-Type", "application/json")
			if tt.token != "" {
				request.Header.Set("Authorization", "Bearer "+tt.token)
			}
			recorder := httptest.NewRecorder()

			router.ServeHTTP(recorder, request)

			if recorder.Code != tt.wantStatus {
				t.Fatalf("status = %d, want %d, body = %s", recorder.Code, tt.wantStatus, recorder.Body.String())
			}
		})
	}
}
