// Command bootstrap creates the first admin account. It is the only way to get
// an admin without an existing one, so it refuses to run once any admin exists.
// Later admins are created through POST /api/v1/admin/users.
//
// The password is read from ADMIN_PASSWORD, never from a flag, so it stays out
// of shell history and process listings.
//
//	ADMIN_PASSWORD=... go run ./scripts/bootstrap -name "Ops Admin" -email ops@example.com -mobile 9876543210
package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"log/slog"
	"os"
	"time"

	"appointment_system/config"
	"appointment_system/internal/apps/appointment/apperrors"
	"appointment_system/internal/apps/appointment/models"
	"appointment_system/internal/apps/appointment/repositories"
	"appointment_system/internal/apps/appointment/services"
	"appointment_system/internal/platform/datastore"
	"appointment_system/internal/platform/logger"
)

const (
	bootstrapTimeout = 30 * time.Second
	// jobName tags every log line of this command.
	jobName = "create_admin"
)

func main() {
	if err := run(); err != nil {
		slog.Error("create admin", "error", err)
		os.Exit(1)
	}
}

func run() error {
	name := flag.String("name", "", "admin display name")
	email := flag.String("email", "", "admin email, used to log in")
	mobile := flag.String("mobile", "", "admin mobile number")
	flag.Parse()

	password := os.Getenv("ADMIN_PASSWORD")
	if *name == "" || *email == "" || *mobile == "" || password == "" {
		return errors.New("-name, -email, -mobile and the ADMIN_PASSWORD environment variable are required")
	}

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

	ctx, cancel := context.WithTimeout(context.Background(), bootstrapTimeout)
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

	userRepository, err := repositories.NewUserRepository(store.DB)
	if err != nil {
		return fmt.Errorf("initialize user repository: %w", err)
	}

	adminUserService, err := services.NewAdminUserService(userRepository, logger)
	if err != nil {
		return fmt.Errorf("initialize admin user service: %w", err)
	}

	createdUser, err := adminUserService.CreateFirst(ctx, &models.User{
		Name:     *name,
		Email:    *email,
		MobileNo: *mobile,
	}, password)
	if err != nil {
		if errors.Is(err, apperrors.ErrAdminAlreadyExists) {
			return errors.New("an admin already exists; create more admins through POST /api/v1/admin/users")
		}
		return err
	}

	logger.Info("run finished", "user_id", createdUser.ID)
	return nil
}
