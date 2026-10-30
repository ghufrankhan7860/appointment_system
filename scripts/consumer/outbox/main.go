// Command outbox publishes outbox events until it is stopped with Ctrl+C or
// SIGTERM. Each event is published at least once, in id order; several copies
// can run side by side because a batch locks its rows and skips locked ones.
//
// For now an event is written to the log. A real channel (email, SMS, push)
// replaces logEvent.
//
//	go run ./scripts/consumer/outbox
package main

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"os"
	"os/signal"
	"syscall"
	"time"
	_ "time/tzdata"

	"appointment_system/config"
	"appointment_system/internal/apps/appointment/models"
	"appointment_system/internal/apps/appointment/repositories"
	"appointment_system/internal/apps/appointment/services"
	"appointment_system/internal/platform/datastore"
)

const (
	outboxBatchSize    = 100
	outboxPollInterval = 5 * time.Second
)

func main() {
	if err := run(); err != nil {
		log.Printf("outbox consumer: %v", err)
		os.Exit(1)
	}
}

func run() error {
	config, err := config.Load()
	if err != nil {
		return fmt.Errorf("load configuration: %w", err)
	}

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	store, err := datastore.Open(ctx, datastore.Config{
		Host:            config.Database.Host,
		Port:            config.Database.Port,
		User:            config.Database.User,
		Password:        config.Database.Password,
		Database:        config.Database.Name,
		SSLMode:         config.Database.SSLMode,
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

	outboxRepository, err := repositories.NewOutboxRepository(store.DB)
	if err != nil {
		return fmt.Errorf("initialize outbox repository: %w", err)
	}
	txManager, err := repositories.NewGORMTransactionManager(store.DB)
	if err != nil {
		return fmt.Errorf("initialize transaction manager: %w", err)
	}
	outboxService, err := services.NewOutboxService(outboxRepository, txManager)
	if err != nil {
		return fmt.Errorf("initialize outbox service: %w", err)
	}

	log.Printf("outbox consumer started, polling every %s", outboxPollInterval)
	for {
		published, err := outboxService.Publish(ctx, outboxBatchSize, logEvent)
		if err != nil && ctx.Err() == nil {
			log.Printf("publish outbox: %v", err)
		}
		if published > 0 {
			log.Printf("published %d events", published)
		}
		if published == outboxBatchSize {
			continue
		}

		select {
		case <-ctx.Done():
			log.Printf("outbox consumer stopped")
			return nil
		case <-time.After(outboxPollInterval):
		}
	}
}

// ponytail: the log is the only publisher; add a notifier port with one adapter
// per channel when the first real channel (email, SMS) is built.
func logEvent(ctx context.Context, event *models.OutboxEvent) error {
	payload, err := json.Marshal(event.Payload)
	if err != nil {
		return fmt.Errorf("encode event %d: %w", event.ID, err)
	}

	log.Printf("event id=%d type=%s %s=%d payload=%s", event.ID, event.EventType, event.AggregateType, event.AggregateID, payload)
	return nil
}
