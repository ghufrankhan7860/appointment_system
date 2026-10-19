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
	"log"
	"os/signal"
	"syscall"

	"appointment_system/config"
	"appointment_system/internal/apps/appointment"
	"appointment_system/internal/platform/datastore"
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

	app, err := appointment.NewContainer(
		store.DB,
		config.Auth.JWTSecret,
		config.Auth.JWTIssuer,
		config.Auth.TokenTTL,
	)
	if err != nil {
		return fmt.Errorf("initialize appointment application: %w", err)
	}

	httpServer, err := server.New(server.Config{
		Address:           config.HTTP.Address,
		ReadHeaderTimeout: config.HTTP.ReadHeaderTimeout,
		ReadTimeout:       config.HTTP.ReadTimeout,
		WriteTimeout:      config.HTTP.WriteTimeout,
		IdleTimeout:       config.HTTP.IdleTimeout,
		ShutdownTimeout:   config.HTTP.ShutdownTimeout,
	}, store, app)
	if err != nil {
		return fmt.Errorf("initialize HTTP server: %w", err)
	}

	log.Printf("starting %s HTTP server on %s", config.App.Name, config.HTTP.Address)
	return httpServer.Run(ctx)
}
