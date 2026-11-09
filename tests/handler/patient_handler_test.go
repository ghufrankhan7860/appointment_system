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

	return newPatientSharingRouter(t, patients, userPatients, &fakePatientInviteRepo{}, &fakeInviteAttemptRepo{})
}

func newPatientSharingRouter(
	t *testing.T,
	patients *fakePatientRepo,
	userPatients *fakeUserPatientRepo,
	invites *fakePatientInviteRepo,
	attempts *fakeInviteAttemptRepo,
) *gin.Engine {
	t.Helper()
	gin.SetMode(gin.TestMode)

	patientController := newTestPatientController(t, patients, userPatients, invites, attempts)
	patientHandler, err := handler.NewPatientHandler(patientController, &fakeTokenVerifier{})
	if err != nil {
		t.Fatalf("NewPatientHandler returned error: %v", err)
	}

	router := gin.New()
	router.Use(middleware.AppContext())
	patientHandler.RegisterRoutes(router.Group("/api/v1"))

	return router
}

func newTestPatientController(
	t *testing.T,
	patients *fakePatientRepo,
	userPatients *fakeUserPatientRepo,
	invites *fakePatientInviteRepo,
	attempts *fakeInviteAttemptRepo,
) *controller.PatientController {
	t.Helper()

	patientService, err := services.NewPatientService(patients, userPatients, &fakeUserPatientHistoryRepo{}, &fakeTxManager{}, discardLogger)
	if err != nil {
		t.Fatalf("NewPatientService returned error: %v", err)
	}
	inviteService, err := services.NewPatientInviteService(invites, attempts, userPatients, &fakeUserPatientHistoryRepo{}, patients, &fakeOutboxRepo{}, &fakeTxManager{}, time.Now, discardLogger)
	if err != nil {
		t.Fatalf("NewPatientInviteService returned error: %v", err)
	}
	membershipService, err := services.NewPatientMembershipService(userPatients, &fakeUserRepo{}, &fakeUserPatientHistoryRepo{}, &fakeOutboxRepo{}, &fakeTxManager{}, discardLogger)
	if err != nil {
		t.Fatalf("NewPatientMembershipService returned error: %v", err)
	}
	patientController, err := controller.NewPatientController(patientService, inviteService, membershipService)
	if err != nil {
		t.Fatalf("NewPatientController returned error: %v", err)
	}

	return patientController
}

func TestNewPatientHandlerValidation(t *testing.T) {
	if _, err := handler.NewPatientHandler(nil, &fakeTokenVerifier{}); !errors.Is(err, apperrors.ErrNilPatientController) {
		t.Fatalf("nil controller error = %v, want %v", err, apperrors.ErrNilPatientController)
	}

	patientService, err := services.NewPatientService(&fakePatientRepo{}, &fakeUserPatientRepo{}, &fakeUserPatientHistoryRepo{}, &fakeTxManager{}, discardLogger)
	if err != nil {
		t.Fatalf("NewPatientService returned error: %v", err)
	}
	membershipService, err := services.NewPatientMembershipService(&fakeUserPatientRepo{}, &fakeUserRepo{}, &fakeUserPatientHistoryRepo{}, &fakeOutboxRepo{}, &fakeTxManager{}, discardLogger)
	if err != nil {
		t.Fatalf("NewPatientMembershipService returned error: %v", err)
	}
	if _, err := controller.NewPatientController(patientService, nil, membershipService); !errors.Is(err, apperrors.ErrNilPatientInviteService) {
		t.Fatalf("nil invite service error = %v, want %v", err, apperrors.ErrNilPatientInviteService)
	}

	patientController := newTestPatientController(t, &fakePatientRepo{}, &fakeUserPatientRepo{}, &fakePatientInviteRepo{}, &fakeInviteAttemptRepo{})
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

func TestPatientSharingHandlerStatusCodes(t *testing.T) {
	savedPatient := &models.Patient{ID: 3, Name: "Asha", Gender: models.GenderFemale, DateOfBirth: time.Date(1990, 1, 2, 0, 0, 0, 0, time.UTC), BloodType: models.BloodTypeOPlus}
	full := func(userID uint64, relationship models.Relationship) *models.UserPatient {
		return &models.UserPatient{UserID: userID, PatientID: 3, PatientRelationship: relationship, AccessLevel: models.AccessLevelFull, Status: models.UserPatientStatusActive}
	}
	readOnly := &models.UserPatient{UserID: 7, PatientID: 3, PatientRelationship: models.RelationshipCaregiver, AccessLevel: models.AccessLevelReadOnly, Status: models.UserPatientStatusActive}
	readOnlyOther := &models.UserPatient{UserID: 8, PatientID: 3, PatientRelationship: models.RelationshipCaregiver, AccessLevel: models.AccessLevelReadOnly, Status: models.UserPatientStatusActive}
	pendingInvite := &models.PatientInvite{ID: 31, PatientID: 3, PatientRelationship: models.RelationshipFather, AccessLevel: models.AccessLevelFull, Status: models.PatientInviteStatusPending, ExpiresAt: time.Now().Add(time.Hour)}

	tests := []struct {
		name         string
		method       string
		path         string
		body         string
		userPatients *fakeUserPatientRepo
		invites      *fakePatientInviteRepo
		attempts     *fakeInviteAttemptRepo
		wantStatus   int
		wantBody     string
	}{
		{name: "get shows the caller's access", method: http.MethodGet, path: "/api/v1/patients/3", userPatients: &fakeUserPatientRepo{userPatient: full(7, models.RelationshipMother)}, wantStatus: http.StatusOK, wantBody: `"relationship":"mother","accessLevel":"full"`},
		{name: "create invite returns the code", method: http.MethodPost, path: "/api/v1/patients/3/invites", body: `{"relationship":"father","accessLevel":"full"}`, userPatients: &fakeUserPatientRepo{patientLinks: []*models.UserPatient{full(7, models.RelationshipMother)}}, wantStatus: http.StatusCreated, wantBody: `"code":"`},
		{name: "create invite with an unknown relationship", method: http.MethodPost, path: "/api/v1/patients/3/invites", body: `{"relationship":"cousin","accessLevel":"full"}`, userPatients: &fakeUserPatientRepo{patientLinks: []*models.UserPatient{full(7, models.RelationshipMother)}}, wantStatus: http.StatusBadRequest, wantBody: `"details"`},
		{name: "create invite with a long lifetime", method: http.MethodPost, path: "/api/v1/patients/3/invites", body: `{"relationship":"father","accessLevel":"full","expiresInHours":500}`, userPatients: &fakeUserPatientRepo{}, wantStatus: http.StatusBadRequest},
		{name: "create invite without full access", method: http.MethodPost, path: "/api/v1/patients/3/invites", body: `{"relationship":"father","accessLevel":"full"}`, userPatients: &fakeUserPatientRepo{patientLinks: []*models.UserPatient{readOnly}}, wantStatus: http.StatusForbidden},
		{name: "create invite for someone else's patient", method: http.MethodPost, path: "/api/v1/patients/3/invites", body: `{"relationship":"father","accessLevel":"full"}`, userPatients: &fakeUserPatientRepo{patientLinks: []*models.UserPatient{full(8, models.RelationshipMother)}}, wantStatus: http.StatusNotFound},
		{name: "create self invite while a self link exists", method: http.MethodPost, path: "/api/v1/patients/3/invites", body: `{"relationship":"self","accessLevel":"full"}`, userPatients: &fakeUserPatientRepo{patientLinks: []*models.UserPatient{full(7, models.RelationshipMother), full(8, models.RelationshipSelf)}}, wantStatus: http.StatusConflict},
		{name: "create invite over the pending limit", method: http.MethodPost, path: "/api/v1/patients/3/invites", body: `{"relationship":"father","accessLevel":"full"}`, userPatients: &fakeUserPatientRepo{patientLinks: []*models.UserPatient{full(7, models.RelationshipMother)}}, invites: &fakePatientInviteRepo{pending: 5}, wantStatus: http.StatusConflict},
		{name: "list invites", method: http.MethodGet, path: "/api/v1/patients/3/invites", userPatients: &fakeUserPatientRepo{userPatient: full(7, models.RelationshipMother)}, wantStatus: http.StatusOK, wantBody: `"invites":[]`},
		{name: "revoke an invite that is not pending", method: http.MethodDelete, path: "/api/v1/patients/3/invites/31", userPatients: &fakeUserPatientRepo{userPatient: full(7, models.RelationshipMother)}, invites: &fakePatientInviteRepo{getErr: apperrors.ErrPatientInviteNotFound}, wantStatus: http.StatusNotFound},
		{name: "claim links the caller", method: http.MethodPost, path: "/api/v1/patients/claim", body: `{"code":"k7q2-m9xd"}`, userPatients: &fakeUserPatientRepo{getErr: apperrors.ErrUserPatientNotFound}, invites: &fakePatientInviteRepo{invite: pendingInvite}, wantStatus: http.StatusOK, wantBody: `"relationship":"father","accessLevel":"full"`},
		{name: "claim without a code", method: http.MethodPost, path: "/api/v1/patients/claim", body: `{}`, userPatients: &fakeUserPatientRepo{}, wantStatus: http.StatusBadRequest},
		{name: "claim an unknown code", method: http.MethodPost, path: "/api/v1/patients/claim", body: `{"code":"K7Q2M9XD"}`, userPatients: &fakeUserPatientRepo{}, invites: &fakePatientInviteRepo{getErr: apperrors.ErrPatientInviteNotFound}, wantStatus: http.StatusNotFound, wantBody: `"Invite code is invalid or expired"`},
		{name: "claim a patient already linked", method: http.MethodPost, path: "/api/v1/patients/claim", body: `{"code":"K7Q2M9XD"}`, userPatients: &fakeUserPatientRepo{userPatient: full(7, models.RelationshipMother)}, invites: &fakePatientInviteRepo{invite: pendingInvite}, wantStatus: http.StatusConflict},
		{name: "claim after too many wrong codes", method: http.MethodPost, path: "/api/v1/patients/claim", body: `{"code":"K7Q2M9XD"}`, userPatients: &fakeUserPatientRepo{}, attempts: &fakeInviteAttemptRepo{count: 5}, wantStatus: http.StatusTooManyRequests},
		{name: "list members masks mobiles", method: http.MethodGet, path: "/api/v1/patients/3/members", userPatients: &fakeUserPatientRepo{patientLinks: []*models.UserPatient{full(7, models.RelationshipMother), readOnlyOther}}, wantStatus: http.StatusOK, wantBody: `"maskedMobile":"98******10"`},
		{name: "list members without full access", method: http.MethodGet, path: "/api/v1/patients/3/members", userPatients: &fakeUserPatientRepo{patientLinks: []*models.UserPatient{readOnly}}, wantStatus: http.StatusForbidden},
		{name: "update a member's access", method: http.MethodPatch, path: "/api/v1/patients/3/members/8", body: `{"accessLevel":"read_only"}`, userPatients: &fakeUserPatientRepo{patientLinks: []*models.UserPatient{full(7, models.RelationshipMother), full(8, models.RelationshipFather)}}, wantStatus: http.StatusOK},
		{name: "update the self link", method: http.MethodPatch, path: "/api/v1/patients/3/members/8", body: `{"accessLevel":"read_only"}`, userPatients: &fakeUserPatientRepo{patientLinks: []*models.UserPatient{full(7, models.RelationshipMother), full(8, models.RelationshipSelf)}}, wantStatus: http.StatusForbidden},
		{name: "update an unknown member", method: http.MethodPatch, path: "/api/v1/patients/3/members/9", body: `{"accessLevel":"read_only"}`, userPatients: &fakeUserPatientRepo{patientLinks: []*models.UserPatient{full(7, models.RelationshipMother)}}, wantStatus: http.StatusNotFound},
		{name: "revoke your own access", method: http.MethodDelete, path: "/api/v1/patients/3/members/7", userPatients: &fakeUserPatientRepo{patientLinks: []*models.UserPatient{full(7, models.RelationshipMother)}}, wantStatus: http.StatusBadRequest},
		{name: "revoke a member", method: http.MethodDelete, path: "/api/v1/patients/3/members/8", userPatients: &fakeUserPatientRepo{patientLinks: []*models.UserPatient{full(7, models.RelationshipMother), readOnlyOther}}, wantStatus: http.StatusOK},
		{name: "last full holder cannot leave", method: http.MethodDelete, path: "/api/v1/patients/3/membership", userPatients: &fakeUserPatientRepo{patientLinks: []*models.UserPatient{full(7, models.RelationshipMother), readOnlyOther}}, wantStatus: http.StatusConflict},
		{name: "leave a patient", method: http.MethodDelete, path: "/api/v1/patients/3/membership", userPatients: &fakeUserPatientRepo{patientLinks: []*models.UserPatient{full(7, models.RelationshipMother), full(8, models.RelationshipFather)}}, wantStatus: http.StatusOK},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			invites := tt.invites
			if invites == nil {
				invites = &fakePatientInviteRepo{}
			}
			attempts := tt.attempts
			if attempts == nil {
				attempts = &fakeInviteAttemptRepo{}
			}
			router := newPatientSharingRouter(t, &fakePatientRepo{patient: savedPatient}, tt.userPatients, invites, attempts)

			request := httptest.NewRequest(tt.method, tt.path, strings.NewReader(tt.body))
			request.Header.Set("Content-Type", "application/json")
			request.Header.Set("Authorization", "Bearer good-token")
			recorder := httptest.NewRecorder()

			router.ServeHTTP(recorder, request)

			if recorder.Code != tt.wantStatus {
				t.Fatalf("status = %d, want %d, body = %s", recorder.Code, tt.wantStatus, recorder.Body.String())
			}
			if !strings.Contains(recorder.Body.String(), tt.wantBody) {
				t.Fatalf("body = %s, want it to contain %s", recorder.Body.String(), tt.wantBody)
			}
		})
	}
}
