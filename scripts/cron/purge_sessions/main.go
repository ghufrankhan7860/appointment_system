// Command purge_sessions deletes sessions that expired or were revoked more than
// the retention period ago (constant.SessionRetention), and exits. Sessions hold
// the client IP and device details, which are personal data, so they are not
// kept longer than needed. Run it once a day from a scheduler; it is safe to
// overlap with itself and with logins.
//
//	go run ./scripts/cron/purge_sessions
package main

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"time"
	_ "time/tzdata"

	"appointment_system/config"
	"appointment_system/internal/apps/appointment/constant"
	"appointment_system/internal/apps/appointment/repositories"
	"appointment_system/internal/apps/appointment/services"
	"appointment_system/internal/platform/datastore"
	"appointment_system/internal/platform/logger"
)

const (
	purgeTimeout = 5 * time.Minute
	// jobName tags every log line of this command.
	jobName = "purge_sessions"
)

func main() {
	if err := run(); err != nil {
		slog.Error("purge sessions", "error", err)
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

	ctx, cancel := context.WithTimeout(context.Background(), purgeTimeout)
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

	sessionRepository, err := repositories.NewSessionRepository(store.DB)
	if err != nil {
		return fmt.Errorf("initialize session repository: %w", err)
	}
	userRepository, err := repositories.NewUserRepository(store.DB)
	if err != nil {
		return fmt.Errorf("initialize user repository: %w", err)
	}
	sessionService, err := services.NewSessionService(sessionRepository, userRepository, time.Now, logger)
	if err != nil {
		return fmt.Errorf("initialize session service: %w", err)
	}

	purged, err := sessionService.Purge(ctx, constant.SessionPurgeBatchSize)
	if err != nil {
		return err
	}

	logger.Info("run finished", "purged", purged, "duration_ms", time.Since(started).Milliseconds())
	return nil
}
