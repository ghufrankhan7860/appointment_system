package services

import (
	"appointment_system/internal/apps/appointment/models"
	"context"
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
	// VerifyPayment checks the client's proof that the order was paid and
	// returns apperrors.ErrPaymentVerificationFailed when it does not hold.
	VerifyPayment(ctx context.Context, payment *models.Payment, providerPaymentID string, signature string) error
	// Refund returns amount of a captured payment and gives the provider's refund id.
	Refund(ctx context.Context, payment *models.Payment, amount int) (string, error)
}
