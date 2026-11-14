// Command expire_payments cancels bookings whose payment did not arrive within
// the hold, which frees their slots, and exits. It first asks the gateway
// whether each open order was paid after all, in case its webhook was lost.
// Run it every minute from a scheduler. A booking paid meanwhile is skipped,
// so it is safe to run while payments come in and to overlap with itself.
//
//	go run ./scripts/cron/expire_payments
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
	expireBatchSize = 100
	expireTimeout   = time.Minute
	// jobName tags every log line of this command.
	jobName = "expire_payments"
)

func main() {
	if err := run(); err != nil {
		slog.Error("expire unpaid bookings", "error", err)
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

	ctx, cancel := context.WithTimeout(context.Background(), expireTimeout)
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
		Methods:               config.Payment.Methods,
		Provider:              config.Payment.Provider,
		RazorpayKeyID:         config.Payment.RazorpayKeyID,
		RazorpayKeySecret:     config.Payment.RazorpayKeySecret,
		RazorpayWebhookSecret: config.Payment.RazorpayWebhookSecret,
	})
	if err != nil {
		return fmt.Errorf("initialize payment gateway: %w", err)
	}
	fileStorage, err := appointment.NewFileStorage(config.Storage.Provider, config.Storage.LocalDir)
	if err != nil {
		return fmt.Errorf("initialize file storage: %w", err)
	}

	staffRepository, err := repositories.NewStaffRepository(store.DB)
	if err != nil {
		return fmt.Errorf("initialize staff repository: %w", err)
	}
	clinicPatientRepository, err := repositories.NewClinicPatientRepository(store.DB)
	if err != nil {
		return fmt.Errorf("initialize clinic patient repository: %w", err)
	}
	clinicActor, err := services.NewClinicActor(doctorRepository, staffRepository, clinicPatientRepository)
	if err != nil {
		return fmt.Errorf("initialize clinic actor: %w", err)
	}

	paymentService, err := services.NewPaymentService(
		paymentRepository,
		appointmentRepository,
		outboxRepository,
		userPatientRepository,
		doctorRepository,
		clinicActor,
		paymentGateway,
		appointment.PaymentMethods(config.Payment.Methods),
		fileStorage,
		txManager,
		time.Now,
		logger,
	)
	if err != nil {
		return fmt.Errorf("initialize payment service: %w", err)
	}

	expired, err := paymentService.ExpireUnpaid(ctx, expireBatchSize)
	if err != nil {
		return err
	}

	logger.Info("run finished", "expired", expired, "duration_ms", time.Since(started).Milliseconds())
	return nil
}
