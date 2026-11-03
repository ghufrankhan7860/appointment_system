// Command reconcile_refunds settles refunds still requested with what the
// payment gateway says about them, in case a refund webhook was lost, and
// exits. Run it every 15 minutes from a scheduler. A refund the gateway never
// received is failed after a grace period so an admin can retry it.
//
//	go run ./scripts/cron/reconcile_refunds
package main

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"time"
	_ "time/tzdata"

	"appointment_system/config"
	"appointment_system/internal/apps/appointment"
	"appointment_system/internal/apps/appointment/repositories"
	"appointment_system/internal/apps/appointment/services"
	"appointment_system/internal/platform/datastore"
	"appointment_system/internal/platform/logger"
)

const (
	reconcileBatchSize = 100
	reconcileTimeout   = 5 * time.Minute
	// jobName tags every log line of this command.
	jobName = "reconcile_refunds"
)

func main() {
	if err := run(); err != nil {
		slog.Error("reconcile refunds", "error", err)
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
	started := time.Now()
	logger.Info("run started")

	ctx, cancel := context.WithTimeout(context.Background(), reconcileTimeout)
	defer cancel()

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

	paymentRepository, err := repositories.NewPaymentRepository(store.DB)
	if err != nil {
		return fmt.Errorf("initialize payment repository: %w", err)
	}
	appointmentRepository, err := repositories.NewAppointmentRepository(store.DB)
	if err != nil {
		return fmt.Errorf("initialize appointment repository: %w", err)
	}
	outboxRepository, err := repositories.NewOutboxRepository(store.DB)
	if err != nil {
		return fmt.Errorf("initialize outbox repository: %w", err)
	}
	userPatientRepository, err := repositories.NewUserPatientRepository(store.DB)
	if err != nil {
		return fmt.Errorf("initialize user-patient repository: %w", err)
	}
	doctorRepository, err := repositories.NewDoctorRepository(store.DB)
	if err != nil {
		return fmt.Errorf("initialize doctor repository: %w", err)
	}
	txManager, err := repositories.NewGORMTransactionManager(store.DB)
	if err != nil {
		return fmt.Errorf("initialize transaction manager: %w", err)
	}
	paymentGateway, err := appointment.NewPaymentGateway(appointment.PaymentSettings{
		Provider:              config.Payment.Provider,
		RazorpayKeyID:         config.Payment.RazorpayKeyID,
		RazorpayKeySecret:     config.Payment.RazorpayKeySecret,
		RazorpayWebhookSecret: config.Payment.RazorpayWebhookSecret,
	})
	if err != nil {
		return fmt.Errorf("initialize payment gateway: %w", err)
	}

	paymentService, err := services.NewPaymentService(
		paymentRepository,
		appointmentRepository,
		outboxRepository,
		userPatientRepository,
		doctorRepository,
		paymentGateway,
		txManager,
		time.Now,
		logger,
	)
	if err != nil {
		return fmt.Errorf("initialize payment service: %w", err)
	}

	settled, err := paymentService.ReconcileRefunds(ctx, reconcileBatchSize)
	if err != nil {
		return err
	}

	logger.Info("run finished", "settled", settled, "duration_ms", time.Since(started).Milliseconds())
	return nil
}
