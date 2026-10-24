package services

import (
	"appointment_system/internal/apps/appointment/models"
	"context"
	"time"
)

type PaymentRepository interface {
	Create(ctx context.Context, payment *models.Payment) (*models.Payment, error)
	GetByID(ctx context.Context, id uint64) (*models.Payment, error)
	GetLatestByAppointment(ctx context.Context, appointmentID uint64) (*models.Payment, error)
	GetActiveByAppointment(ctx context.Context, appointmentID uint64) (*models.Payment, error)
	GetByProviderOrderID(ctx context.Context, provider string, providerOrderID string) (*models.Payment, error)
	UpdateStatus(ctx context.Context, id uint64, fromStatus models.PaymentStatus, toStatus models.PaymentStatus, providerPaymentID string, capturedAt *time.Time) error
	MoveToAppointment(ctx context.Context, fromAppointmentID uint64, toAppointmentID uint64) error
	CreateStatusChange(ctx context.Context, change *models.PaymentStatusChange) error
	CreateRefund(ctx context.Context, refund *models.Refund) (*models.Refund, error)
	FinishRefund(ctx context.Context, id uint64, status models.RefundStatus, providerRefundID string) error
	GetRefundByID(ctx context.Context, id uint64) (*models.Refund, error)
	ListRefunds(ctx context.Context, status models.RefundStatus, limit int, offset int) ([]*models.Refund, error)
}
