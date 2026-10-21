// Command reminders queues every appointment reminder that is due and exits.
// Run it every few minutes from a scheduler (cron, a Kubernetes CronJob). A
// reminder is never queued twice, so repeated or overlapping runs are safe.
// The reminders reach patients through the outbox consumer.
//
//	go run ./scripts/cron/reminders
package main

import (
	"context"
	"fmt"
	"log"
	"os"
	"time"
	_ "time/tzdata"

	"appointment_system/config"
	"appointment_system/internal/apps/appointment/repositories"
	"appointment_system/internal/apps/appointment/services"
	"appointment_system/internal/platform/datastore"
)

const (
	reminderBatchSize = 100
	reminderTimeout   = time.Minute
)

func main() {
	if err := run(); err != nil {
		log.Printf("queue reminders: %v", err)
		os.Exit(1)
	}
}

func run() error {
	config, err := config.Load()
	if err != nil {
		return fmt.Errorf("load configuration: %w", err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), reminderTimeout)
	defer cancel()

	store, err := datastore.Open(ctx, datastore.Config{
		Host:            config.Database.Host,
		Port:            config.Database.Port,
		User:            config.Database.User,
		Password:        config.Database.Password,
		Database:        config.Database.Name,
		MaxOpenConns:    config.Database.MaxOpenConns,
		MaxIdleConns:    config.Database.MaxIdleConns,
		ConnMaxLifetime: config.Database.ConnMaxLifetime,
		ConnectTimeout:  config.Database.ConnectTimeout,
	})
	if err != nil {
		return fmt.Errorf("initialize datastore: %w", err)
	}
	defer func() {
		if err := store.Close(); err != nil {
			log.Printf("close datastore: %v", err)
		}
	}()

	appointmentRepository, err := repositories.NewAppointmentRepository(store.DB)
	if err != nil {
		return fmt.Errorf("initialize appointment repository: %w", err)
	}
	outboxRepository, err := repositories.NewOutboxRepository(store.DB)
	if err != nil {
		return fmt.Errorf("initialize outbox repository: %w", err)
	}
	txManager, err := repositories.NewGORMTransactionManager(store.DB)
	if err != nil {
		return fmt.Errorf("initialize transaction manager: %w", err)
	}
	reminderService, err := services.NewReminderService(appointmentRepository, outboxRepository, txManager, config.App.Location, time.Now)
	if err != nil {
		return fmt.Errorf("initialize reminder service: %w", err)
	}

	queued, err := reminderService.QueueDue(ctx, reminderBatchSize)
	if err != nil {
		return err
	}

	log.Printf("queued %d reminders", queued)
	return nil
}
