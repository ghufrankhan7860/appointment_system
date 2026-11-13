package handler_test

import (
	"bytes"
	"mime/multipart"
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
	"appointment_system/middleware"

	"github.com/gin-gonic/gin"
)

var pngImage = []byte("\x89PNG\r\n\x1a\n\x00\x00\x00\rIHDR")

func newManualPaymentRouter(
	t *testing.T,
	doctors *fakeDoctorRepo,
	appointments *fakeAppointmentRepo,
	userPatients *fakeUserPatientRepo,
	payments *fakePaymentRepo,
	storage *fakeFileStorage,
) *gin.Engine {
	t.Helper()
	gin.SetMode(gin.TestMode)

	methods := []models.PaymentMethod{models.PaymentMethodUPI, models.PaymentMethodClinic}
	paymentService, err := services.NewPaymentService(payments, appointments, &fakeOutboxRepo{}, userPatients, doctors, none.New(), methods, storage, &fakeTxManager{}, time.Now, discardLogger)
	if err != nil {
		t.Fatalf("NewPaymentService returned error: %v", err)
	}
	paymentController, err := controller.NewPaymentController(paymentService)
	if err != nil {
		t.Fatalf("NewPaymentController returned error: %v", err)
	}
	paymentHandler, err := handler.NewPaymentHandler(paymentController, &fakeRoleTokenVerifier{}, discardLogger)
	if err != nil {
		t.Fatalf("NewPaymentHandler returned error: %v", err)
	}

	router := gin.New()
	router.Use(middleware.AppContext())
	paymentHandler.RegisterRoutes(router.Group("/api/v1"))

	return router
}

// multipartBody writes fields and, when content is not nil, a file under
// fileField, returning the body and its content type.
func multipartBody(t *testing.T, fields map[string]string, fileField string, content []byte) (string, string) {
	t.Helper()

	var body bytes.Buffer
	writer := multipart.NewWriter(&body)
	for name, value := range fields {
		if err := writer.WriteField(name, value); err != nil {
			t.Fatalf("WriteField returned error: %v", err)
		}
	}
	if content != nil {
		part, err := writer.CreateFormFile(fileField, "image.png")
		if err != nil {
			t.Fatalf("CreateFormFile returned error: %v", err)
		}
		if _, err := part.Write(content); err != nil {
			t.Fatalf("Write returned error: %v", err)
		}
	}
	if err := writer.Close(); err != nil {
		t.Fatalf("Close returned error: %v", err)
	}

	return body.String(), writer.FormDataContentType()
}

func TestManualPaymentHandlerStatusCodes(t *testing.T) {
	nextWeek := time.Now().Add(7 * 24 * time.Hour)
	booking := func(paymentStatus models.AppointmentPaymentStatus) *models.AppointmentDetails {
		dueAt := time.Now().Add(5 * time.Minute)
		if paymentStatus == models.AppointmentPaymentPendingVerification {
			dueAt = nextWeek
		}
		return &models.AppointmentDetails{Appointment: models.Appointment{
			ID:              50,
			PatientID:       3,
			DoctorID:        5,
			StartsAt:        nextWeek,
			EndsAt:          nextWeek.Add(15 * time.Minute),
			Mode:            models.ConsultationModeInPerson,
			Status:          models.AppointmentStatusScheduled,
			ConsultationFee: 50000,
			PaymentStatus:   paymentStatus,
			PaymentDueAt:    &dueAt,
		}}
	}
	upiPayment := func(status models.PaymentStatus) *models.Payment {
		proofKey := "payments/70/proof-x"
		return &models.Payment{ID: 70, AppointmentID: 50, PayerUserID: 7, Amount: 50000, Currency: "INR", Provider: models.ProviderUPIDirect, ProviderOrderID: "upi_direct_50_x", Status: status, PayerReference: "412356789012", ProofStorageKey: &proofKey}
	}
	requestedRefund := func() []*models.Refund {
		return []*models.Refund{{ID: 90, PaymentID: 70, Amount: 50000, Reason: models.RefundReasonPatientCancelled, Status: models.RefundStatusRequested}}
	}
	fullLink := &models.UserPatient{UserID: 7, PatientID: 3, PatientRelationship: models.RelationshipChild, AccessLevel: models.AccessLevelFull, Status: models.UserPatientStatusActive}
	claimBody, claimType := multipartBody(t, map[string]string{"reference": "4123 5678 9012"}, "proof", pngImage)
	noReferenceBody, noReferenceType := multipartBody(t, map[string]string{}, "proof", pngImage)
	badReferenceBody, badReferenceType := multipartBody(t, map[string]string{"reference": "41-23"}, "", nil)
	qrBody, qrType := multipartBody(t, map[string]string{}, "file", pngImage)
	noQRBody, noQRType := multipartBody(t, map[string]string{}, "", nil)
	notImageBody, notImageType := multipartBody(t, map[string]string{}, "file", []byte("%PDF-1.7"))

	tests := []struct {
		name        string
		method      string
		path        string
		body        string
		contentType string
		token       string
		noUPI       bool
		withQR      bool
		appointment *models.AppointmentDetails
		payment     *models.Payment
		refunds     []*models.Refund
		wantStatus  int
		wantBody    string
	}{
		{name: "patient reads the payment options", method: http.MethodGet, path: "/api/v1/appointments/50/payment-options", token: "patient-token", appointment: booking(models.AppointmentPaymentUnpaid), wantStatus: http.StatusOK, wantBody: `"methods":["upi","clinic"],"upi":{"upiId":"asha@okbank"`},
		{name: "options without a token", method: http.MethodGet, path: "/api/v1/appointments/50/payment-options", wantStatus: http.StatusUnauthorized},
		{name: "options of a missing booking", method: http.MethodGet, path: "/api/v1/appointments/50/payment-options", token: "patient-token", wantStatus: http.StatusNotFound},
		{name: "patient starts a upi payment", method: http.MethodPost, path: "/api/v1/appointments/50/payments", body: `{"method":"upi"}`, contentType: "application/json", token: "patient-token", appointment: booking(models.AppointmentPaymentUnpaid), wantStatus: http.StatusCreated, wantBody: `"upiLink":"upi://pay?`},
		{name: "upi with a doctor who has no upi id", method: http.MethodPost, path: "/api/v1/appointments/50/payments", body: `{"method":"upi"}`, contentType: "application/json", token: "patient-token", noUPI: true, appointment: booking(models.AppointmentPaymentUnpaid), wantStatus: http.StatusConflict},
		{name: "razorpay when it is off", method: http.MethodPost, path: "/api/v1/appointments/50/payments", body: `{"method":"razorpay"}`, contentType: "application/json", token: "patient-token", appointment: booking(models.AppointmentPaymentUnpaid), wantStatus: http.StatusConflict},
		{name: "an unknown method", method: http.MethodPost, path: "/api/v1/appointments/50/payments", body: `{"method":"cash"}`, contentType: "application/json", token: "patient-token", appointment: booking(models.AppointmentPaymentUnpaid), wantStatus: http.StatusBadRequest},
		{name: "patient chooses to pay at the clinic", method: http.MethodPost, path: "/api/v1/appointments/50/payment/clinic", token: "patient-token", appointment: booking(models.AppointmentPaymentUnpaid), wantStatus: http.StatusOK},
		{name: "pay at the clinic for a paid booking", method: http.MethodPost, path: "/api/v1/appointments/50/payment/clinic", token: "patient-token", appointment: booking(models.AppointmentPaymentPaid), wantStatus: http.StatusConflict},
		{name: "patient opens the doctor's qr", method: http.MethodGet, path: "/api/v1/appointments/50/payment/upi-qr", token: "patient-token", withQR: true, appointment: booking(models.AppointmentPaymentUnpaid), wantStatus: http.StatusOK},
		{name: "a doctor without a qr", method: http.MethodGet, path: "/api/v1/appointments/50/payment/upi-qr", token: "patient-token", appointment: booking(models.AppointmentPaymentUnpaid), wantStatus: http.StatusNotFound},
		{name: "patient claims with a reference and screenshot", method: http.MethodPost, path: "/api/v1/payments/70/claim", body: claimBody, contentType: claimType, token: "patient-token", appointment: booking(models.AppointmentPaymentUnpaid), payment: upiPayment(models.PaymentStatusCreated), wantStatus: http.StatusOK, wantBody: `"status":"claimed"`},
		{name: "a claim without a reference", method: http.MethodPost, path: "/api/v1/payments/70/claim", body: noReferenceBody, contentType: noReferenceType, token: "patient-token", appointment: booking(models.AppointmentPaymentUnpaid), payment: upiPayment(models.PaymentStatusCreated), wantStatus: http.StatusBadRequest},
		{name: "a claim with a bad reference", method: http.MethodPost, path: "/api/v1/payments/70/claim", body: badReferenceBody, contentType: badReferenceType, token: "patient-token", appointment: booking(models.AppointmentPaymentUnpaid), payment: upiPayment(models.PaymentStatusCreated), wantStatus: http.StatusBadRequest, wantBody: `"details":"reference must be`},
		{name: "a claim made twice", method: http.MethodPost, path: "/api/v1/payments/70/claim", body: claimBody, contentType: claimType, token: "patient-token", appointment: booking(models.AppointmentPaymentPendingVerification), payment: upiPayment(models.PaymentStatusClaimed), wantStatus: http.StatusConflict},
		{name: "a doctor account cannot claim", method: http.MethodPost, path: "/api/v1/payments/70/claim", body: claimBody, contentType: claimType, token: "doctor-token", appointment: booking(models.AppointmentPaymentUnpaid), payment: upiPayment(models.PaymentStatusCreated), wantStatus: http.StatusForbidden},
		{name: "the doctor opens the screenshot", method: http.MethodGet, path: "/api/v1/payments/70/proof", token: "doctor-token", appointment: booking(models.AppointmentPaymentPendingVerification), payment: upiPayment(models.PaymentStatusClaimed), wantStatus: http.StatusOK},
		{name: "doctor sets a upi id", method: http.MethodPut, path: "/api/v1/doctors/me/upi", body: `{"upiId":"asha@okaxis"}`, contentType: "application/json", token: "doctor-token", wantStatus: http.StatusOK, wantBody: `"upiId":"asha@okaxis"`},
		{name: "a upi id that is not one", method: http.MethodPut, path: "/api/v1/doctors/me/upi", body: `{"upiId":"9876543210"}`, contentType: "application/json", token: "doctor-token", wantStatus: http.StatusBadRequest, wantBody: `"details":"upiId must look like name@bank"`},
		{name: "a patient cannot set a upi id", method: http.MethodPut, path: "/api/v1/doctors/me/upi", body: `{"upiId":"asha@okaxis"}`, contentType: "application/json", token: "patient-token", wantStatus: http.StatusForbidden},
		{name: "doctor uploads a qr", method: http.MethodPut, path: "/api/v1/doctors/me/upi-qr", body: qrBody, contentType: qrType, token: "doctor-token", wantStatus: http.StatusOK, wantBody: `"hasUpiQr":true`},
		{name: "a qr upload without a file", method: http.MethodPut, path: "/api/v1/doctors/me/upi-qr", body: noQRBody, contentType: noQRType, token: "doctor-token", wantStatus: http.StatusBadRequest},
		{name: "a qr that is not an image", method: http.MethodPut, path: "/api/v1/doctors/me/upi-qr", body: notImageBody, contentType: notImageType, token: "doctor-token", wantStatus: http.StatusBadRequest},
		{name: "doctor opens their own qr", method: http.MethodGet, path: "/api/v1/doctors/me/upi-qr", token: "doctor-token", withQR: true, wantStatus: http.StatusOK},
		{name: "doctor removes their qr", method: http.MethodDelete, path: "/api/v1/doctors/me/upi-qr", token: "doctor-token", withQR: true, wantStatus: http.StatusOK, wantBody: `"hasUpiQr":false`},
		{name: "doctor lists claims to check", method: http.MethodGet, path: "/api/v1/doctors/me/payments/claims", token: "doctor-token", appointment: booking(models.AppointmentPaymentPendingVerification), payment: upiPayment(models.PaymentStatusClaimed), wantStatus: http.StatusOK, wantBody: `"payerReference":"412356789012"`},
		{name: "a patient cannot list claims", method: http.MethodGet, path: "/api/v1/doctors/me/payments/claims", token: "patient-token", wantStatus: http.StatusForbidden},
		{name: "doctor verifies a claim", method: http.MethodPost, path: "/api/v1/doctors/me/payments/70/verify", body: `{"note":"seen"}`, contentType: "application/json", token: "doctor-token", appointment: booking(models.AppointmentPaymentPendingVerification), payment: upiPayment(models.PaymentStatusClaimed), wantStatus: http.StatusOK, wantBody: `"status":"captured"`},
		{name: "verify a payment not claimed", method: http.MethodPost, path: "/api/v1/doctors/me/payments/70/verify", token: "doctor-token", appointment: booking(models.AppointmentPaymentUnpaid), payment: upiPayment(models.PaymentStatusCreated), wantStatus: http.StatusConflict},
		{name: "verify a missing payment", method: http.MethodPost, path: "/api/v1/doctors/me/payments/71/verify", token: "doctor-token", wantStatus: http.StatusNotFound},
		{name: "doctor rejects a claim", method: http.MethodPost, path: "/api/v1/doctors/me/payments/70/reject", body: `{"reason":"not in my account"}`, contentType: "application/json", token: "doctor-token", appointment: booking(models.AppointmentPaymentPendingVerification), payment: upiPayment(models.PaymentStatusClaimed), wantStatus: http.StatusOK, wantBody: `"status":"rejected"`},
		{name: "a rejection without a reason", method: http.MethodPost, path: "/api/v1/doctors/me/payments/70/reject", body: `{}`, contentType: "application/json", token: "doctor-token", appointment: booking(models.AppointmentPaymentPendingVerification), payment: upiPayment(models.PaymentStatusClaimed), wantStatus: http.StatusBadRequest},
		{name: "doctor marks a clinic fee collected", method: http.MethodPost, path: "/api/v1/doctors/me/appointments/50/collected", body: `{"note":"cash"}`, contentType: "application/json", token: "doctor-token", appointment: booking(models.AppointmentPaymentDueAtClinic), wantStatus: http.StatusOK, wantBody: `"provider":"clinic"`},
		{name: "collected on a booking not due at the clinic", method: http.MethodPost, path: "/api/v1/doctors/me/appointments/50/collected", token: "doctor-token", appointment: booking(models.AppointmentPaymentPaid), wantStatus: http.StatusConflict},
		{name: "doctor lists refunds due", method: http.MethodGet, path: "/api/v1/doctors/me/refunds", token: "doctor-token", appointment: booking(models.AppointmentPaymentPaid), payment: upiPayment(models.PaymentStatusCaptured), refunds: requestedRefund(), wantStatus: http.StatusOK, wantBody: `"amount":50000`},
		{name: "doctor closes a refund as sent", method: http.MethodPost, path: "/api/v1/doctors/me/refunds/90/close", body: `{"outcome":"refunded","note":"sent on GPay"}`, contentType: "application/json", token: "doctor-token", appointment: booking(models.AppointmentPaymentPaid), payment: upiPayment(models.PaymentStatusCaptured), refunds: requestedRefund(), wantStatus: http.StatusOK},
		{name: "a verified payment cannot be closed as never received", method: http.MethodPost, path: "/api/v1/doctors/me/refunds/90/close", body: `{"outcome":"not_received"}`, contentType: "application/json", token: "doctor-token", appointment: booking(models.AppointmentPaymentPaid), payment: upiPayment(models.PaymentStatusCaptured), refunds: requestedRefund(), wantStatus: http.StatusConflict},
		{name: "an unknown refund outcome", method: http.MethodPost, path: "/api/v1/doctors/me/refunds/90/close", body: `{"outcome":"maybe"}`, contentType: "application/json", token: "doctor-token", wantStatus: http.StatusBadRequest},
		{name: "close a missing refund", method: http.MethodPost, path: "/api/v1/doctors/me/refunds/91/close", body: `{"outcome":"refunded"}`, contentType: "application/json", token: "doctor-token", wantStatus: http.StatusNotFound},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			doctor := &models.Doctor{ID: 5, UserID: 11, FullName: "Asha Rao", UPIID: "asha@okbank"}
			if tt.noUPI {
				doctor.UPIID = ""
			}
			storage := &fakeFileStorage{files: map[string][]byte{"payments/70/proof-x": pngImage}}
			if tt.withQR {
				qrKey := "doctors/5/upi-qr-x"
				doctor.UPIQRStorageKey = &qrKey
				storage.files[qrKey] = pngImage
			}
			userPatients := &fakeUserPatientRepo{userPatient: fullLink}
			if tt.token != "patient-token" {
				userPatients = &fakeUserPatientRepo{getErr: apperrors.ErrUserPatientNotFound}
			}
			router := newManualPaymentRouter(t, &fakeDoctorRepo{doctor: doctor}, &fakeAppointmentRepo{byID: tt.appointment}, userPatients, &fakePaymentRepo{payment: tt.payment, refunds: tt.refunds}, storage)

			request := httptest.NewRequest(tt.method, tt.path, strings.NewReader(tt.body))
			if tt.contentType != "" {
				request.Header.Set("Content-Type", tt.contentType)
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
			if tt.method == http.MethodGet && tt.wantStatus == http.StatusOK && strings.Contains(tt.path, "qr") || strings.HasSuffix(tt.path, "/proof") && tt.wantStatus == http.StatusOK {
				if recorder.Header().Get("Content-Type") != "image/png" || !bytes.Equal(recorder.Body.Bytes(), pngImage) {
					t.Fatalf("image response %q with %d bytes, want the png", recorder.Header().Get("Content-Type"), recorder.Body.Len())
				}
			}
		})
	}
}

func TestAdminManualPaymentStatusCodes(t *testing.T) {
	tests := []struct {
		name       string
		method     string
		path       string
		body       string
		token      string
		wantStatus int
	}{
		{name: "admin lists every claim", method: http.MethodGet, path: "/api/v1/admin/payments/claims", token: "admin-token", wantStatus: http.StatusOK},
		{name: "a doctor cannot use the admin claims", method: http.MethodGet, path: "/api/v1/admin/payments/claims", token: "doctor-token", wantStatus: http.StatusForbidden},
		{name: "admin verifies a missing claim", method: http.MethodPost, path: "/api/v1/admin/payments/71/claim/verify", token: "admin-token", wantStatus: http.StatusNotFound},
		{name: "admin rejects without a reason", method: http.MethodPost, path: "/api/v1/admin/payments/70/claim/reject", body: `{}`, token: "admin-token", wantStatus: http.StatusBadRequest},
		{name: "a patient cannot reject", method: http.MethodPost, path: "/api/v1/admin/payments/70/claim/reject", body: `{"reason":"no"}`, token: "patient-token", wantStatus: http.StatusForbidden},
		{name: "admin lists manual refunds", method: http.MethodGet, path: "/api/v1/admin/refunds/manual", token: "admin-token", wantStatus: http.StatusOK},
		{name: "admin closes a missing refund", method: http.MethodPost, path: "/api/v1/admin/refunds/91/close", body: `{"outcome":"refunded"}`, token: "admin-token", wantStatus: http.StatusNotFound},
		{name: "a doctor cannot close through admin", method: http.MethodPost, path: "/api/v1/admin/refunds/91/close", body: `{"outcome":"refunded"}`, token: "doctor-token", wantStatus: http.StatusForbidden},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			router := newDoctorAdminRouterWithPayments(t, &fakeDoctorRepo{}, &fakeAvailabilityRepo{}, &fakePaymentRepo{})

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
		})
	}
}
