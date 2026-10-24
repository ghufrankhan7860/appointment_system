package adapters_test

import (
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"appointment_system/internal/apps/appointment/adapters/payment/youpi"
	"appointment_system/internal/apps/appointment/apperrors"
	"appointment_system/internal/apps/appointment/models"
)

const youpiMerchantKey = "merchant-webhook-secret-at-least-32-chars"

func newYouPIGateway(t *testing.T, handler http.HandlerFunc) (*youpi.Gateway, *httptest.Server) {
	t.Helper()

	server := httptest.NewServer(handler)
	t.Cleanup(server.Close)
	gateway, err := youpi.New(server.URL+"/", "merchant-1", youpiMerchantKey, "https://app.example.com/payments/return")
	if err != nil {
		t.Fatalf("New returned error: %v", err)
	}

	return gateway, server
}

func signYouPI(body string) string {
	mac := hmac.New(sha256.New, []byte(youpiMerchantKey))
	mac.Write([]byte(body))
	return "sha256=" + hex.EncodeToString(mac.Sum(nil))
}

func TestYouPIGatewayNewRequiresSettings(t *testing.T) {
	if _, err := youpi.New("http://localhost:7000", "merchant-1", "", "https://app.example.com/return"); err == nil {
		t.Fatalf("New with no merchant key returned no error")
	}
}

func TestYouPIGatewayCreateOrder(t *testing.T) {
	var received map[string]any
	gateway, server := newYouPIGateway(t, func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost || r.URL.Path != "/api/v1/payments" {
			t.Errorf("CreateOrder called %s %s, want POST /api/v1/payments", r.Method, r.URL.Path)
		}
		if r.Header.Get("X-Merchant-Id") != "merchant-1" || r.Header.Get("X-Merchant-Key") != youpiMerchantKey {
			t.Errorf("CreateOrder sent merchant headers %q and %q", r.Header.Get("X-Merchant-Id"), r.Header.Get("X-Merchant-Key"))
		}
		if err := json.NewDecoder(r.Body).Decode(&received); err != nil {
			t.Errorf("CreateOrder body is not JSON: %v", err)
		}
		if r.Header.Get("Idempotency-Key") != received["orderId"] {
			t.Errorf("CreateOrder idempotency key = %q, want the order id %v", r.Header.Get("Idempotency-Key"), received["orderId"])
		}
		w.WriteHeader(http.StatusCreated)
		w.Write([]byte(`{"paymentId":"sess-1","status":"INITIATED","amountPaise":"40050"}`))
	})

	orderID, err := gateway.CreateOrder(context.Background(), 50, 40050, "INR")
	if err != nil {
		t.Fatalf("CreateOrder returned error: %v", err)
	}
	if orderID != "sess-1" {
		t.Fatalf("CreateOrder = %q, want the session id sess-1", orderID)
	}
	if received["amount"] != "400.50" || received["currency"] != "INR" || received["returnUrl"] != "https://app.example.com/payments/return" || !strings.HasPrefix(received["orderId"].(string), "apt_50_") {
		t.Fatalf("CreateOrder sent %v, want 400.50 INR for appointment 50 with the return url", received)
	}

	payload := gateway.ClientPayload(&models.Payment{ProviderOrderID: orderID, Amount: 40050, Currency: "INR"})
	if payload["checkoutUrl"] != server.URL+"/checkout/sess-1" {
		t.Fatalf("ClientPayload checkoutUrl = %v, want %s/checkout/sess-1", payload["checkoutUrl"], server.URL)
	}
}

func TestYouPIGatewayCreateOrderRefused(t *testing.T) {
	gateway, _ := newYouPIGateway(t, func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusConflict)
		w.Write([]byte(`{"error":"DUPLICATE_SESSION_DETECTED"}`))
	})

	if _, err := gateway.CreateOrder(context.Background(), 50, 40000, "INR"); err == nil {
		t.Fatalf("CreateOrder on a 409 returned no error")
	}
}

func TestYouPIGatewayVerifyPayment(t *testing.T) {
	payment := &models.Payment{ProviderOrderID: "sess-1", Amount: 40000}

	tests := []struct {
		name       string
		statusCode int
		body       string
		wantErr    error
		wantOther  bool
	}{
		{name: "a session paid in full", statusCode: http.StatusOK, body: `{"paymentId":"sess-1","status":"SUCCESS","amountPaise":"40000"}`},
		{name: "a numeric amount is read too", statusCode: http.StatusOK, body: `{"paymentId":"sess-1","status":"SUCCESS","amountPaise":40000}`},
		{name: "a session still waiting", statusCode: http.StatusOK, body: `{"paymentId":"sess-1","status":"PROCESSING","amountPaise":"40000"}`, wantErr: apperrors.ErrPaymentVerificationFailed},
		{name: "a different amount", statusCode: http.StatusOK, body: `{"paymentId":"sess-1","status":"SUCCESS","amountPaise":"100"}`, wantErr: apperrors.ErrPaymentVerificationFailed},
		{name: "another session", statusCode: http.StatusOK, body: `{"paymentId":"sess-2","status":"SUCCESS","amountPaise":"40000"}`, wantErr: apperrors.ErrPaymentVerificationFailed},
		{name: "an unknown session", statusCode: http.StatusNotFound, body: `{}`, wantErr: apperrors.ErrPaymentVerificationFailed},
		{name: "youpi failing is not a verification failure", statusCode: http.StatusInternalServerError, body: `{}`, wantOther: true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			gateway, _ := newYouPIGateway(t, func(w http.ResponseWriter, r *http.Request) {
				if r.Method != http.MethodGet || r.URL.Path != "/api/v1/payments/sess-1" || r.Header.Get("X-Merchant-Key") != youpiMerchantKey {
					t.Errorf("VerifyPayment called %s %s with key %q", r.Method, r.URL.Path, r.Header.Get("X-Merchant-Key"))
				}
				w.WriteHeader(tt.statusCode)
				w.Write([]byte(tt.body))
			})

			err := gateway.VerifyPayment(context.Background(), payment, "412345678901", "")
			switch {
			case tt.wantOther:
				if err == nil || errors.Is(err, apperrors.ErrPaymentVerificationFailed) {
					t.Fatalf("VerifyPayment error = %v, want a gateway failure", err)
				}
			case tt.wantErr != nil:
				if !errors.Is(err, tt.wantErr) {
					t.Fatalf("VerifyPayment error = %v, want %v", err, tt.wantErr)
				}
			case err != nil:
				t.Fatalf("VerifyPayment returned error: %v", err)
			}
		})
	}
}

func TestYouPIGatewayParseWebhook(t *testing.T) {
	gateway, _ := newYouPIGateway(t, func(w http.ResponseWriter, r *http.Request) {})
	success := `{"event":"SUCCESS","paymentId":"sess-1","orderId":"apt_50_1","amount":"400.00","amountPaise":40000,"utr":"412345678901"}`
	noUTR := `{"event":"SUCCESS","paymentId":"sess-1","amountPaise":40000,"utr":""}`

	tests := []struct {
		name          string
		body          string
		signature     string
		wantErr       error
		wantStatus    models.PaymentStatus
		wantPaymentID string
	}{
		{name: "a signed success", body: success, signature: signYouPI(success), wantStatus: models.PaymentStatusCaptured, wantPaymentID: "412345678901"},
		{name: "no utr falls back to the session id", body: noUTR, signature: signYouPI(noUTR), wantStatus: models.PaymentStatusCaptured, wantPaymentID: "sess-1"},
		{name: "a failed payment", body: `{"event":"FAILED","paymentId":"sess-1","amountPaise":40000}`, signature: signYouPI(`{"event":"FAILED","paymentId":"sess-1","amountPaise":40000}`), wantStatus: models.PaymentStatusFailed, wantPaymentID: "sess-1"},
		{name: "a cancelled checkout expires the order", body: `{"event":"CANCELLED","paymentId":"sess-1","amountPaise":40000}`, signature: signYouPI(`{"event":"CANCELLED","paymentId":"sess-1","amountPaise":40000}`), wantStatus: models.PaymentStatusExpired, wantPaymentID: "sess-1"},
		{name: "a tampered body", body: strings.Replace(success, "40000", "1", 1), signature: signYouPI(success), wantErr: apperrors.ErrPaymentVerificationFailed},
		{name: "a signature without its prefix", body: success, signature: strings.TrimPrefix(signYouPI(success), "sha256="), wantErr: apperrors.ErrPaymentVerificationFailed},
		{name: "a signature that is not hex", body: success, signature: "sha256=not-hex", wantErr: apperrors.ErrPaymentVerificationFailed},
		{name: "no signature", body: success, wantErr: apperrors.ErrPaymentVerificationFailed},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			headers := http.Header{}
			if tt.signature != "" {
				headers.Set("X-Gateway-Signature", tt.signature)
			}

			event, err := gateway.ParseWebhook([]byte(tt.body), headers)
			if tt.wantErr != nil {
				if !errors.Is(err, tt.wantErr) {
					t.Fatalf("ParseWebhook error = %v, want %v", err, tt.wantErr)
				}
				return
			}
			if err != nil {
				t.Fatalf("ParseWebhook returned error: %v", err)
			}
			if event.ProviderOrderID != "sess-1" || event.Amount != 40000 || event.Status != tt.wantStatus || event.ProviderPaymentID != tt.wantPaymentID {
				t.Fatalf("ParseWebhook = %+v, want sess-1 for 40000 %q paid by %q", event, tt.wantStatus, tt.wantPaymentID)
			}
		})
	}
}

func TestYouPIGatewayCancelOrder(t *testing.T) {
	tests := []struct {
		name       string
		statusCode int
		wantErr    bool
	}{
		{name: "an open session is cancelled", statusCode: http.StatusOK},
		{name: "a session youpi no longer knows counts as closed", statusCode: http.StatusNotFound},
		{name: "youpi failing is an error", statusCode: http.StatusInternalServerError, wantErr: true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			gateway, _ := newYouPIGateway(t, func(w http.ResponseWriter, r *http.Request) {
				if r.Method != http.MethodPost || r.URL.Path != "/api/v1/checkout/sess-1/cancel" {
					t.Errorf("CancelOrder called %s %s", r.Method, r.URL.Path)
				}
				w.WriteHeader(tt.statusCode)
				w.Write([]byte(`{"success":true,"status":"CANCELLED"}`))
			})

			err := gateway.CancelOrder(context.Background(), &models.Payment{ProviderOrderID: "sess-1"})
			if tt.wantErr != (err != nil) {
				t.Fatalf("CancelOrder error = %v, want error %v", err, tt.wantErr)
			}
		})
	}
}

func TestYouPIGatewayRefundIsManual(t *testing.T) {
	gateway, _ := newYouPIGateway(t, func(w http.ResponseWriter, r *http.Request) {
		t.Errorf("Refund called YouPI at %s", r.URL.Path)
	})

	if _, err := gateway.Refund(context.Background(), &models.Payment{ProviderOrderID: "sess-1", Amount: 40000}, 40000); !errors.Is(err, apperrors.ErrRefundManual) {
		t.Fatalf("Refund error = %v, want %v", err, apperrors.ErrRefundManual)
	}
}
