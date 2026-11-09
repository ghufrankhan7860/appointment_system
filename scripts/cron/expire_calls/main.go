// Command expire_calls ends call rooms whose time is over and exits: an
// appointment's room once the appointment closed or its call window passed, and
// a code room past its expiry, left by its host or empty for too long. Run it
// every five minutes from a scheduler. A room ended meanwhile is skipped, so it
// is safe to overlap with itself and with calls ending. Sockets still open in
// the API see the room ended on their next heartbeat.
//
//	go run ./scripts/cron/expire_calls
package main

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"time"
	_ "time/tzdata"

	"appointment_system/config"
	"appointment_system/internal/apps/appointment/adapters/callhub/memory"
	"appointment_system/internal/apps/appointment/constant"
	"appointment_system/internal/apps/appointment/repositories"
	"appointment_system/internal/apps/appointment/services"
	"appointment_system/internal/apps/appointment/utils"
	"appointment_system/internal/platform/datastore"
	"appointment_system/internal/platform/logger"
)

const (
	expireBatchSize = 200
	expireTimeout   = time.Minute
	// jobName tags every log line of this command.
	jobName = "expire_calls"
)

func main() {
	if err := run(); err != nil {
		slog.Error("expire call rooms", "error", err)
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
	doctorRepository, err := repositories.NewDoctorRepository(store.DB)
	if err != nil {
		return fmt.Errorf("initialize doctor repository: %w", err)
	}
	userPatientRepository, err := repositories.NewUserPatientRepository(store.DB)
	if err != nil {
		return fmt.Errorf("initialize user-patient repository: %w", err)
	}
	userRepository, err := repositories.NewUserRepository(store.DB)
	if err != nil {
		return fmt.Errorf("initialize user repository: %w", err)
	}
	callRoomRepository, err := repositories.NewCallRoomRepository(store.DB)
	if err != nil {
		return fmt.Errorf("initialize call room repository: %w", err)
	}
	callParticipantRepository, err := repositories.NewCallParticipantRepository(store.DB)
	if err != nil {
		return fmt.Errorf("initialize call participant repository: %w", err)
	}
	callGuestTokenRepository, err := repositories.NewCallGuestTokenRepository(store.DB)
	if err != nil {
		return fmt.Errorf("initialize call guest token repository: %w", err)
	}
	callEventRepository, err := repositories.NewCallEventRepository(store.DB)
	if err != nil {
		return fmt.Errorf("initialize call event repository: %w", err)
	}
	callJoinAttemptRepository, err := repositories.NewCallJoinAttemptRepository(store.DB)
	if err != nil {
		return fmt.Errorf("initialize call join attempt repository: %w", err)
	}
	outboxRepository, err := repositories.NewOutboxRepository(store.DB)
	if err != nil {
		return fmt.Errorf("initialize outbox repository: %w", err)
	}
	txManager, err := repositories.NewGORMTransactionManager(store.DB)
	if err != nil {
		return fmt.Errorf("initialize transaction manager: %w", err)
	}
	callTokenManager, err := utils.NewCallTokenManager(config.Auth.JWTSecret, config.Auth.JWTIssuer, constant.CallTokenTTL)
	if err != nil {
		return fmt.Errorf("initialize call token manager: %w", err)
	}

	callService, err := services.NewCallService(
		appointmentRepository,
		doctorRepository,
		userPatientRepository,
		userRepository,
		callRoomRepository,
		callParticipantRepository,
		callGuestTokenRepository,
		callEventRepository,
		callJoinAttemptRepository,
		outboxRepository,
		txManager,
		callTokenManager,
		memory.New(),
		services.ICESettings{},
		time.Now,
		logger,
	)
	if err != nil {
		return fmt.Errorf("initialize call service: %w", err)
	}

	expired, err := callService.Expire(ctx, expireBatchSize)
	if err != nil {
		return err
	}

	logger.Info("run finished", "expired", expired, "duration_ms", time.Since(started).Milliseconds())
	return nil
}
