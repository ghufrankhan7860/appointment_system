// Package razorpay collects payments through Razorpay Standard Checkout. The
// server opens an order, the browser pays it in Razorpay's checkout, and the
// server confirms the payment with Razorpay's API before any money is recorded.
// Refunds go through the API at normal speed and finish later, reported by
// webhook. Razorpay orders cannot be cancelled; money paid on an order the
// service already closed is captured and refunded.
package razorpay

import (
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"time"

	razorpaysdk "github.com/razorpay/razorpay-go"

	"appointment_system/internal/apps/appointment/apperrors"
	"appointment_system/internal/apps/appointment/models"
)

const (
	ProviderName = "razorpay"

	// requestTimeoutSeconds stays under the 5 seconds Razorpay gives a webhook
	// to answer, because the webhook confirms payments with the API.
	requestTimeoutSeconds = 4
	receiptPrefix         = "apt_"
	refundReceiptPrefix   = "refund_"
	// refundIdempotencyPrefix keys each refund request on our refund id, so a
	// retry after a timeout cannot refund twice.
	refundIdempotencyPrefix = "hc_refund_"
	refundSpeed             = "normal"

	signatureHeader         = "X-Razorpay-Signature"
	refundIdempotencyHeader = "X-Refund-Idempotency"

	paymentAuthorized = "authorized"
	paymentCaptured   = "captured"
	refundProcessed   = "processed"
	refundPending     = "pending"
	refundFailed      = "failed"

	eventPaymentCaptured = "payment.captured"
	eventOrderPaid       = "order.paid"
	eventRefundProcessed = "refund.processed"
	eventRefundFailed    = "refund.failed"
)

type Gateway struct {
	client        *razorpaysdk.Client
	keyID         string
	keySecret     string
	webhookSecret string
}

// New takes the account's key id and key secret and the secret the account's
// webhook is signed with.
func New(keyID string, keySecret string, webhookSecret string) (*Gateway, error) {
	if strings.TrimSpace(keyID) == "" || strings.TrimSpace(keySecret) == "" || strings.TrimSpace(webhookSecret) == "" {
		return nil, errors.New("razorpay key id, key secret and webhook secret are required")
	}

	client := razorpaysdk.NewClient(keyID, keySecret)
	client.SetTimeout(requestTimeoutSeconds)

	return &Gateway{
		client:        client,
		keyID:         keyID,
		keySecret:     keySecret,
		webhookSecret: webhookSecret,
	}, nil
}

// WithBaseURL points the gateway at another API host, such as a test server.
func (g *Gateway) WithBaseURL(baseURL string) *Gateway {
	g.client.Request.BaseURL = strings.TrimRight(baseURL, "/")
	return g
}

type orderEntity struct {
	ID string `json:"id"`
}

type paymentEntity struct {
	ID        string `json:"id"`
	OrderID   string `json:"order_id"`
	Amount    int    `json:"amount"`
	Currency  string `json:"currency"`
	Status    string `json:"status"`
	CreatedAt int64  `json:"created_at"`
}

type refundEntity struct {
	ID        string `json:"id"`
	PaymentID string `json:"payment_id"`
	Amount    int    `json:"amount"`
	Status    string `json:"status"`
}

type collection[T any] struct {
	Items []T `json:"items"`
}

type webhookPayload struct {
	Event   string `json:"event"`
	Payload struct {
		Payment struct {
			Entity paymentEntity `json:"entity"`
		} `json:"payment"`
		Refund struct {
			Entity refundEntity `json:"entity"`
		} `json:"refund"`
	} `json:"payload"`
}

func (g *Gateway) Provider() string {
	return ProviderName
}

func (g *Gateway) CreateOrder(ctx context.Context, appointmentID uint64, amount int, currency string) (string, error) {
	response, err := g.client.Order.Create(map[string]interface{}{
		"amount":   amount,
		"currency": currency,
		"receipt":  fmt.Sprintf("%s%d_%d", receiptPrefix, appointmentID, time.Now().Unix()),
		"notes":    map[string]interface{}{"appointment_id": strconv.FormatUint(appointmentID, 10)},
	}, nil)
	if err != nil {
		return "", fmt.Errorf("create razorpay order: %w", err)
	}

	var order orderEntity
	if err := decode(response, &order); err != nil {
		return "", fmt.Errorf("create razorpay order: %w", err)
	}
	if order.ID == "" {
		return "", errors.New("create razorpay order: response has no id")
	}

	return order.ID, nil
}

// ClientPayload is what Razorpay Checkout opens with. The key id is public; the
// key secret never leaves the server.
func (g *Gateway) ClientPayload(payment *models.Payment) map[string]any {
	return map[string]any{
		"keyId":    g.keyID,
		"orderId":  payment.ProviderOrderID,
		"amount":   payment.Amount,
		"currency": payment.Currency,
	}
}

// VerifyPayment checks checkout's signature over the stored order id, never one
// the client sends, then fetches the payment and requires it to belong to the
// order with the same amount and currency. An authorized payment is captured.
// Razorpay is the authority, so a payment the server learned of itself, with
// no signature, is checked the same way.
func (g *Gateway) VerifyPayment(ctx context.Context, payment *models.Payment, providerPaymentID string, signature string) (time.Time, error) {
	if signature != "" && !validSignature([]byte(payment.ProviderOrderID+"|"+providerPaymentID), signature, g.keySecret) {
		return time.Time{}, apperrors.Invalid(apperrors.ErrPaymentVerificationFailed, "signature", "does not match the order")
	}

	paid, err := g.fetchPayment(providerPaymentID)
	if err != nil {
		return time.Time{}, err
	}
	if paid.OrderID != payment.ProviderOrderID || paid.Amount != payment.Amount || paid.Currency != payment.Currency {
		return time.Time{}, apperrors.Invalid(apperrors.ErrPaymentVerificationFailed, "providerPaymentId", "does not pay this order in full")
	}

	switch paid.Status {
	case paymentCaptured:
	case paymentAuthorized:
		if _, err := g.client.Payment.Capture(providerPaymentID, payment.Amount, map[string]interface{}{"currency": payment.Currency}, nil); err != nil {
			// Razorpay's automatic capture may have won the race.
			again, fetchErr := g.fetchPayment(providerPaymentID)
			if fetchErr != nil || again.Status != paymentCaptured {
				return time.Time{}, fmt.Errorf("capture razorpay payment: %w", err)
			}
		}
	default:
		return time.Time{}, apperrors.Invalid(apperrors.ErrPaymentVerificationFailed, "", fmt.Sprintf("the payment is %s", paid.Status))
	}

	return time.Unix(paid.CreatedAt, 0), nil
}

// FindPaidOrder looks through the order's payments for one that paid it in
// full.
func (g *Gateway) FindPaidOrder(ctx context.Context, payment *models.Payment) (string, error) {
	response, err := g.client.Order.Payments(payment.ProviderOrderID, nil, nil)
	if err != nil {
		return "", fmt.Errorf("list razorpay order payments: %w", err)
	}

	var payments collection[paymentEntity]
	if err := decode(response, &payments); err != nil {
		return "", fmt.Errorf("list razorpay order payments: %w", err)
	}
	for _, paid := range payments.Items {
		if (paid.Status == paymentCaptured || paid.Status == paymentAuthorized) && paid.Amount == payment.Amount {
			return paid.ID, nil
		}
	}

	return "", nil
}

// Refund asks Razorpay for a normal-speed refund, keyed on the refund's id so
// that a retried request is not refunded twice.
func (g *Gateway) Refund(ctx context.Context, payment *models.Payment, refund *models.Refund) (string, models.RefundStatus, error) {
	response, err := g.client.Payment.Refund(
		payment.ProviderPaymentID,
		refund.Amount,
		map[string]interface{}{
			"speed":   refundSpeed,
			"receipt": fmt.Sprintf("%s%d", refundReceiptPrefix, refund.ID),
			"notes":   map[string]interface{}{"refund_id": strconv.FormatUint(refund.ID, 10)},
		},
		map[string]string{refundIdempotencyHeader: fmt.Sprintf("%s%d", refundIdempotencyPrefix, refund.ID)},
	)
	if err != nil {
		return "", "", fmt.Errorf("refund razorpay payment: %w", err)
	}

	var refunded refundEntity
	if err := decode(response, &refunded); err != nil {
		return "", "", fmt.Errorf("refund razorpay payment: %w", err)
	}
	if refunded.ID == "" {
		return "", "", errors.New("refund razorpay payment: response has no id")
	}

	return refunded.ID, toRefundStatus(refunded.Status), nil
}

// RefundStatus reports the payment's most settled refund: processed before
// pending before failed.
func (g *Gateway) RefundStatus(ctx context.Context, payment *models.Payment) (string, models.RefundStatus, error) {
	response, err := g.client.Payment.FetchMultipleRefund(payment.ProviderPaymentID, nil, nil)
	if err != nil {
		return "", "", fmt.Errorf("list razorpay refunds: %w", err)
	}

	var refunds collection[refundEntity]
	if err := decode(response, &refunds); err != nil {
		return "", "", fmt.Errorf("list razorpay refunds: %w", err)
	}

	rank := map[string]int{refundProcessed: 3, refundPending: 2, refundFailed: 1}
	var best refundEntity
	for _, refund := range refunds.Items {
		if rank[refund.Status] > rank[best.Status] {
			best = refund
		}
	}
	if best.ID == "" {
		return "", "", nil
	}

	return best.ID, toRefundStatus(best.Status), nil
}

// ParseWebhook checks X-Razorpay-Signature, the hex HMAC-SHA256 of the raw body
// keyed with the webhook secret. Razorpay signs no timestamp, so a body can be
// replayed; the service confirms every capture with the API and a replay
// changes nothing. A failed payment is ignored because Razorpay lets the payer
// try again on the same order.
func (g *Gateway) ParseWebhook(body []byte, headers map[string][]string) (*models.GatewayEvent, error) {
	if !validSignature(body, http.Header(headers).Get(signatureHeader), g.webhookSecret) {
		return nil, apperrors.ErrPaymentVerificationFailed
	}

	var payload webhookPayload
	if err := json.Unmarshal(body, &payload); err != nil {
		return nil, apperrors.Invalid(apperrors.ErrWebhookIgnored, "", "the body is not a Razorpay event")
	}

	switch payload.Event {
	case eventPaymentCaptured, eventOrderPaid:
		paid := payload.Payload.Payment.Entity
		if paid.ID == "" || paid.OrderID == "" {
			return nil, apperrors.Invalid(apperrors.ErrWebhookIgnored, "", payload.Event+" has no payment or order id")
		}
		return &models.GatewayEvent{
			Type:              models.GatewayEventPaymentCaptured,
			ProviderOrderID:   paid.OrderID,
			ProviderPaymentID: paid.ID,
			Amount:            paid.Amount,
		}, nil
	case eventRefundProcessed, eventRefundFailed:
		refund := payload.Payload.Refund.Entity
		if refund.ID == "" || refund.PaymentID == "" {
			return nil, apperrors.Invalid(apperrors.ErrWebhookIgnored, "", payload.Event+" has no refund or payment id")
		}
		eventType := models.GatewayEventRefundProcessed
		if payload.Event == eventRefundFailed {
			eventType = models.GatewayEventRefundFailed
		}
		return &models.GatewayEvent{
			Type:              eventType,
			ProviderPaymentID: refund.PaymentID,
			ProviderRefundID:  refund.ID,
			Amount:            refund.Amount,
		}, nil
	default:
		return nil, apperrors.Invalid(apperrors.ErrWebhookIgnored, "", fmt.Sprintf("event %q is not used", payload.Event))
	}
}

func (g *Gateway) fetchPayment(providerPaymentID string) (*paymentEntity, error) {
	response, err := g.client.Payment.Fetch(providerPaymentID, nil, nil)
	if err != nil {
		return nil, fmt.Errorf("fetch razorpay payment: %w", err)
	}

	var paid paymentEntity
	if err := decode(response, &paid); err != nil {
		return nil, fmt.Errorf("fetch razorpay payment: %w", err)
	}

	return &paid, nil
}

// validSignature compares the hex HMAC-SHA256 of message in constant time. It
// is computed here rather than with the SDK's utils package, which would
// build its test helpers into the binary.
func validSignature(message []byte, signature string, secret string) bool {
	presented, err := hex.DecodeString(signature)
	if err != nil || len(presented) == 0 {
		return false
	}
	mac := hmac.New(sha256.New, []byte(secret))
	mac.Write(message)

	return hmac.Equal(presented, mac.Sum(nil))
}

// decode turns the SDK's untyped response into one of the wire types above.
func decode(response map[string]interface{}, out any) error {
	encoded, err := json.Marshal(response)
	if err != nil {
		return fmt.Errorf("encode response: %w", err)
	}
	if err := json.Unmarshal(encoded, out); err != nil {
		return fmt.Errorf("decode response: %w", err)
	}

	return nil
}

func toRefundStatus(status string) models.RefundStatus {
	switch status {
	case refundProcessed:
		return models.RefundStatusSucceeded
	case refundFailed:
		return models.RefundStatusFailed
	default:
		return models.RefundStatusRequested
	}
}
