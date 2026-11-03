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
	"fmt"
	"log/slog"
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
	"appointment_system/internal/platform/logger"
)

const (
	outboxBatchSize    = 100
	outboxPollInterval = 5 * time.Second
	// jobName tags every log line of this command.
	jobName = "outbox_consumer"
)

func main() {
	if err := run(); err != nil {
		slog.Error("outbox consumer", "error", err)
		os.Exit(1)
	}
}

func run() error {
	config, err := config.Load()
	if err != nil {
		return fmt.Errorf("load configuration: %w", err)
	}

	logOutput, _, err := logger.Output(config.Log.File)
	if err != nil {
		return fmt.Errorf("initialize log output: %w", err)
	}
	baseLogger, err := logger.New(logger.Config{Level: config.Log.Level, Format: config.Log.Format, Source: config.Log.Source}, logOutput)
	if err != nil {
		return fmt.Errorf("initialize logger: %w", err)
	}
	slog.SetDefault(baseLogger)
	logger := baseLogger.With("job", jobName)

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
		Logger:          logger,
	})
	if err != nil {
		return fmt.Errorf("initialize datastore: %w", err)
	}
	defer func() {
		if err := store.Close(); err != nil {
			logger.Error("close datastore", "error", err)
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
	outboxService, err := services.NewOutboxService(outboxRepository, txManager, logger)
	if err != nil {
		return fmt.Errorf("initialize outbox service: %w", err)
	}

	logger.Info("consumer started", "poll_interval", outboxPollInterval.String())
	for {
		published, err := outboxService.Publish(ctx, outboxBatchSize, logEvent(logger))
		if err != nil && ctx.Err() == nil {
			logger.Error("publish outbox", "error", err)
		}
		if published == outboxBatchSize {
			continue
		}

		select {
		case <-ctx.Done():
			logger.Info("consumer stopped")
			return nil
		case <-time.After(outboxPollInterval):
		}
	}
}

// ponytail: the log is the only publisher; add a notifier port with one adapter
// per channel when the first real channel (email, SMS) is built.
// The payload is never logged: it can carry personal data.
func logEvent(logger *slog.Logger) func(context.Context, *models.OutboxEvent) error {
	return func(ctx context.Context, event *models.OutboxEvent) error {
		logger.InfoContext(ctx, "event published",
			"event_id", event.ID,
			"event_type", event.EventType,
			"aggregate_type", event.AggregateType,
			"aggregate_id", event.AggregateID,
		)

		return nil
	}
}
