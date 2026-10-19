package handler

import (
	"appointment_system/internal/apps/appointment/apperrors"
	"appointment_system/internal/apps/appointment/controller"
	"appointment_system/internal/apps/appointment/models"
	"appointment_system/internal/apps/appointment/services"
	"appointment_system/middleware"
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
)

type fakePatientRepo struct {
	patient *models.Patient
	err     error
}

func (f *fakePatientRepo) Create(ctx context.Context, patient *models.Patient) (*models.Patient, error) {
	return f.patient, f.err
}

func (f *fakePatientRepo) GetByID(ctx context.Context, id uint64) (*models.Patient, error) {
	return f.patient, f.err
}

func (f *fakePatientRepo) GetByIDs(ctx context.Context, ids []uint64) (map[uint64]*models.Patient, error) {
	if f.err != nil {
		return nil, f.err
	}
	return map[uint64]*models.Patient{f.patient.ID: f.patient}, nil
}

func (f *fakePatientRepo) Update(ctx context.Context, patient *models.Patient) (*models.Patient, error) {
	return f.patient, f.err
}

type fakeUserPatientRepo struct {
	userPatient *models.UserPatient
	createErr   error
	getErr      error
}

func (f *fakeUserPatientRepo) Create(ctx context.Context, userPatient *models.UserPatient) (*models.UserPatient, error) {
	return userPatient, f.createErr
}

func (f *fakeUserPatientRepo) GetByUserAndPatient(ctx context.Context, userID uint64, patientID uint64) (*models.UserPatient, error) {
	return f.userPatient, f.getErr
}

func (f *fakeUserPatientRepo) GetActiveByUserAndPatient(ctx context.Context, userID uint64, patientID uint64) (*models.UserPatient, error) {
	return f.userPatient, f.getErr
}

func (f *fakeUserPatientRepo) ListByPatient(ctx context.Context, patientID uint64, limit int, offset int) ([]*models.UserPatient, error) {
	return []*models.UserPatient{f.userPatient}, f.getErr
}

func (f *fakeUserPatientRepo) ListByUser(ctx context.Context, userID uint64, limit int, offset int) ([]*models.UserPatient, error) {
	return []*models.UserPatient{f.userPatient}, f.getErr
}

func (f *fakeUserPatientRepo) ListActiveByUser(ctx context.Context, userID uint64, limit int, offset int) ([]*models.UserPatient, error) {
	return []*models.UserPatient{f.userPatient}, f.getErr
}

func (f *fakeUserPatientRepo) Update(ctx context.Context, userPatient *models.UserPatient) (*models.UserPatient, error) {
	return userPatient, nil
}

func (f *fakeUserPatientRepo) Revoke(ctx context.Context, userID uint64, patientID uint64) (*models.UserPatient, error) {
	return f.userPatient, nil
}

type fakeTxManager struct{}

func (f *fakeTxManager) WithinTransaction(ctx context.Context, fn func(context.Context) error) error {
	return fn(ctx)
}

type fakeTokenVerifier struct{}

func (f *fakeTokenVerifier) Verify(token string) (uint64, error) {
	if token != "good-token" {
		return 0, errors.New("invalid token")
	}
	return 7, nil
}

const patientPayload = `{"name":"Asha","gender":"Female","dateOfBirth":"1990-01-02","bloodType":"O+","emergencyContact":"1234567890","relationship":"self"}`

func newPatientRouter(t *testing.T, patients *fakePatientRepo, userPatients *fakeUserPatientRepo) *gin.Engine {
	t.Helper()
	gin.SetMode(gin.TestMode)

	service, err := services.NewPatientService(patients, userPatients, &fakeTxManager{})
	if err != nil {
		t.Fatalf("NewPatientService returned error: %v", err)
	}
	patientController, err := controller.NewPatientController(service)
	if err != nil {
		t.Fatalf("NewPatientController returned error: %v", err)
	}
	patientHandler, err := NewPatientHandler(patientController, &fakeTokenVerifier{})
	if err != nil {
		t.Fatalf("NewPatientHandler returned error: %v", err)
	}

	router := gin.New()
	router.Use(middleware.AppContext())
	patientHandler.RegisterRoutes(router.Group("/api/v1"))

	return router
}

func TestNewPatientHandlerValidation(t *testing.T) {
	if _, err := NewPatientHandler(nil, &fakeTokenVerifier{}); !errors.Is(err, apperrors.ErrNilPatientController) {
		t.Fatalf("nil controller error = %v, want %v", err, apperrors.ErrNilPatientController)
	}

	service, err := services.NewPatientService(&fakePatientRepo{}, &fakeUserPatientRepo{}, &fakeTxManager{})
	if err != nil {
		t.Fatalf("NewPatientService returned error: %v", err)
	}
	patientController, err := controller.NewPatientController(service)
	if err != nil {
		t.Fatalf("NewPatientController returned error: %v", err)
	}
	if _, err := NewPatientHandler(patientController, nil); !errors.Is(err, apperrors.ErrNilTokenVerifier) {
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
