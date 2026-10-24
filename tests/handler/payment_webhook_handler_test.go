package handler_test

import (
	"appointment_system/internal/apps/appointment/adapters/payment/youpi"
	"appointment_system/internal/apps/appointment/models"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

const youpiWebhookSecret = "merchant-webhook-secret-at-least-32-chars"

// newYouPIGateway answers YouPI's status API with a paid session sess-1 and
// knows no other session.
func newYouPIGateway(t *testing.T) *youpi.Gateway {
	t.Helper()

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/v1/payments/sess-1" {
			w.WriteHeader(http.StatusNotFound)
			return
		}
		w.Write([]byte(`{"paymentId":"sess-1","status":"SUCCESS","amountPaise":"50000"}`))
	}))
	t.Cleanup(server.Close)
	gateway, err := youpi.New(server.URL, "merchant-1", youpiWebhookSecret, "https://app.example.com/payments/return")
	if err != nil {
		t.Fatalf("youpi.New returned error: %v", err)
	}

	return gateway
}

func signWebhook(body string) string {
	mac := hmac.New(sha256.New, []byte(youpiWebhookSecret))
	mac.Write([]byte(body))
	return "sha256=" + hex.EncodeToString(mac.Sum(nil))
}

func TestPaymentHandlerWebhookStatusCodes(t *testing.T) {
	dueAt := time.Now().Add(5 * time.Minute)
	nextWeek := time.Now().Add(7 * 24 * time.Hour)
	booking := &models.AppointmentDetails{Appointment: models.Appointment{
		ID:              50,
		PatientID:       3,
		DoctorID:        5,
		StartsAt:        nextWeek,
		EndsAt:          nextWeek.Add(15 * time.Minute),
		Status:          models.AppointmentStatusScheduled,
		ConsultationFee: 50000,
		PaymentStatus:   models.AppointmentPaymentUnpaid,
		PaymentDueAt:    &dueAt,
	}}
	success := `{"event":"SUCCESS","paymentId":"sess-1","orderId":"apt_50_1","amount":"500.00","amountPaise":50000,"utr":"412345678901"}`
	unknown := `{"event":"SUCCESS","paymentId":"sess-9","amountPaise":50000,"utr":"412345678909"}`

	tests := []struct {
		name        string
		body        string
		signature   string
		none        bool
		wantStatus  int
		wantBody    string
		wantPayment models.PaymentStatus
	}{
		{name: "a signed success captures the payment", body: success, signature: signWebhook(success), wantStatus: http.StatusOK, wantBody: `"received":true`, wantPayment: models.PaymentStatusCaptured},
		{name: "a forged signature", body: success, signature: signWebhook(success + " "), wantStatus: http.StatusUnauthorized, wantPayment: models.PaymentStatusCreated},
		{name: "no signature", body: success, wantStatus: http.StatusUnauthorized, wantPayment: models.PaymentStatusCreated},
		{name: "a session we never opened", body: unknown, signature: signWebhook(unknown), wantStatus: http.StatusNotFound, wantPayment: models.PaymentStatusCreated},
		{name: "a body over 64 KB", body: success + strings.Repeat(" ", 64<<10), signature: signWebhook(success), wantStatus: http.StatusRequestEntityTooLarge, wantPayment: models.PaymentStatusCreated},
		{name: "the none gateway takes no webhooks", body: success, signature: signWebhook(success), none: true, wantStatus: http.StatusUnauthorized, wantPayment: models.PaymentStatusCreated},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			payments := &fakePaymentRepo{payment: &models.Payment{ID: 70, AppointmentID: 50, PayerUserID: 7, Amount: 50000, Currency: "INR", Provider: youpi.ProviderName, ProviderOrderID: "sess-1", Status: models.PaymentStatusCreated}}
			router := newPaymentRouterWithGateway(t, &fakeAppointmentRepo{byID: booking}, &fakeUserPatientRepo{}, payments, newYouPIGateway(t))
			if tt.none {
				router = newPaymentRouter(t, &fakeAppointmentRepo{byID: booking}, &fakeUserPatientRepo{}, payments)
			}

			request := httptest.NewRequest(http.MethodPost, "/api/v1/webhooks/payment", strings.NewReader(tt.body))
			request.Header.Set("Content-Type", "application/json")
			if tt.signature != "" {
				request.Header.Set("X-Gateway-Signature", tt.signature)
			}
			recorder := httptest.NewRecorder()

			router.ServeHTTP(recorder, request)

			if recorder.Code != tt.wantStatus {
				t.Fatalf("status = %d, want %d, body = %s", recorder.Code, tt.wantStatus, recorder.Body.String())
			}
			if tt.wantBody != "" && !strings.Contains(recorder.Body.String(), tt.wantBody) {
				t.Fatalf("body = %s, want it to contain %s", recorder.Body.String(), tt.wantBody)
			}
			if payments.payment.Status != tt.wantPayment {
				t.Fatalf("payment status = %q, want %q", payments.payment.Status, tt.wantPayment)
			}
		})
	}
}

func TestAdminRefundQueueStatusCodes(t *testing.T) {
	refund := func(status models.RefundStatus) []*models.Refund {
		return []*models.Refund{{ID: 90, PaymentID: 70, Amount: 50000, Reason: models.RefundReasonDoctorCancelled, Status: status}}
	}

	tests := []struct {
		name       string
		method     string
		path       string
		body       string
		token      string
		refunds    []*models.Refund
		youpi      bool
		wantStatus int
		wantBody   string
	}{
		{name: "admin lists the manual queue", method: http.MethodGet, path: "/api/v1/admin/refunds", token: "admin-token", refunds: refund(models.RefundStatusRequested), wantStatus: http.StatusOK, wantBody: `"refunds":[{"id":90`},
		{name: "an unknown status", method: http.MethodGet, path: "/api/v1/admin/refunds?status=pending", token: "admin-token", wantStatus: http.StatusBadRequest, wantBody: `"details":"status must be one of requested, succeeded, failed"`},
		{name: "patient cannot list refunds", method: http.MethodGet, path: "/api/v1/admin/refunds", token: "patient-token", wantStatus: http.StatusForbidden},
		{name: "admin completes a payout", method: http.MethodPost, path: "/api/v1/admin/refunds/90/complete", body: `{"reference":"412345678901"}`, token: "admin-token", refunds: refund(models.RefundStatusRequested), wantStatus: http.StatusOK, wantBody: `"providerRefundId":"412345678901"`},
		{name: "complete without a reference", method: http.MethodPost, path: "/api/v1/admin/refunds/90/complete", body: `{}`, token: "admin-token", refunds: refund(models.RefundStatusRequested), wantStatus: http.StatusBadRequest},
		{name: "complete a finished refund", method: http.MethodPost, path: "/api/v1/admin/refunds/90/complete", body: `{"reference":"412345678901"}`, token: "admin-token", refunds: refund(models.RefundStatusSucceeded), wantStatus: http.StatusConflict},
		{name: "complete a missing refund", method: http.MethodPost, path: "/api/v1/admin/refunds/91/complete", body: `{"reference":"412345678901"}`, token: "admin-token", wantStatus: http.StatusNotFound},
		{name: "doctor cannot complete a refund", method: http.MethodPost, path: "/api/v1/admin/refunds/90/complete", body: `{"reference":"412345678901"}`, token: "doctor-token", refunds: refund(models.RefundStatusRequested), wantStatus: http.StatusForbidden},
		{name: "a youpi refund is accepted for a manual payout", method: http.MethodPost, path: "/api/v1/admin/payments/70/refund", token: "admin-token", youpi: true, wantStatus: http.StatusAccepted, wantBody: `"refundPending":true`},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			payments := &fakePaymentRepo{payment: &models.Payment{ID: 70, AppointmentID: 50, Amount: 50000, Currency: "INR", Provider: youpi.ProviderName, ProviderOrderID: "sess-1", Status: models.PaymentStatusCaptured}, refunds: tt.refunds}
			router := newDoctorAdminRouterWithPayments(t, &fakeDoctorRepo{}, &fakeAvailabilityRepo{}, payments)
			if tt.youpi {
				router = newDoctorAdminRouterWithGateway(t, &fakeDoctorRepo{}, &fakeAvailabilityRepo{}, payments, &fakeMedicalRecordRepo{}, newYouPIGateway(t))
			}

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
