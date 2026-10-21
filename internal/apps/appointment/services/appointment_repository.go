package services

import (
	"appointment_system/internal/apps/appointment/models"
	"context"
	"time"
)

type AppointmentRepository interface {
	Create(ctx context.Context, appointment *models.Appointment) (*models.Appointment, error)
	GetByID(ctx context.Context, id uint64) (*models.AppointmentDetails, error)
	GetByIdempotencyKey(ctx context.Context, userID uint64, key string) (*models.AppointmentDetails, error)
	UpdateStatus(ctx context.Context, id uint64, fromStatus models.AppointmentStatus, toStatus models.AppointmentStatus) error
	Cancel(ctx context.Context, id uint64, cancelledByUserID uint64, reason models.CancelReason) error
	ListByUser(ctx context.Context, userID uint64, filter models.AppointmentFilter) ([]*models.AppointmentDetails, error)
	ListByDoctorBetween(ctx context.Context, doctorID uint64, from time.Time, to time.Time) ([]*models.AppointmentDetails, error)
	CreateStatusChange(ctx context.Context, change *models.AppointmentStatusChange) error
	ListDueForReminder(ctx context.Context, window models.ReminderWindow, from time.Time, to time.Time, limit int) ([]*models.AppointmentDetails, error)
	CreateReminder(ctx context.Context, appointmentID uint64, window models.ReminderWindow) (bool, error)
	MarkPaid(ctx context.Context, id uint64) error
	SetPaymentStatus(ctx context.Context, id uint64, fromStatus models.AppointmentPaymentStatus, toStatus models.AppointmentPaymentStatus) error
	ListUnpaidDue(ctx context.Context, dueBefore time.Time, limit int) ([]*models.AppointmentDetails, error)
	CancelUnpaid(ctx context.Context, id uint64, dueBefore time.Time) error
	CreateProposal(ctx context.Context, proposal *models.RescheduleProposal) (*models.RescheduleProposal, error)
	GetPendingProposal(ctx context.Context, appointmentID uint64) (*models.RescheduleProposal, error)
	ListExpiredProposals(ctx context.Context, now time.Time, limit int) ([]*models.RescheduleProposal, error)
	CloseProposal(ctx context.Context, id uint64, toStatus models.ProposalStatus, respondedByUserID uint64) error
}
