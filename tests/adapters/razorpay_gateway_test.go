package adapters_test

import (
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"testing"
	"time"

	"appointment_system/internal/apps/appointment/adapters/payment/razorpay"
	"appointment_system/internal/apps/appointment/apperrors"
	"appointment_system/internal/apps/appointment/models"
)

const (
	testKeySecret     = "razorpay-key-secret"
	testWebhookSecret = "razorpay-webhook-secret-at-least-32-chars"
)

// fakeRazorpay answers each "METHOD /path" with a canned JSON body and records
// the requests it saw.
type fakeRazorpay struct {
	routes   map[string]string
	requests []*http.Request
	bodies   []map[string]any
}

func (f *fakeRazorpay) gateway(t *testing.T) *razorpay.Gateway {
	t.Helper()

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body := map[string]any{}
		if raw, _ := io.ReadAll(r.Body); len(raw) > 0 {
			json.Unmarshal(raw, &body)
		}
		f.requests = append(f.requests, r)
		f.bodies = append(f.bodies, body)

		answer, ok := f.routes[r.Method+" "+r.URL.Path]
		if !ok {
			w.WriteHeader(http.StatusBadRequest)
			w.Write([]byte(`{"error":{"code":"BAD_REQUEST_ERROR","description":"no such route"}}`))
			return
		}
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(answer))
	}))
	t.Cleanup(server.Close)

	gateway, err := razorpay.New("rzp_test_key", testKeySecret, testWebhookSecret)
	if err != nil {
		t.Fatalf("razorpay.New returned error: %v", err)
	}
	return gateway.WithBaseURL(server.URL)
}

func sign(secret string, message string) string {
	mac := hmac.New(sha256.New, []byte(secret))
	mac.Write([]byte(message))
	return hex.EncodeToString(mac.Sum(nil))
}

func openOrder() *models.Payment {
	return &models.Payment{ID: 70, Amount: 50000, Currency: "INR", ProviderOrderID: "order_1", ProviderPaymentID: "pay_1"}
}

func TestRazorpayNewNeedsEverySecret(t *testing.T) {
	if _, err := razorpay.New("rzp_test_key", "", testWebhookSecret); err == nil {
		t.Fatal("New without a key secret returned no error")
	}
}

func TestRazorpayCreateOrder(t *testing.T) {
	fake := &fakeRazorpay{routes: map[string]string{"POST /v1/orders": `{"id":"order_1","amount":50000,"currency":"INR","status":"created"}`}}

	orderID, err := fake.gateway(t).CreateOrder(context.Background(), 50, 50000, "INR")
	if err != nil {
		t.Fatalf("CreateOrder returned error: %v", err)
	}
	if orderID != "order_1" {
		t.Fatalf("CreateOrder = %q, want order_1", orderID)
	}

	sent := fake.bodies[0]
	receipt, _ := sent["receipt"].(string)
	if sent["amount"] != float64(50000) || sent["currency"] != "INR" || !strings.HasPrefix(receipt, "apt_50_") || len(receipt) > 40 {
		t.Fatalf("CreateOrder sent %+v, want 50000 paise in INR with an apt_50_ receipt of at most 40 characters", sent)
	}
	if user, password, ok := fake.requests[0].BasicAuth(); !ok || user != "rzp_test_key" || password != testKeySecret {
		t.Fatal("CreateOrder did not authenticate with the key id and secret")
	}
}

func TestRazorpayClientPayloadKeepsTheSecret(t *testing.T) {
	payload := (&fakeRazorpay{}).gateway(t).ClientPayload(openOrder())

	if payload["keyId"] != "rzp_test_key" || payload["orderId"] != "order_1" || payload["amount"] != 50000 {
		t.Fatalf("ClientPayload = %+v, want the key id, order and amount", payload)
	}
	for key, value := range payload {
		if text, ok := value.(string); ok && (strings.Contains(text, testKeySecret) || strings.Contains(text, testWebhookSecret)) {
			t.Fatalf("ClientPayload leaks a secret in %q", key)
		}
	}
}

func TestRazorpayVerifyPayment(t *testing.T) {
	paidAt := time.Date(2026, 10, 25, 11, 58, 0, 0, time.UTC)
	payment := func(orderID string, amount string, status string) string {
		return `{"id":"pay_1","order_id":"` + orderID + `","amount":` + amount + `,"currency":"INR","status":"` + status + `","created_at":` + strconv.FormatInt(paidAt.Unix(), 10) + `}`
	}
	goodSignature := sign(testKeySecret, "order_1|pay_1")

	tests := []struct {
		name        string
		routes      map[string]string
		signature   string
		wantErr     error
		wantCapture bool
	}{
		{name: "a captured payment of the order", routes: map[string]string{"GET /v1/payments/pay_1": payment("order_1", "50000", "captured")}, signature: goodSignature},
		{name: "the server's own check needs no signature", routes: map[string]string{"GET /v1/payments/pay_1": payment("order_1", "50000", "captured")}},
		{name: "an authorized payment is captured", routes: map[string]string{"GET /v1/payments/pay_1": payment("order_1", "50000", "authorized"), "POST /v1/payments/pay_1/capture": payment("order_1", "50000", "captured")}, signature: goodSignature, wantCapture: true},
		{name: "a signature over another order", routes: map[string]string{"GET /v1/payments/pay_1": payment("order_1", "50000", "captured")}, signature: sign(testKeySecret, "order_2|pay_1"), wantErr: apperrors.ErrPaymentVerificationFailed},
		{name: "a signature that is not hex", routes: map[string]string{"GET /v1/payments/pay_1": payment("order_1", "50000", "captured")}, signature: "not-hex", wantErr: apperrors.ErrPaymentVerificationFailed},
		{name: "a payment of another order", routes: map[string]string{"GET /v1/payments/pay_1": payment("order_2", "50000", "captured")}, wantErr: apperrors.ErrPaymentVerificationFailed},
		{name: "a payment of part of the fee", routes: map[string]string{"GET /v1/payments/pay_1": payment("order_1", "100", "captured")}, wantErr: apperrors.ErrPaymentVerificationFailed},
		{name: "a failed payment", routes: map[string]string{"GET /v1/payments/pay_1": payment("order_1", "50000", "failed")}, wantErr: apperrors.ErrPaymentVerificationFailed},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			fake := &fakeRazorpay{routes: tt.routes}

			at, err := fake.gateway(t).VerifyPayment(context.Background(), openOrder(), "pay_1", tt.signature)
			if tt.wantErr != nil {
				if !errors.Is(err, tt.wantErr) {
					t.Fatalf("VerifyPayment error = %v, want %v", err, tt.wantErr)
				}
				return
			}
			if err != nil {
				t.Fatalf("VerifyPayment returned error: %v", err)
			}
			if !at.Equal(paidAt) {
				t.Fatalf("VerifyPayment paid at %v, want %v", at, paidAt)
			}
			captured := len(fake.requests) == 2 && fake.requests[1].URL.Path == "/v1/payments/pay_1/capture" && fake.bodies[1]["amount"] == float64(50000)
			if captured != tt.wantCapture {
				t.Fatalf("VerifyPayment captured = %v, want %v", captured, tt.wantCapture)
			}
		})
	}
}

func TestRazorpayFindPaidOrder(t *testing.T) {
	fake := &fakeRazorpay{routes: map[string]string{"GET /v1/orders/order_1/payments": `{"items":[
		{"id":"pay_0","order_id":"order_1","amount":50000,"status":"failed"},
		{"id":"pay_1","order_id":"order_1","amount":50000,"status":"captured"}]}`}}

	paymentID, err := fake.gateway(t).FindPaidOrder(context.Background(), openOrder())
	if err != nil || paymentID != "pay_1" {
		t.Fatalf("FindPaidOrder = %q, %v; want pay_1", paymentID, err)
	}

	unpaid := &fakeRazorpay{routes: map[string]string{"GET /v1/orders/order_1/payments": `{"items":[{"id":"pay_0","amount":50000,"status":"failed"}]}`}}
	if paymentID, err := unpaid.gateway(t).FindPaidOrder(context.Background(), openOrder()); err != nil || paymentID != "" {
		t.Fatalf("FindPaidOrder of an unpaid order = %q, %v; want nothing", paymentID, err)
	}
}

func TestRazorpayRefund(t *testing.T) {
	tests := []struct {
		name       string
		answer     string
		wantStatus models.RefundStatus
	}{
		{name: "a pending refund is still requested", answer: `{"id":"rfnd_1","payment_id":"pay_1","amount":50000,"status":"pending"}`, wantStatus: models.RefundStatusRequested},
		{name: "a processed refund succeeded", answer: `{"id":"rfnd_1","payment_id":"pay_1","amount":50000,"status":"processed"}`, wantStatus: models.RefundStatusSucceeded},
		{name: "a failed refund failed", answer: `{"id":"rfnd_1","payment_id":"pay_1","amount":50000,"status":"failed"}`, wantStatus: models.RefundStatusFailed},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			fake := &fakeRazorpay{routes: map[string]string{"POST /v1/payments/pay_1/refund": tt.answer}}

			refundID, status, err := fake.gateway(t).Refund(context.Background(), openOrder(), &models.Refund{ID: 90, Amount: 50000})
			if err != nil {
				t.Fatalf("Refund returned error: %v", err)
			}
			if refundID != "rfnd_1" || status != tt.wantStatus {
				t.Fatalf("Refund = %q, %q; want rfnd_1, %q", refundID, status, tt.wantStatus)
			}
			sent := fake.bodies[0]
			if sent["amount"] != float64(50000) || sent["speed"] != "normal" || sent["receipt"] != "refund_90" {
				t.Fatalf("Refund sent %+v, want 50000 paise at normal speed with receipt refund_90", sent)
			}
			if key := fake.requests[0].Header.Get("X-Refund-Idempotency"); key != "hc_refund_90" {
				t.Fatalf("Refund idempotency key = %q, want hc_refund_90", key)
			}
		})
	}

	t.Run("an API error leaves the outcome unknown", func(t *testing.T) {
		if _, _, err := (&fakeRazorpay{}).gateway(t).Refund(context.Background(), openOrder(), &models.Refund{ID: 90, Amount: 50000}); err == nil {
			t.Fatal("Refund returned no error for an API error")
		}
	})
}

func TestRazorpayRefundStatus(t *testing.T) {
	fake := &fakeRazorpay{routes: map[string]string{"GET /v1/payments/pay_1/refunds": `{"items":[
		{"id":"rfnd_0","payment_id":"pay_1","amount":50000,"status":"failed"},
		{"id":"rfnd_1","payment_id":"pay_1","amount":50000,"status":"processed"},
		{"id":"rfnd_2","payment_id":"pay_1","amount":50000,"status":"pending"}]}`}}

	refundID, status, err := fake.gateway(t).RefundStatus(context.Background(), openOrder())
	if err != nil || refundID != "rfnd_1" || status != models.RefundStatusSucceeded {
		t.Fatalf("RefundStatus = %q, %q, %v; want the processed rfnd_1", refundID, status, err)
	}

	none := &fakeRazorpay{routes: map[string]string{"GET /v1/payments/pay_1/refunds": `{"items":[]}`}}
	if refundID, _, err := none.gateway(t).RefundStatus(context.Background(), openOrder()); err != nil || refundID != "" {
		t.Fatalf("RefundStatus with no refunds = %q, %v; want nothing", refundID, err)
	}
}

func TestRazorpayParseWebhook(t *testing.T) {
	signed := func(body string) map[string][]string {
		return map[string][]string{"X-Razorpay-Signature": {sign(testWebhookSecret, body)}}
	}
	captured := `{"event":"payment.captured","payload":{"payment":{"entity":{"id":"pay_1","order_id":"order_1","amount":50000,"status":"captured"}}}}`
	orderPaid := `{"event":"order.paid","payload":{"payment":{"entity":{"id":"pay_1","order_id":"order_1","amount":50000,"status":"captured"}},"order":{"entity":{"id":"order_1"}}}}`
	refundFailed := `{"event":"refund.failed","payload":{"refund":{"entity":{"id":"rfnd_1","payment_id":"pay_1","amount":50000,"status":"failed"}}}}`
	paymentFailed := `{"event":"payment.failed","payload":{"payment":{"entity":{"id":"pay_2","order_id":"order_1","amount":50000,"status":"failed"}}}}`

	tests := []struct {
		name      string
		body      string
		headers   map[string][]string
		wantErr   error
		wantEvent models.GatewayEvent
	}{
		{name: "payment.captured", body: captured, headers: signed(captured), wantEvent: models.GatewayEvent{Type: models.GatewayEventPaymentCaptured, ProviderOrderID: "order_1", ProviderPaymentID: "pay_1", Amount: 50000}},
		{name: "order.paid", body: orderPaid, headers: signed(orderPaid), wantEvent: models.GatewayEvent{Type: models.GatewayEventPaymentCaptured, ProviderOrderID: "order_1", ProviderPaymentID: "pay_1", Amount: 50000}},
		{name: "refund.failed", body: refundFailed, headers: signed(refundFailed), wantEvent: models.GatewayEvent{Type: models.GatewayEventRefundFailed, ProviderPaymentID: "pay_1", ProviderRefundID: "rfnd_1", Amount: 50000}},
		{name: "payment.failed is ignored so the payer can retry", body: paymentFailed, headers: signed(paymentFailed), wantErr: apperrors.ErrWebhookIgnored},
		{name: "a signed body that is not an event is ignored", body: `not json`, headers: signed(`not json`), wantErr: apperrors.ErrWebhookIgnored},
		{name: "a forged signature", body: captured, headers: signed(captured + "x"), wantErr: apperrors.ErrPaymentVerificationFailed},
		{name: "no signature", body: captured, headers: map[string][]string{}, wantErr: apperrors.ErrPaymentVerificationFailed},
	}

	gateway := (&fakeRazorpay{}).gateway(t)
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			event, err := gateway.ParseWebhook([]byte(tt.body), tt.headers)
			if tt.wantErr != nil {
				if !errors.Is(err, tt.wantErr) {
					t.Fatalf("ParseWebhook error = %v, want %v", err, tt.wantErr)
				}
				return
			}
			if err != nil {
				t.Fatalf("ParseWebhook returned error: %v", err)
			}
			if *event != tt.wantEvent {
				t.Fatalf("ParseWebhook = %+v, want %+v", *event, tt.wantEvent)
			}
		})
	}
}
