package services

import (
	"appointment_system/internal/apps/appointment/models"
	"context"
	"time"
)

// PaymentGateway is the application's view of a payment provider. Each provider
// is one adapter under adapters/payment, chosen by configuration; services never
// see a provider's SDK or wire types, so adding one needs no service change.
type PaymentGateway interface {
	// Provider is the stable id stored on every payment, such as "none".
	Provider() string
	// CreateOrder opens an order for amount (minor units) and returns its id.
	CreateOrder(ctx context.Context, appointmentID uint64, amount int, currency string) (string, error)
	// ClientPayload is what the client needs to pay an existing order.
	ClientPayload(payment *models.Payment) map[string]any
	// VerifyPayment checks with the gateway that providerPaymentID paid the
	// payment's order in full and returns when the payer paid, or the zero time
	// when the gateway does not say. signature is the client's proof from
	// checkout; it is empty when the server learned of the payment itself. A
	// payment that does not hold returns apperrors.ErrPaymentVerificationFailed.
	VerifyPayment(ctx context.Context, payment *models.Payment, providerPaymentID string, signature string) (time.Time, error)
	// FindPaidOrder asks the gateway whether the payment's order was paid after
	// all and returns the paying gateway payment id, or "" when it was not.
	FindPaidOrder(ctx context.Context, payment *models.Payment) (string, error)
	// Refund returns refund.Amount of a captured payment. It gives the
	// gateway's refund id and status: succeeded, requested while the gateway
	// is still processing it, or failed. An error means the outcome is unknown.
	Refund(ctx context.Context, payment *models.Payment, refund *models.Refund) (string, models.RefundStatus, error)
	// RefundStatus reports the gateway's refund of a payment, its id and
	// status, or "" when the gateway has none.
	RefundStatus(ctx context.Context, payment *models.Payment) (string, models.RefundStatus, error)
	// ParseWebhook checks a provider callback's signature over the raw body and
	// returns what it reports. A bad signature, or a provider without webhooks,
	// returns apperrors.ErrPaymentVerificationFailed; a callback with nothing
	// to apply returns apperrors.ErrWebhookIgnored. headers is http.Header's
	// underlying map, so services stay free of HTTP types.
	ParseWebhook(body []byte, headers map[string][]string) (*models.GatewayEvent, error)
}
