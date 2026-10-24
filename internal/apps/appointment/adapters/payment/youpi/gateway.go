// Package youpi collects UPI payments through a self-hosted YouPI PG server
// (github.com/youpi-pg/YouPI-PG). Money goes straight to the merchant's own UPI
// account and YouPI confirms it from bank alerts, so it has no refund API:
// Refund returns apperrors.ErrRefundManual and an admin pays it back by hand.
package youpi

import (
	"bytes"
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"

	"appointment_system/internal/apps/appointment/apperrors"
	"appointment_system/internal/apps/appointment/models"
)

const (
	ProviderName = "youpi"

	requestTimeout   = 10 * time.Second
	maxResponseBytes = 1 << 20
	orderPrefix      = "apt_"

	merchantIDHeader  = "X-Merchant-Id"
	merchantKeyHeader = "X-Merchant-Key"
	idempotencyHeader = "Idempotency-Key"
	signatureHeader   = "X-Gateway-Signature"
	signaturePrefix   = "sha256="

	statusSuccess   = "SUCCESS"
	statusFailed    = "FAILED"
	statusExpired   = "EXPIRED"
	statusCancelled = "CANCELLED"
)

type Gateway struct {
	baseURL     string
	merchantID  string
	merchantKey string
	returnURL   string
	client      *http.Client
}

// New takes the public URL of the YouPI checkout portal, which also serves its
// API, the merchant's id, the merchant's webhook secret (YouPI uses it as the
// merchant key too) and the page YouPI sends the payer back to.
func New(baseURL string, merchantID string, merchantKey string, returnURL string) (*Gateway, error) {
	if strings.TrimSpace(baseURL) == "" || strings.TrimSpace(merchantID) == "" || strings.TrimSpace(merchantKey) == "" || strings.TrimSpace(returnURL) == "" {
		return nil, errors.New("youpi base url, merchant id, merchant key and return url are required")
	}

	return &Gateway{
		baseURL:     strings.TrimRight(baseURL, "/"),
		merchantID:  merchantID,
		merchantKey: merchantKey,
		returnURL:   returnURL,
		client:      &http.Client{Timeout: requestTimeout},
	}, nil
}

type createPaymentRequest struct {
	OrderID   string         `json:"orderId"`
	Amount    string         `json:"amount"`
	Currency  string         `json:"currency"`
	ReturnURL string         `json:"returnUrl"`
	Metadata  map[string]any `json:"metadata"`
}

type paymentResponse struct {
	PaymentID   string      `json:"paymentId"`
	OrderID     string      `json:"orderId"`
	AmountPaise json.Number `json:"amountPaise"`
	Status      string      `json:"status"`
}

type webhookPayload struct {
	Event       string      `json:"event"`
	PaymentID   string      `json:"paymentId"`
	AmountPaise json.Number `json:"amountPaise"`
	UTR         string      `json:"utr"`
}

func (g *Gateway) Provider() string {
	return ProviderName
}

// CreateOrder opens a YouPI payment session and returns its id, which is also
// the path of its checkout page.
func (g *Gateway) CreateOrder(ctx context.Context, appointmentID uint64, amount int, currency string) (string, error) {
	orderID := fmt.Sprintf("%s%d_%d", orderPrefix, appointmentID, time.Now().UnixNano())
	request := createPaymentRequest{
		OrderID:   orderID,
		Amount:    fmt.Sprintf("%d.%02d", amount/100, amount%100),
		Currency:  currency,
		ReturnURL: g.returnURL,
		Metadata:  map[string]any{"appointmentId": appointmentID},
	}

	var created paymentResponse
	statusCode, err := g.send(ctx, http.MethodPost, "/api/v1/payments", request, orderID, &created)
	if err != nil {
		return "", fmt.Errorf("create youpi payment: %w", err)
	}
	if statusCode != http.StatusOK && statusCode != http.StatusCreated {
		return "", fmt.Errorf("create youpi payment: status %d", statusCode)
	}
	if created.PaymentID == "" {
		return "", errors.New("create youpi payment: response has no paymentId")
	}

	return created.PaymentID, nil
}

func (g *Gateway) ClientPayload(payment *models.Payment) map[string]any {
	return map[string]any{
		"orderId":     payment.ProviderOrderID,
		"amount":      payment.Amount,
		"currency":    payment.Currency,
		"checkoutUrl": g.baseURL + "/checkout/" + url.PathEscape(payment.ProviderOrderID),
	}
}

// VerifyPayment asks YouPI itself whether the session was paid in full rather
// than trusting the client. YouPI signs nothing for the client, so signature is
// not used.
func (g *Gateway) VerifyPayment(ctx context.Context, payment *models.Payment, providerPaymentID string, signature string) error {
	var session paymentResponse
	statusCode, err := g.send(ctx, http.MethodGet, "/api/v1/payments/"+url.PathEscape(payment.ProviderOrderID), nil, "", &session)
	if err != nil {
		return fmt.Errorf("get youpi payment: %w", err)
	}
	if statusCode == http.StatusNotFound {
		return apperrors.ErrPaymentVerificationFailed
	}
	if statusCode != http.StatusOK {
		return fmt.Errorf("get youpi payment: status %d", statusCode)
	}

	amount, err := strconv.Atoi(session.AmountPaise.String())
	if err != nil || session.Status != statusSuccess || session.PaymentID != payment.ProviderOrderID || amount != payment.Amount {
		return apperrors.ErrPaymentVerificationFailed
	}

	return nil
}

func (g *Gateway) Refund(ctx context.Context, payment *models.Payment, amount int) (string, error) {
	return "", apperrors.ErrRefundManual
}

// CancelOrder closes the checkout session. A session YouPI no longer knows
// counts as closed.
func (g *Gateway) CancelOrder(ctx context.Context, payment *models.Payment) error {
	statusCode, err := g.send(ctx, http.MethodPost, "/api/v1/checkout/"+url.PathEscape(payment.ProviderOrderID)+"/cancel", nil, "", nil)
	if err != nil {
		return fmt.Errorf("cancel youpi payment: %w", err)
	}
	if statusCode != http.StatusOK && statusCode != http.StatusNotFound {
		return fmt.Errorf("cancel youpi payment: status %d", statusCode)
	}

	return nil
}

// ParseWebhook checks X-Gateway-Signature, "sha256=" and the hex HMAC-SHA256 of
// the raw body keyed with the merchant's webhook secret. YouPI does not sign
// its timestamp, so a webhook can be replayed; the service re-checks every
// capture with VerifyPayment and a replay changes nothing. The bank's UTR is
// the payment id, falling back to the session id when YouPI has none.
func (g *Gateway) ParseWebhook(body []byte, headers map[string][]string) (*models.GatewayEvent, error) {
	presented, found := strings.CutPrefix(http.Header(headers).Get(signatureHeader), signaturePrefix)
	if !found {
		return nil, apperrors.ErrPaymentVerificationFailed
	}
	signature, err := hex.DecodeString(presented)
	if err != nil {
		return nil, apperrors.ErrPaymentVerificationFailed
	}
	mac := hmac.New(sha256.New, []byte(g.merchantKey))
	mac.Write(body)
	if !hmac.Equal(signature, mac.Sum(nil)) {
		return nil, apperrors.ErrPaymentVerificationFailed
	}

	var payload webhookPayload
	if err := json.Unmarshal(body, &payload); err != nil {
		return nil, fmt.Errorf("decode youpi webhook: %w", err)
	}
	amount, err := strconv.Atoi(payload.AmountPaise.String())
	if err != nil || payload.PaymentID == "" {
		return nil, errors.New("decode youpi webhook: paymentId or amountPaise is missing")
	}

	event := &models.GatewayEvent{
		ProviderOrderID:   payload.PaymentID,
		ProviderPaymentID: strings.TrimSpace(payload.UTR),
		Amount:            amount,
	}
	if event.ProviderPaymentID == "" {
		event.ProviderPaymentID = payload.PaymentID
	}
	switch payload.Event {
	case statusSuccess:
		event.Status = models.PaymentStatusCaptured
	case statusFailed:
		event.Status = models.PaymentStatusFailed
	case statusExpired, statusCancelled:
		event.Status = models.PaymentStatusExpired
	}

	return event, nil
}

// send calls YouPI with the merchant's credentials and decodes a 2xx JSON
// answer into out. It returns the status code; only transport and decoding
// failures are errors, so callers decide what each status means.
func (g *Gateway) send(ctx context.Context, method string, path string, body any, idempotencyKey string, out any) (int, error) {
	var content io.Reader
	if body != nil {
		encoded, err := json.Marshal(body)
		if err != nil {
			return 0, fmt.Errorf("encode request: %w", err)
		}
		content = bytes.NewReader(encoded)
	}

	request, err := http.NewRequestWithContext(ctx, method, g.baseURL+path, content)
	if err != nil {
		return 0, fmt.Errorf("build request: %w", err)
	}
	request.Header.Set("Accept", "application/json")
	request.Header.Set(merchantIDHeader, g.merchantID)
	request.Header.Set(merchantKeyHeader, g.merchantKey)
	if body != nil {
		request.Header.Set("Content-Type", "application/json")
	}
	if idempotencyKey != "" {
		request.Header.Set(idempotencyHeader, idempotencyKey)
	}

	response, err := g.client.Do(request)
	if err != nil {
		return 0, fmt.Errorf("send request: %w", err)
	}
	defer response.Body.Close()

	if out != nil && response.StatusCode >= http.StatusOK && response.StatusCode < http.StatusMultipleChoices {
		if err := json.NewDecoder(io.LimitReader(response.Body, maxResponseBytes)).Decode(out); err != nil {
			return response.StatusCode, fmt.Errorf("decode response: %w", err)
		}
	}

	return response.StatusCode, nil
}
