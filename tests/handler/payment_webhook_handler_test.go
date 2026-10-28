package handler_test

import (
	"appointment_system/internal/apps/appointment/adapters/payment/razorpay"
	"appointment_system/internal/apps/appointment/models"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

const (
	razorpayKeySecret     = "razorpay-key-secret"
	razorpayWebhookSecret = "razorpay-webhook-secret-at-least-32-chars"
)

// newRazorpayGateway answers Razorpay's API with the captured payment pay_1 of
// order_1, refunds it as pending and knows nothing else.
func newRazorpayGateway(t *testing.T) *razorpay.Gateway {
	t.Helper()

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch r.Method + " " + r.URL.Path {
		case "GET /v1/payments/pay_1":
			fmt.Fprintf(w, `{"id":"pay_1","order_id":"order_1","amount":50000,"currency":"INR","status":"captured","created_at":%d}`, time.Now().Unix())
		case "POST /v1/payments/pay_1/refund":
			w.Write([]byte(`{"id":"rfnd_1","payment_id":"pay_1","amount":50000,"status":"pending"}`))
		default:
			w.WriteHeader(http.StatusBadRequest)
			w.Write([]byte(`{"error":{"code":"BAD_REQUEST_ERROR","description":"The id provided does not exist"}}`))
		}
	}))
	t.Cleanup(server.Close)
	gateway, err := razorpay.New("rzp_test_key", razorpayKeySecret, razorpayWebhookSecret)
	if err != nil {
		t.Fatalf("razorpay.New returned error: %v", err)
	}

	return gateway.WithBaseURL(server.URL)
}

func hexHMAC(secret string, message string) string {
	mac := hmac.New(sha256.New, []byte(secret))
	mac.Write([]byte(message))
	return hex.EncodeToString(mac.Sum(nil))
}

func razorpayBooking() *models.AppointmentDetails {
	dueAt := time.Now().Add(5 * time.Minute)
	nextWeek := time.Now().Add(7 * 24 * time.Hour)
	return &models.AppointmentDetails{Appointment: models.Appointment{
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
}

func razorpayPayment(status models.PaymentStatus) *models.Payment {
	payment := &models.Payment{ID: 70, AppointmentID: 50, PayerUserID: 7, Amount: 50000, Currency: "INR", Provider: razorpay.ProviderName, ProviderOrderID: "order_1", Status: status}
	if status != models.PaymentStatusCreated {
		payment.ProviderPaymentID = "pay_1"
	}
	return payment
}

func TestPaymentHandlerWebhookStatusCodes(t *testing.T) {
	captured := `{"entity":"event","event":"payment.captured","payload":{"payment":{"entity":{"id":"pay_1","order_id":"order_1","amount":50000,"currency":"INR","status":"captured"}}}}`
	failed := `{"entity":"event","event":"payment.failed","payload":{"payment":{"entity":{"id":"pay_2","order_id":"order_1","amount":50000,"currency":"INR","status":"failed"}}}}`
	unknown := `{"entity":"event","event":"payment.captured","payload":{"payment":{"entity":{"id":"pay_9","order_id":"order_9","amount":50000,"currency":"INR","status":"captured"}}}}`
	refunded := `{"entity":"event","event":"refund.processed","payload":{"refund":{"entity":{"id":"rfnd_1","payment_id":"pay_1","amount":50000,"status":"processed"}}}}`

	tests := []struct {
		name        string
		body        string
		signature   string
		payment     *models.Payment
		refunds     []*models.Refund
		none        bool
		wantStatus  int
		wantBody    string
		wantPayment models.PaymentStatus
	}{
		{name: "a signed capture captures the payment", body: captured, signature: hexHMAC(razorpayWebhookSecret, captured), wantStatus: http.StatusOK, wantBody: `"received":true`, wantPayment: models.PaymentStatusCaptured},
		{name: "a forged signature", body: captured, signature: hexHMAC(razorpayWebhookSecret, captured+" "), wantStatus: http.StatusUnauthorized, wantPayment: models.PaymentStatusCreated},
		{name: "a body signed with the key secret", body: captured, signature: hexHMAC(razorpayKeySecret, captured), wantStatus: http.StatusUnauthorized, wantPayment: models.PaymentStatusCreated},
		{name: "no signature", body: captured, wantStatus: http.StatusUnauthorized, wantPayment: models.PaymentStatusCreated},
		{name: "a failed attempt leaves the order open for a retry", body: failed, signature: hexHMAC(razorpayWebhookSecret, failed), wantStatus: http.StatusOK, wantBody: `"Webhook ignored"`, wantPayment: models.PaymentStatusCreated},
		{name: "an order we never opened is acknowledged", body: unknown, signature: hexHMAC(razorpayWebhookSecret, unknown), wantStatus: http.StatusOK, wantBody: `"Webhook ignored"`, wantPayment: models.PaymentStatusCreated},
		{name: "a processed refund marks the payment refunded", body: refunded, signature: hexHMAC(razorpayWebhookSecret, refunded), payment: razorpayPayment(models.PaymentStatusCaptured), refunds: []*models.Refund{{ID: 90, PaymentID: 70, Amount: 50000, Reason: models.RefundReasonAdmin, Status: models.RefundStatusRequested}}, wantStatus: http.StatusOK, wantPayment: models.PaymentStatusRefunded},
		{name: "a body over 64 KB", body: captured + strings.Repeat(" ", 64<<10), signature: hexHMAC(razorpayWebhookSecret, captured), wantStatus: http.StatusRequestEntityTooLarge, wantPayment: models.PaymentStatusCreated},
		{name: "the none gateway takes no webhooks", body: captured, signature: hexHMAC(razorpayWebhookSecret, captured), none: true, wantStatus: http.StatusUnauthorized, wantPayment: models.PaymentStatusCreated},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if tt.payment == nil {
				tt.payment = razorpayPayment(models.PaymentStatusCreated)
			}
			payments := &fakePaymentRepo{payment: tt.payment, refunds: tt.refunds}
			router := newPaymentRouterWithGateway(t, &fakeAppointmentRepo{byID: razorpayBooking()}, &fakeUserPatientRepo{}, payments, newRazorpayGateway(t))
			if tt.none {
				router = newPaymentRouter(t, &fakeAppointmentRepo{byID: razorpayBooking()}, &fakeUserPatientRepo{}, payments)
			}

			request := httptest.NewRequest(http.MethodPost, "/api/v1/webhooks/payment", strings.NewReader(tt.body))
			request.Header.Set("Content-Type", "application/json")
			if tt.signature != "" {
				request.Header.Set("X-Razorpay-Signature", tt.signature)
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

func TestPaymentHandlerRazorpayVerifyStatusCodes(t *testing.T) {
	fullLink := &models.UserPatient{UserID: 7, PatientID: 3, PatientRelationship: models.RelationshipChild, AccessLevel: models.AccessLevelFull, Status: models.UserPatientStatusActive}
	body := func(paymentID string, signature string) string {
		return fmt.Sprintf(`{"providerPaymentId":%q,"signature":%q}`, paymentID, signature)
	}

	tests := []struct {
		name        string
		body        string
		wantStatus  int
		wantPayment models.PaymentStatus
	}{
		{name: "checkout's signature over the stored order captures", body: body("pay_1", hexHMAC(razorpayKeySecret, "order_1|pay_1")), wantStatus: http.StatusOK, wantPayment: models.PaymentStatusCaptured},
		{name: "a signature over another order is refused", body: body("pay_1", hexHMAC(razorpayKeySecret, "order_2|pay_1")), wantStatus: http.StatusUnprocessableEntity, wantPayment: models.PaymentStatusCreated},
		{name: "a signature made with the webhook secret is refused", body: body("pay_1", hexHMAC(razorpayWebhookSecret, "order_1|pay_1")), wantStatus: http.StatusUnprocessableEntity, wantPayment: models.PaymentStatusCreated},
		{name: "no signature", body: `{"providerPaymentId":"pay_1"}`, wantStatus: http.StatusBadRequest, wantPayment: models.PaymentStatusCreated},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			payments := &fakePaymentRepo{payment: razorpayPayment(models.PaymentStatusCreated)}
			router := newPaymentRouterWithGateway(t, &fakeAppointmentRepo{byID: razorpayBooking()}, &fakeUserPatientRepo{userPatient: fullLink}, payments, newRazorpayGateway(t))

			request := httptest.NewRequest(http.MethodPost, "/api/v1/payments/70/verify", strings.NewReader(tt.body))
			request.Header.Set("Content-Type", "application/json")
			request.Header.Set("Authorization", "Bearer patient-token")
			recorder := httptest.NewRecorder()

			router.ServeHTTP(recorder, request)

			if recorder.Code != tt.wantStatus {
				t.Fatalf("status = %d, want %d, body = %s", recorder.Code, tt.wantStatus, recorder.Body.String())
			}
			if payments.payment.Status != tt.wantPayment {
				t.Fatalf("payment status = %q, want %q", payments.payment.Status, tt.wantPayment)
			}
		})
	}
}

func TestAdminRefundStatusCodes(t *testing.T) {
	requested := []*models.Refund{{ID: 90, PaymentID: 70, Amount: 50000, Reason: models.RefundReasonDoctorCancelled, Status: models.RefundStatusRequested}}

	tests := []struct {
		name       string
		method     string
		path       string
		token      string
		refunds    []*models.Refund
		razorpay   bool
		wantStatus int
		wantBody   string
	}{
		{name: "admin lists refunds still with the gateway", method: http.MethodGet, path: "/api/v1/admin/refunds", token: "admin-token", refunds: requested, wantStatus: http.StatusOK, wantBody: `"refunds":[{"id":90`},
		{name: "an unknown status", method: http.MethodGet, path: "/api/v1/admin/refunds?status=pending", token: "admin-token", wantStatus: http.StatusBadRequest, wantBody: `"details":"status must be one of requested, succeeded, failed"`},
		{name: "patient cannot list refunds", method: http.MethodGet, path: "/api/v1/admin/refunds", token: "patient-token", wantStatus: http.StatusForbidden},
		{name: "a razorpay refund is accepted while it is processed", method: http.MethodPost, path: "/api/v1/admin/payments/70/refund", token: "admin-token", razorpay: true, wantStatus: http.StatusAccepted, wantBody: `"refundPending":true`},
		{name: "the manual complete route is gone", method: http.MethodPost, path: "/api/v1/admin/refunds/90/complete", token: "admin-token", refunds: requested, wantStatus: http.StatusNotFound},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			payments := &fakePaymentRepo{payment: razorpayPayment(models.PaymentStatusCaptured), refunds: tt.refunds}
			router := newDoctorAdminRouterWithPayments(t, &fakeDoctorRepo{}, &fakeAvailabilityRepo{}, payments)
			if tt.razorpay {
				router = newDoctorAdminRouterWithGateway(t, &fakeDoctorRepo{}, &fakeAvailabilityRepo{}, payments, &fakeMedicalRecordRepo{}, newRazorpayGateway(t))
			}

			request := httptest.NewRequest(tt.method, tt.path, nil)
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
