// Package none is a payment gateway for local development that moves no money.
// An order counts as paid when it is verified with any payment id and the
// signature from its client payload. Configuration refuses it in production.
package none

import (
	"context"
	"fmt"
	"time"

	"appointment_system/internal/apps/appointment/apperrors"
	"appointment_system/internal/apps/appointment/models"
)

const (
	ProviderName    = "none"
	orderPrefix     = "none_order_"
	signaturePrefix = "none_signature_"
	refundPrefix    = "none_refund_"
)

type Gateway struct{}

func New() *Gateway {
	return &Gateway{}
}

func (g *Gateway) Provider() string {
	return ProviderName
}

func (g *Gateway) CreateOrder(ctx context.Context, appointmentID uint64, amount int, currency string) (string, error) {
	if err := ctx.Err(); err != nil {
		return "", fmt.Errorf("create none order context: %w", err)
	}

	return fmt.Sprintf("%s%d_%d", orderPrefix, appointmentID, time.Now().UnixNano()), nil
}

// ClientPayload hands the client the signature to verify with, standing in for
// the one a real gateway's checkout would return after payment.
func (g *Gateway) ClientPayload(payment *models.Payment) map[string]any {
	return map[string]any{
		"orderId":   payment.ProviderOrderID,
		"amount":    payment.Amount,
		"currency":  payment.Currency,
		"signature": signaturePrefix + payment.ProviderOrderID,
	}
}

// VerifyPayment does not know when the payer paid, so it returns the zero time.
func (g *Gateway) VerifyPayment(ctx context.Context, payment *models.Payment, providerPaymentID string, signature string) (time.Time, error) {
	if providerPaymentID == "" || signature != signaturePrefix+payment.ProviderOrderID {
		return time.Time{}, apperrors.ErrPaymentVerificationFailed
	}

	return time.Time{}, nil
}

// FindPaidOrder finds nothing: an order is paid only by verifying it.
func (g *Gateway) FindPaidOrder(ctx context.Context, payment *models.Payment) (string, error) {
	return "", nil
}

// Refund succeeds at once.
func (g *Gateway) Refund(ctx context.Context, payment *models.Payment, refund *models.Refund) (string, models.RefundStatus, error) {
	if err := ctx.Err(); err != nil {
		return "", "", fmt.Errorf("refund none payment context: %w", err)
	}

	return fmt.Sprintf("%s%d", refundPrefix, payment.ID), models.RefundStatusSucceeded, nil
}

// RefundStatus finds nothing, because Refund never leaves one in progress.
func (g *Gateway) RefundStatus(ctx context.Context, payment *models.Payment) (string, models.RefundStatus, error) {
	return "", "", nil
}

// ParseWebhook refuses every callback: nothing calls back for a gateway that
// moves no money.
func (g *Gateway) ParseWebhook(body []byte, headers map[string][]string) (*models.GatewayEvent, error) {
	return nil, apperrors.ErrPaymentVerificationFailed
}
