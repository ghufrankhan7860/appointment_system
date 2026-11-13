// Package cmd owns runtime selection and composition roots.
//
// Add commands in this order:
//  1. Load and validate typed config.
//  2. Initialize logging and telemetry.
//  3. Open infrastructure clients with bounded timeouts.
//  4. Construct repositories, services, controllers, and delivery adapters.
//  5. Run the selected mode and close resources in reverse order.
//
// Use Cobra when the first real server, consumer, cron, or backfill command is
// introduced. Concrete implementations may be selected here because this is a
// composition root.
package cmd

import (
	"context"
	"fmt"
	"log/slog"
	"os/signal"
	"runtime/debug"
	"syscall"

	"appointment_system/config"
	"appointment_system/internal/apps/appointment"
	"appointment_system/internal/platform/datastore"
	"appointment_system/internal/platform/logger"
	"appointment_system/server"
)

// Execute builds and runs the HTTP application until an interrupt is received.
func Execute() error {
	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	config, err := config.Load()
	if err != nil {
		return fmt.Errorf("load configuration: %w", err)
	}

	logOutput, _, err := logger.Output(config.Log.File)
	if err != nil {
		return fmt.Errorf("initialize log output: %w", err)
	}
	logger, err := logger.New(logger.Config{Level: config.Log.Level, Format: config.Log.Format, Source: config.Log.Source}, logOutput)
	if err != nil {
		return fmt.Errorf("initialize logger: %w", err)
	}
	slog.SetDefault(logger)

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

	app, err := appointment.NewContainer(
		store.DB,
		config.Auth.JWTSecret,
		config.Auth.JWTIssuer,
		config.Auth.TokenTTL,
		config.App.Location,
		appointment.PaymentSettings{
			Methods:               config.Payment.Methods,
			Provider:              config.Payment.Provider,
			RazorpayKeyID:         config.Payment.RazorpayKeyID,
			RazorpayKeySecret:     config.Payment.RazorpayKeySecret,
			RazorpayWebhookSecret: config.Payment.RazorpayWebhookSecret,
		},
		appointment.CallSettings{
			STUNURLs:   config.Call.STUNURLs,
			TURNURLs:   config.Call.TURNURLs,
			TURNSecret: config.Call.TURNSecret,
		},
		config.Storage.Provider,
		config.Storage.LocalDir,
		logger,
	)
	if err != nil {
		return fmt.Errorf("initialize appointment application: %w", err)
	}
	defer app.Close()

	httpServer, err := server.New(server.Config{
		Address:           config.HTTP.Address,
		ReadHeaderTimeout: config.HTTP.ReadHeaderTimeout,
		ReadTimeout:       config.HTTP.ReadTimeout,
		WriteTimeout:      config.HTTP.WriteTimeout,
		IdleTimeout:       config.HTTP.IdleTimeout,
		ShutdownTimeout:   config.HTTP.ShutdownTimeout,
		TrustedProxies:    config.HTTP.TrustedProxies,
	}, logger, store, app)
	if err != nil {
		return fmt.Errorf("initialize HTTP server: %w", err)
	}

	logger.Info("server started",
		"app", config.App.Name,
		"address", config.HTTP.Address,
		"env", config.App.Environment,
		"log_level", config.Log.Level,
		"log_file", config.Log.File,
		"version", buildRevision(),
	)
	return httpServer.Run(ctx)
}

// buildRevision returns the VCS revision the binary was built from, or
// "unknown" when the build did not record one (go run, or a build without .git).
func buildRevision() string {
	if info, ok := debug.ReadBuildInfo(); ok {
		for _, setting := range info.Settings {
			if setting.Key == "vcs.revision" {
				return setting.Value
			}
		}
	}

	return "unknown"
}
