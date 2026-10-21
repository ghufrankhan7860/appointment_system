package services_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"appointment_system/internal/apps/appointment/apperrors"
	"appointment_system/internal/apps/appointment/constant"
	"appointment_system/internal/apps/appointment/models"
	"appointment_system/internal/apps/appointment/services"
)

func TestOutboxServicePublish(t *testing.T) {
	errChannelDown := errors.New("channel down")
	events := func() []*models.OutboxEvent {
		return []*models.OutboxEvent{{ID: 1}, {ID: 2}, {ID: 3}}
	}

	tests := []struct {
		name          string
		failAt        uint64
		wantPublished []uint64
		wantErr       error
	}{
		{name: "every event is published and marked", wantPublished: []uint64{1, 2, 3}},
		{name: "a failed publish stops the batch after the sent ones", failAt: 2, wantPublished: []uint64{1}, wantErr: errChannelDown},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			outbox := &fakeOutboxRepo{unpublished: events()}
			service, err := services.NewOutboxService(outbox, &fakeTxManager{})
			if err != nil {
				t.Fatalf("NewOutboxService returned error: %v", err)
			}

			count, err := service.Publish(context.Background(), 10, func(ctx context.Context, event *models.OutboxEvent) error {
				if event.ID == tt.failAt {
					return errChannelDown
				}
				return nil
			})
			if tt.wantErr != nil && !errors.Is(err, tt.wantErr) {
				t.Fatalf("Publish error = %v, want %v", err, tt.wantErr)
			}
			if tt.wantErr == nil && err != nil {
				t.Fatalf("Publish returned error: %v", err)
			}
			if count != len(tt.wantPublished) || len(outbox.published) != len(tt.wantPublished) {
				t.Fatalf("Publish count = %d, marked %v; want %v", count, outbox.published, tt.wantPublished)
			}
			for index, id := range tt.wantPublished {
				if outbox.published[index] != id {
					t.Fatalf("Publish marked %v, want %v", outbox.published, tt.wantPublished)
				}
			}
		})
	}
}

func TestNewOutboxServiceValidation(t *testing.T) {
	if _, err := services.NewOutboxService(nil, &fakeTxManager{}); !errors.Is(err, apperrors.ErrNilOutboxRepository) {
		t.Fatalf("NewOutboxService error = %v, want %v", err, apperrors.ErrNilOutboxRepository)
	}
	if _, err := services.NewOutboxService(&fakeOutboxRepo{}, nil); !errors.Is(err, apperrors.ErrNilTransactionManager) {
		t.Fatalf("NewOutboxService error = %v, want %v", err, apperrors.ErrNilTransactionManager)
	}
}

func TestReminderServiceQueueDue(t *testing.T) {
	location := clinicLocation(t)
	afternoon := time.Date(2026, 10, 25, 15, 0, 0, 0, location)
	evening := time.Date(2026, 10, 25, 19, 0, 0, 0, location)
	lateEvening := time.Date(2026, 10, 25, 23, 30, 0, 0, location)
	soon := scheduledAt(afternoon.Add(90*time.Minute), models.AppointmentStatusScheduled)
	tomorrow := scheduledAt(time.Date(2026, 10, 26, 9, 0, 0, 0, location), models.AppointmentStatusScheduled)
	tomorrow.ID = 51

	tests := []struct {
		name        string
		now         time.Time
		wantWindows []models.ReminderWindow
	}{
		{name: "afternoon queues only the two-hour reminder", now: afternoon, wantWindows: []models.ReminderWindow{models.ReminderWindowTwoHours}},
		{name: "evening also queues tomorrow's reminders", now: evening, wantWindows: []models.ReminderWindow{models.ReminderWindowTwoHours, models.ReminderWindowDayBefore}},
		{name: "late evening leaves just-after-midnight appointments to the lead-time reminder", now: lateEvening, wantWindows: []models.ReminderWindow{models.ReminderWindowTwoHours, models.ReminderWindowDayBefore}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			appointments := &fakeAppointmentRepo{due: map[models.ReminderWindow][]*models.AppointmentDetails{
				models.ReminderWindowTwoHours:  {soon},
				models.ReminderWindowDayBefore: {tomorrow},
			}}
			outbox := &fakeOutboxRepo{}
			service, err := services.NewReminderService(appointments, outbox, &fakeTxManager{}, location, func() time.Time { return tt.now })
			if err != nil {
				t.Fatalf("NewReminderService returned error: %v", err)
			}

			queued, err := service.QueueDue(context.Background(), 10)
			if err != nil {
				t.Fatalf("QueueDue returned error: %v", err)
			}
			if queued != len(tt.wantWindows) || len(outbox.events) != len(tt.wantWindows) {
				t.Fatalf("QueueDue queued %d with %d events, want %d", queued, len(outbox.events), len(tt.wantWindows))
			}
			for index, window := range tt.wantWindows {
				event := outbox.events[index]
				if event.EventType != models.OutboxEventAppointmentReminder || event.Payload["window"] != window {
					t.Fatalf("QueueDue event %d = %s %v, want a %s reminder", index, event.EventType, event.Payload["window"], window)
				}
			}
			if tt.now == lateEvening && !appointments.dueFrom[models.ReminderWindowDayBefore].Equal(lateEvening.Add(constant.ReminderLeadTime)) {
				t.Fatalf("QueueDue late-evening day-before window starts %v, want after the lead-time window", appointments.dueFrom[models.ReminderWindowDayBefore])
			}
			if tt.now == evening && !appointments.dueFrom[models.ReminderWindowDayBefore].Equal(time.Date(2026, 10, 26, 0, 0, 0, 0, location)) {
				t.Fatalf("QueueDue day-before window starts %v, want tomorrow's clinic midnight", appointments.dueFrom[models.ReminderWindowDayBefore])
			}

			again, err := service.QueueDue(context.Background(), 10)
			if err != nil || again != 0 {
				t.Fatalf("second QueueDue = %d, %v; want nothing queued twice", again, err)
			}
		})
	}
}
