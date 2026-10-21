package services

import (
	"context"
	"fmt"
	"time"

	"appointment_system/internal/apps/appointment/apperrors"
	"appointment_system/internal/apps/appointment/constant"
	"appointment_system/internal/apps/appointment/models"
	"appointment_system/internal/apps/appointment/utils"
)

// ReminderService queues appointment reminders as outbox events: one in the
// evening before (from constant.ReminderEveningHour, for tomorrow's clinic
// date) and one constant.ReminderLeadTime before the start. An appointment
// just after midnight that the lead-time reminder already covers gets only that. Each is queued at
// most once per appointment, so the job can run as often as needed.
type ReminderService struct {
	appointments AppointmentRepository
	outbox       OutboxRepository
	tx           TransactionManager
	location     *time.Location
	now          func() time.Time
}

func NewReminderService(
	appointments AppointmentRepository,
	outbox OutboxRepository,
	tx TransactionManager,
	location *time.Location,
	now func() time.Time,
) (*ReminderService, error) {
	if appointments == nil {
		return nil, apperrors.ErrNilAppointmentRepository
	}
	if outbox == nil {
		return nil, apperrors.ErrNilOutboxRepository
	}
	if tx == nil {
		return nil, apperrors.ErrNilTransactionManager
	}
	if location == nil {
		return nil, apperrors.ErrNilClinicLocation
	}
	if now == nil {
		return nil, apperrors.ErrNilClock
	}

	return &ReminderService{
		appointments: appointments,
		outbox:       outbox,
		tx:           tx,
		location:     location,
		now:          now,
	}, nil
}

// QueueDue queues every reminder that is due now, up to limit per window, and
// returns how many it queued.
func (rs *ReminderService) QueueDue(ctx context.Context, limit int) (int, error) {
	if err := ctx.Err(); err != nil {
		return 0, fmt.Errorf("queue reminders context: %w", err)
	}
	limit, _ = clampPagination(limit, 0)

	now := rs.now().In(rs.location)
	leadEnd := now.Add(constant.ReminderLeadTime)
	queued, err := rs.queue(ctx, models.ReminderWindowTwoHours, now, leadEnd, limit)
	if err != nil {
		return queued, err
	}

	if now.Hour() >= constant.ReminderEveningHour {
		tomorrow := utils.CivilDate(now).AddDate(0, 0, 1)
		tomorrowStart := time.Date(tomorrow.Year(), tomorrow.Month(), tomorrow.Day(), 0, 0, 0, 0, rs.location)
		dayBeforeStart := tomorrowStart
		if dayBeforeStart.Before(leadEnd) {
			dayBeforeStart = leadEnd
		}
		dayBefore, err := rs.queue(ctx, models.ReminderWindowDayBefore, dayBeforeStart, tomorrowStart.AddDate(0, 0, 1), limit)
		queued += dayBefore
		if err != nil {
			return queued, err
		}
	}

	return queued, nil
}

func (rs *ReminderService) queue(
	ctx context.Context,
	window models.ReminderWindow,
	from time.Time,
	to time.Time,
	limit int,
) (int, error) {
	queued := 0
	err := rs.tx.WithinTransaction(ctx, func(txCtx context.Context) error {
		due, err := rs.appointments.ListDueForReminder(txCtx, window, from, to, limit)
		if err != nil {
			return err
		}

		for _, appointment := range due {
			created, err := rs.appointments.CreateReminder(txCtx, appointment.ID, window)
			if err != nil {
				return err
			}
			if !created {
				continue
			}

			event := appointmentEvent(models.OutboxEventAppointmentReminder, &appointment.Appointment, 0)
			delete(event.Payload, "changedByUserId")
			event.Payload["window"] = window
			if err := rs.outbox.Create(txCtx, event); err != nil {
				return err
			}
			queued++
		}

		return nil
	})
	if err != nil {
		return 0, fmt.Errorf("queue %s reminders: %w", window, err)
	}

	return queued, nil
}
