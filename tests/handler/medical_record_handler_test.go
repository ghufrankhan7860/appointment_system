package handler_test

import (
	"appointment_system/internal/apps/appointment/adapters/storage/local"
	"appointment_system/internal/apps/appointment/apperrors"
	"appointment_system/internal/apps/appointment/controller"
	"appointment_system/internal/apps/appointment/handler"
	"appointment_system/internal/apps/appointment/models"
	"appointment_system/internal/apps/appointment/services"
	"appointment_system/middleware"
	"bytes"
	"encoding/json"
	"fmt"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
)

var pdfBytes = []byte("%PDF-1.4\n1 0 obj << /Type /Catalog >> endobj\n")

func newMedicalRecordRouter(t *testing.T, records *fakeMedicalRecordRepo, appointments *fakeAppointmentRepo, userPatients *fakeUserPatientRepo) *gin.Engine {
	t.Helper()
	gin.SetMode(gin.TestMode)

	storage, err := local.New(t.TempDir())
	if err != nil {
		t.Fatalf("local.New returned error: %v", err)
	}
	doctor := doctorWithStatus(models.VerificationStatusVerified)
	medicalRecordService, err := services.NewMedicalRecordService(records, appointments, userPatients, &fakeDoctorRepo{doctor: doctor}, storage, &fakeOutboxRepo{}, &fakeTxManager{}, time.Now)
	if err != nil {
		t.Fatalf("NewMedicalRecordService returned error: %v", err)
	}
	medicalRecordController, err := controller.NewMedicalRecordController(medicalRecordService)
	if err != nil {
		t.Fatalf("NewMedicalRecordController returned error: %v", err)
	}
	medicalRecordHandler, err := handler.NewMedicalRecordHandler(medicalRecordController, &fakeRoleTokenVerifier{})
	if err != nil {
		t.Fatalf("NewMedicalRecordHandler returned error: %v", err)
	}

	router := gin.New()
	router.Use(middleware.AppContext())
	medicalRecordHandler.RegisterRoutes(router.Group("/api/v1"))

	return router
}

func uploadBody(t *testing.T, fields map[string]string, content []byte) (*bytes.Buffer, string) {
	t.Helper()

	body := &bytes.Buffer{}
	writer := multipart.NewWriter(body)
	for name, value := range fields {
		if err := writer.WriteField(name, value); err != nil {
			t.Fatalf("WriteField returned error: %v", err)
		}
	}
	if content != nil {
		part, err := writer.CreateFormFile("file", "report.bin")
		if err != nil {
			t.Fatalf("CreateFormFile returned error: %v", err)
		}
		part.Write(content)
	}
	if err := writer.Close(); err != nil {
		t.Fatalf("Close returned error: %v", err)
	}

	return body, writer.FormDataContentType()
}

func TestMedicalRecordHandlerStatusCodes(t *testing.T) {
	started := func() *models.AppointmentDetails {
		startsAt := time.Now().Add(-time.Hour)
		return &models.AppointmentDetails{Appointment: models.Appointment{ID: 50, PatientID: 3, DoctorID: 5, StartsAt: startsAt, EndsAt: startsAt.Add(15 * time.Minute), Status: models.AppointmentStatusScheduled, PaymentStatus: models.AppointmentPaymentPaid}}
	}
	recordWith := func(status models.MedicalRecordStatus, diagnosis string) *models.MedicalRecord {
		return &models.MedicalRecord{ID: 90, AppointmentID: 50, PatientID: 3, DoctorID: 5, Status: status, Diagnosis: diagnosis}
	}
	fullLink := &models.UserPatient{UserID: 7, PatientID: 3, AccessLevel: models.AccessLevelFull, Status: models.UserPatientStatusActive}
	readOnlyLink := &models.UserPatient{UserID: 7, PatientID: 3, AccessLevel: models.AccessLevelReadOnly, Status: models.UserPatientStatusActive}
	appointmentsOnlyLink := &models.UserPatient{UserID: 7, PatientID: 3, AccessLevel: models.AccessLevelAppointmentsOnly, Status: models.UserPatientStatusActive}
	draftPayload := `{"diagnosis":"Viral fever","vitals":{"pulsePerMinute":82},"medicines":[{"medicineName":"Paracetamol","dosage":"1-0-1","timing":"after_food"}]}`

	tests := []struct {
		name       string
		method     string
		path       string
		body       string
		token      string
		link       *models.UserPatient
		record     *models.MedicalRecord
		wantStatus int
		wantBody   string
	}{
		{name: "doctor opens the record", method: http.MethodPost, path: "/api/v1/doctors/me/appointments/50/record", token: "doctor-token", wantStatus: http.StatusCreated, wantBody: `"status":"draft"`},
		{name: "patient cannot open a record", method: http.MethodPost, path: "/api/v1/doctors/me/appointments/50/record", token: "patient-token", wantStatus: http.StatusForbidden},
		{name: "doctor edits the draft", method: http.MethodPut, path: "/api/v1/doctors/me/records/90", body: draftPayload, token: "doctor-token", record: recordWith(models.MedicalRecordStatusDraft, ""), wantStatus: http.StatusOK, wantBody: `"medicineName":"Paracetamol"`},
		{name: "a follow-up date in the wrong format", method: http.MethodPut, path: "/api/v1/doctors/me/records/90", body: `{"followUpOn":"02/11/2026"}`, token: "doctor-token", record: recordWith(models.MedicalRecordStatusDraft, ""), wantStatus: http.StatusBadRequest, wantBody: `"details":"followUpOn must be YYYY-MM-DD"`},
		{name: "an impossible pulse", method: http.MethodPut, path: "/api/v1/doctors/me/records/90", body: `{"vitals":{"pulsePerMinute":900}}`, token: "doctor-token", record: recordWith(models.MedicalRecordStatusDraft, ""), wantStatus: http.StatusBadRequest, wantBody: `vitals.pulsePerMinute must be between`},
		{name: "editing a finalized record", method: http.MethodPut, path: "/api/v1/doctors/me/records/90", body: draftPayload, token: "doctor-token", record: recordWith(models.MedicalRecordStatusFinalized, "x"), wantStatus: http.StatusConflict},
		{name: "finalizing without a diagnosis", method: http.MethodPost, path: "/api/v1/doctors/me/records/90/finalize", token: "doctor-token", record: recordWith(models.MedicalRecordStatusDraft, ""), wantStatus: http.StatusBadRequest, wantBody: `"details":"diagnosis is required to finalize"`},
		{name: "doctor finalizes", method: http.MethodPost, path: "/api/v1/doctors/me/records/90/finalize", token: "doctor-token", record: recordWith(models.MedicalRecordStatusDraft, "Viral fever"), wantStatus: http.StatusOK, wantBody: `"status":"finalized"`},
		{name: "full access reads a finalized record", method: http.MethodGet, path: "/api/v1/records/90", token: "patient-token", link: fullLink, record: recordWith(models.MedicalRecordStatusFinalized, "Viral fever"), wantStatus: http.StatusOK, wantBody: `"diagnosis":"Viral fever"`},
		{name: "full access cannot see a draft", method: http.MethodGet, path: "/api/v1/records/90", token: "patient-token", link: fullLink, record: recordWith(models.MedicalRecordStatusDraft, "Viral fever"), wantStatus: http.StatusNotFound},
		{name: "read_only access cannot read records", method: http.MethodGet, path: "/api/v1/records/90", token: "patient-token", link: readOnlyLink, record: recordWith(models.MedicalRecordStatusFinalized, "Viral fever"), wantStatus: http.StatusNotFound},
		{name: "the record's doctor reads the draft", method: http.MethodGet, path: "/api/v1/records/90", token: "doctor-token", record: recordWith(models.MedicalRecordStatusDraft, ""), wantStatus: http.StatusOK},
		{name: "full access lists records", method: http.MethodGet, path: "/api/v1/patients/3/records", token: "patient-token", link: fullLink, record: recordWith(models.MedicalRecordStatusFinalized, "Viral fever"), wantStatus: http.StatusOK, wantBody: `"records":[{`},
		{name: "appointments_only access cannot list records", method: http.MethodGet, path: "/api/v1/patients/3/records", token: "patient-token", link: appointmentsOnlyLink, wantStatus: http.StatusNotFound},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			userPatients := &fakeUserPatientRepo{userPatient: tt.link}
			if tt.link == nil {
				userPatients.getErr = apperrors.ErrUserPatientNotFound
			}
			router := newMedicalRecordRouter(t, &fakeMedicalRecordRepo{record: tt.record}, &fakeAppointmentRepo{byID: started()}, userPatients)

			request := httptest.NewRequest(tt.method, tt.path, strings.NewReader(tt.body))
			if tt.body != "" {
				request.Header.Set("Content-Type", "application/json")
			}
			request.Header.Set("Authorization", "Bearer "+tt.token)
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

func TestMedicalRecordHandlerAttachments(t *testing.T) {
	fields := map[string]string{"kind": "lab_report", "title": "Blood report", "takenOn": "2026-10-20"}
	fullLink := &models.UserPatient{UserID: 7, PatientID: 3, AccessLevel: models.AccessLevelFull, Status: models.UserPatientStatusActive}
	readOnlyLink := &models.UserPatient{UserID: 7, PatientID: 3, AccessLevel: models.AccessLevelReadOnly, Status: models.UserPatientStatusActive}

	tests := []struct {
		name       string
		path       string
		token      string
		link       *models.UserPatient
		record     *models.MedicalRecord
		fields     map[string]string
		content    []byte
		wantStatus int
		wantBody   string
	}{
		{name: "full access uploads a PDF", path: "/api/v1/patients/3/attachments", token: "patient-token", link: fullLink, fields: fields, content: pdfBytes, wantStatus: http.StatusCreated, wantBody: `"contentType":"application/pdf"`},
		{name: "the doctor uploads to their draft", path: "/api/v1/doctors/me/records/90/attachments", token: "doctor-token", record: &models.MedicalRecord{ID: 90, PatientID: 3, DoctorID: 5, Status: models.MedicalRecordStatusDraft}, fields: fields, content: pdfBytes, wantStatus: http.StatusCreated, wantBody: `"medicalRecordId":90`},
		{name: "a text file", path: "/api/v1/patients/3/attachments", token: "patient-token", link: fullLink, fields: fields, content: []byte("plain notes"), wantStatus: http.StatusUnsupportedMediaType},
		{name: "no file", path: "/api/v1/patients/3/attachments", token: "patient-token", link: fullLink, fields: fields, wantStatus: http.StatusBadRequest, wantBody: `"details":"file is required"`},
		{name: "no kind", path: "/api/v1/patients/3/attachments", token: "patient-token", link: fullLink, fields: map[string]string{"title": "Blood report"}, content: pdfBytes, wantStatus: http.StatusBadRequest},
		{name: "an unknown kind", path: "/api/v1/patients/3/attachments", token: "patient-token", link: fullLink, fields: map[string]string{"kind": "selfie", "title": "Blood report"}, content: pdfBytes, wantStatus: http.StatusBadRequest},
		{name: "read_only access cannot upload", path: "/api/v1/patients/3/attachments", token: "patient-token", link: readOnlyLink, fields: fields, content: pdfBytes, wantStatus: http.StatusNotFound},
		{name: "a file over the limit", path: "/api/v1/patients/3/attachments", token: "patient-token", link: fullLink, fields: fields, content: append(append([]byte{}, pdfBytes...), make([]byte, 12<<20)...), wantStatus: http.StatusRequestEntityTooLarge},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			userPatients := &fakeUserPatientRepo{userPatient: tt.link}
			if tt.link == nil {
				userPatients.getErr = apperrors.ErrUserPatientNotFound
			}
			router := newMedicalRecordRouter(t, &fakeMedicalRecordRepo{record: tt.record}, &fakeAppointmentRepo{}, userPatients)

			body, contentType := uploadBody(t, tt.fields, tt.content)
			request := httptest.NewRequest(http.MethodPost, tt.path, body)
			request.Header.Set("Content-Type", contentType)
			request.Header.Set("Authorization", "Bearer "+tt.token)
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

	t.Run("an uploaded file downloads byte for byte", func(t *testing.T) {
		records := &fakeMedicalRecordRepo{}
		router := newMedicalRecordRouter(t, records, &fakeAppointmentRepo{}, &fakeUserPatientRepo{userPatient: fullLink})

		body, contentType := uploadBody(t, fields, pdfBytes)
		request := httptest.NewRequest(http.MethodPost, "/api/v1/patients/3/attachments", body)
		request.Header.Set("Content-Type", contentType)
		request.Header.Set("Authorization", "Bearer patient-token")
		recorder := httptest.NewRecorder()
		router.ServeHTTP(recorder, request)
		var uploaded struct {
			Data struct {
				ID uint64 `json:"id"`
			} `json:"data"`
		}
		if err := json.Unmarshal(recorder.Body.Bytes(), &uploaded); err != nil || recorder.Code != http.StatusCreated {
			t.Fatalf("upload status = %d, body = %s", recorder.Code, recorder.Body.String())
		}

		request = httptest.NewRequest(http.MethodGet, fmt.Sprintf("/api/v1/attachments/%d/content", uploaded.Data.ID), nil)
		request.Header.Set("Authorization", "Bearer patient-token")
		recorder = httptest.NewRecorder()
		router.ServeHTTP(recorder, request)

		if recorder.Code != http.StatusOK || !bytes.Equal(recorder.Body.Bytes(), pdfBytes) {
			t.Fatalf("download status = %d with %d bytes, want 200 and the uploaded file", recorder.Code, recorder.Body.Len())
		}
		if recorder.Header().Get("Content-Type") != "application/pdf" || recorder.Header().Get("X-Content-Type-Options") != "nosniff" ||
			recorder.Header().Get("Content-Disposition") != fmt.Sprintf(`attachment; filename="attachment-%d.pdf"`, uploaded.Data.ID) {
			t.Fatalf("download headers = %v, want the pdf type, nosniff and an attachment filename", recorder.Header())
		}
		if len(records.accesses) != 2 || records.accesses[1].Action != models.RecordAccessDownload {
			t.Fatalf("access log = %+v, want an upload then a download", records.accesses)
		}

		request = httptest.NewRequest(http.MethodGet, "/api/v1/attachments/999/content", nil)
		request.Header.Set("Authorization", "Bearer patient-token")
		recorder = httptest.NewRecorder()
		router.ServeHTTP(recorder, request)
		if recorder.Code != http.StatusNotFound {
			t.Fatalf("download of a missing file status = %d, want 404", recorder.Code)
		}
	})
}

func TestAdminRecordAccessLogStatusCodes(t *testing.T) {
	recordID := uint64(90)
	records := func() *fakeMedicalRecordRepo {
		return &fakeMedicalRecordRepo{
			record:   &models.MedicalRecord{ID: 90, PatientID: 3, DoctorID: 5, Status: models.MedicalRecordStatusFinalized},
			accesses: []*models.RecordAccess{{ID: 1, PatientID: 3, MedicalRecordID: &recordID, AccessedByUserID: 7, Action: models.RecordAccessView, CorrelationID: "corr-1"}},
		}
	}

	tests := []struct {
		name       string
		path       string
		token      string
		wantStatus int
		wantBody   string
	}{
		{name: "admin reads the access log", path: "/api/v1/admin/records/90/access-log", token: "admin-token", wantStatus: http.StatusOK, wantBody: `"correlationId":"corr-1"`},
		{name: "a missing record", path: "/api/v1/admin/records/91/access-log", token: "admin-token", wantStatus: http.StatusNotFound},
		{name: "doctor cannot read the access log", path: "/api/v1/admin/records/90/access-log", token: "doctor-token", wantStatus: http.StatusForbidden},
		{name: "patient cannot read the access log", path: "/api/v1/admin/records/90/access-log", token: "patient-token", wantStatus: http.StatusForbidden},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			router := newDoctorAdminRouterWithRecords(t, &fakeDoctorRepo{}, &fakeAvailabilityRepo{}, &fakePaymentRepo{}, records())

			request := httptest.NewRequest(http.MethodGet, tt.path, nil)
			request.Header.Set("Authorization", "Bearer "+tt.token)
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
