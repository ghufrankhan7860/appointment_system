// Command expire_proposals closes reschedule proposals nobody answered in
// time, as if the patient side declined them, and exits. Run it every few
// minutes from a scheduler. A proposal answered meanwhile is skipped, so it is
// safe to overlap with itself and with patients answering.
//
//	go run ./scripts/cron/expire_proposals
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
	jobName = "expire_proposals"
)

func main() {
	if err := run(); err != nil {
		slog.Error("expire reschedule proposals", "error", err)
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
	availabilityRepository, err := repositories.NewAvailabilityRepository(store.DB)
	if err != nil {
		return fmt.Errorf("initialize availability repository: %w", err)
	}
	paymentRepository, err := repositories.NewPaymentRepository(store.DB)
	if err != nil {
		return fmt.Errorf("initialize payment repository: %w", err)
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

	availabilityService, err := services.NewAvailabilityService(
		doctorRepository,
		availabilityRepository,
		appointmentRepository,
		txManager,
		config.App.Location,
		time.Now,
		logger,
	)
	if err != nil {
		return fmt.Errorf("initialize availability service: %w", err)
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
	callRoomRepository, err := repositories.NewCallRoomRepository(store.DB)
	if err != nil {
		return fmt.Errorf("initialize call room repository: %w", err)
	}
	callEventRepository, err := repositories.NewCallEventRepository(store.DB)
	if err != nil {
		return fmt.Errorf("initialize call event repository: %w", err)
	}
	appointmentService, err := services.NewAppointmentService(
		appointmentRepository,
		outboxRepository,
		userPatientRepository,
		doctorRepository,
		clinicActor,
		availabilityService,
		paymentService,
		callRoomRepository,
		callEventRepository,
		txManager,
		logger,
	)
	if err != nil {
		return fmt.Errorf("initialize appointment service: %w", err)
	}

	expired, err := appointmentService.ExpireProposals(ctx, expireBatchSize)
	if err != nil {
		return err
	}

	logger.Info("run finished", "expired", expired, "duration_ms", time.Since(started).Milliseconds())
	return nil
}
