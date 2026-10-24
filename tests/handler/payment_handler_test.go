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

func newPaymentRouter(
	t *testing.T,
	appointments *fakeAppointmentRepo,
	userPatients *fakeUserPatientRepo,
	payments *fakePaymentRepo,
) *gin.Engine {
	t.Helper()

	return newPaymentRouterWithGateway(t, appointments, userPatients, payments, none.New())
}

func newPaymentRouterWithGateway(
	t *testing.T,
	appointments *fakeAppointmentRepo,
	userPatients *fakeUserPatientRepo,
	payments *fakePaymentRepo,
	gateway services.PaymentGateway,
) *gin.Engine {
	t.Helper()
	gin.SetMode(gin.TestMode)

	paymentService, err := services.NewPaymentService(payments, appointments, &fakeOutboxRepo{}, userPatients, &fakeDoctorRepo{}, gateway, &fakeTxManager{}, time.Now)
	if err != nil {
		t.Fatalf("NewPaymentService returned error: %v", err)
	}
	paymentController, err := controller.NewPaymentController(paymentService)
	if err != nil {
		t.Fatalf("NewPaymentController returned error: %v", err)
	}
	paymentHandler, err := handler.NewPaymentHandler(paymentController, &fakeRoleTokenVerifier{})
	if err != nil {
		t.Fatalf("NewPaymentHandler returned error: %v", err)
	}

	router := gin.New()
	router.Use(middleware.AppContext())
	paymentHandler.RegisterRoutes(router.Group("/api/v1"))

	return router
}

func TestPaymentHandlerStatusCodes(t *testing.T) {
	nextWeek := time.Now().Add(7 * 24 * time.Hour)
	booking := func(paymentStatus models.AppointmentPaymentStatus, dueIn time.Duration) *models.AppointmentDetails {
		dueAt := time.Now().Add(dueIn)
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
	openPayment := func() *models.Payment {
		return &models.Payment{ID: 70, AppointmentID: 50, PayerUserID: 7, Amount: 50000, Currency: "INR", Provider: none.ProviderName, ProviderOrderID: "none_order_x", Status: models.PaymentStatusCreated}
	}
	fullLink := &models.UserPatient{UserID: 7, PatientID: 3, PatientRelationship: models.RelationshipChild, AccessLevel: models.AccessLevelFull, Status: models.UserPatientStatusActive}
	readOnlyLink := &models.UserPatient{UserID: 7, PatientID: 3, PatientRelationship: models.RelationshipChild, AccessLevel: models.AccessLevelReadOnly, Status: models.UserPatientStatusActive}
	verifyPayload := `{"providerPaymentId":"pay_1","signature":"none_signature_none_order_x"}`

	tests := []struct {
		name        string
		method      string
		path        string
		body        string
		token       string
		link        *models.UserPatient
		appointment *models.AppointmentDetails
		payment     *models.Payment
		wantStatus  int
		wantBody    string
	}{
		{name: "pay without token", method: http.MethodPost, path: "/api/v1/appointments/50/payments", wantStatus: http.StatusUnauthorized},
		{name: "patient opens a payment", method: http.MethodPost, path: "/api/v1/appointments/50/payments", token: "patient-token", link: fullLink, appointment: booking(models.AppointmentPaymentUnpaid, 5*time.Minute), wantStatus: http.StatusCreated, wantBody: `"signature":"none_signature_none_order_50_`},
		{name: "doctor account cannot pay", method: http.MethodPost, path: "/api/v1/appointments/50/payments", token: "doctor-token", appointment: booking(models.AppointmentPaymentUnpaid, 5*time.Minute), wantStatus: http.StatusForbidden},
		{name: "read_only access cannot pay", method: http.MethodPost, path: "/api/v1/appointments/50/payments", token: "patient-token", link: readOnlyLink, appointment: booking(models.AppointmentPaymentUnpaid, 5*time.Minute), wantStatus: http.StatusForbidden},
		{name: "pay without a link", method: http.MethodPost, path: "/api/v1/appointments/50/payments", token: "patient-token", appointment: booking(models.AppointmentPaymentUnpaid, 5*time.Minute), wantStatus: http.StatusNotFound},
		{name: "pay for a paid booking", method: http.MethodPost, path: "/api/v1/appointments/50/payments", token: "patient-token", link: fullLink, appointment: booking(models.AppointmentPaymentPaid, 5*time.Minute), wantStatus: http.StatusConflict},
		{name: "pay after the hold", method: http.MethodPost, path: "/api/v1/appointments/50/payments", token: "patient-token", link: fullLink, appointment: booking(models.AppointmentPaymentUnpaid, -time.Minute), wantStatus: http.StatusConflict, wantBody: `"details":"the payment window for this booking has closed; book again"`},
		{name: "get the payment", method: http.MethodGet, path: "/api/v1/appointments/50/payment", token: "patient-token", link: readOnlyLink, appointment: booking(models.AppointmentPaymentUnpaid, 5*time.Minute), payment: openPayment(), wantStatus: http.StatusOK, wantBody: `"status":"created"`},
		{name: "get when nothing was paid", method: http.MethodGet, path: "/api/v1/appointments/50/payment", token: "patient-token", link: fullLink, appointment: booking(models.AppointmentPaymentUnpaid, 5*time.Minute), wantStatus: http.StatusNotFound},
		{name: "verify with the right signature", method: http.MethodPost, path: "/api/v1/payments/70/verify", body: verifyPayload, token: "patient-token", link: fullLink, appointment: booking(models.AppointmentPaymentUnpaid, 5*time.Minute), payment: openPayment(), wantStatus: http.StatusOK, wantBody: `"status":"captured"`},
		{name: "verify with a wrong signature", method: http.MethodPost, path: "/api/v1/payments/70/verify", body: strings.Replace(verifyPayload, "none_signature_", "forged_", 1), token: "patient-token", link: fullLink, appointment: booking(models.AppointmentPaymentUnpaid, 5*time.Minute), payment: openPayment(), wantStatus: http.StatusUnprocessableEntity},
		{name: "verify without a signature fails with the none gateway", method: http.MethodPost, path: "/api/v1/payments/70/verify", body: `{"providerPaymentId":"pay_1"}`, token: "patient-token", link: fullLink, appointment: booking(models.AppointmentPaymentUnpaid, 5*time.Minute), payment: openPayment(), wantStatus: http.StatusUnprocessableEntity},
		{name: "verify without a payment id", method: http.MethodPost, path: "/api/v1/payments/70/verify", body: `{"signature":"none_signature_none_order_x"}`, token: "patient-token", link: fullLink, wantStatus: http.StatusBadRequest},
		{name: "verify after the hold refunds", method: http.MethodPost, path: "/api/v1/payments/70/verify", body: verifyPayload, token: "patient-token", link: fullLink, appointment: booking(models.AppointmentPaymentUnpaid, -time.Minute), payment: openPayment(), wantStatus: http.StatusConflict, wantBody: `the payment is being refunded`},
		{name: "verify a missing payment", method: http.MethodPost, path: "/api/v1/payments/71/verify", body: verifyPayload, token: "patient-token", link: fullLink, wantStatus: http.StatusNotFound},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			userPatients := &fakeUserPatientRepo{userPatient: tt.link}
			if tt.link == nil {
				userPatients.getErr = apperrors.ErrUserPatientNotFound
			}
			router := newPaymentRouter(t, &fakeAppointmentRepo{byID: tt.appointment}, userPatients, &fakePaymentRepo{payment: tt.payment})

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

func TestAdminRefundPaymentStatusCodes(t *testing.T) {
	payment := func(status models.PaymentStatus) *models.Payment {
		return &models.Payment{ID: 70, AppointmentID: 50, Amount: 50000, Currency: "INR", Provider: none.ProviderName, ProviderOrderID: "none_order_x", Status: status}
	}

	tests := []struct {
		name       string
		path       string
		token      string
		payment    *models.Payment
		wantStatus int
		wantBody   string
	}{
		{name: "admin refunds a captured payment", path: "/api/v1/admin/payments/70/refund", token: "admin-token", payment: payment(models.PaymentStatusCaptured), wantStatus: http.StatusOK, wantBody: `"status":"refunded"`},
		{name: "a payment that was never captured", path: "/api/v1/admin/payments/70/refund", token: "admin-token", payment: payment(models.PaymentStatusCreated), wantStatus: http.StatusConflict},
		{name: "a missing payment", path: "/api/v1/admin/payments/71/refund", token: "admin-token", wantStatus: http.StatusNotFound},
		{name: "patient cannot refund", path: "/api/v1/admin/payments/70/refund", token: "patient-token", payment: payment(models.PaymentStatusCaptured), wantStatus: http.StatusForbidden},
		{name: "doctor cannot refund", path: "/api/v1/admin/payments/70/refund", token: "doctor-token", payment: payment(models.PaymentStatusCaptured), wantStatus: http.StatusForbidden},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			router := newDoctorAdminRouterWithPayments(t, &fakeDoctorRepo{}, &fakeAvailabilityRepo{}, &fakePaymentRepo{payment: tt.payment})

			request := httptest.NewRequest(http.MethodPost, tt.path, nil)
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
