// Package server owns HTTP engine construction, versioned route groups, health
// endpoints, startup, and bounded graceful shutdown.
//
// Build this package after handlers exist. Accept handlers and runtime policies
// through a constructor, register routes here, and return lifecycle errors to cmd.
// Do not construct repositories or put business rules in route registration.
package server

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"time"
)

// Config defines HTTP listener and graceful-shutdown policy supplied by cmd.
type Config struct {
	Address           string
	ReadHeaderTimeout time.Duration
	ReadTimeout       time.Duration
	WriteTimeout      time.Duration
	IdleTimeout       time.Duration
	ShutdownTimeout   time.Duration
	// TrustedProxies are the proxies whose X-Forwarded-For gives the client IP;
	// empty trusts none.
	TrustedProxies []string
}

// Server owns the HTTP server lifecycle.
type Server struct {
	httpServer      *http.Server
	shutdownTimeout time.Duration
	logger          *slog.Logger
}

// New constructs a server from validated runtime policy.
func New(config Config, logger *slog.Logger, healthChecker HealthChecker, appRoutes RouteRegistrar) (*Server, error) {
	if config.Address == "" {
		return nil, errors.New("server address is required")
	}
	if config.ReadHeaderTimeout <= 0 || config.ReadTimeout <= 0 || config.WriteTimeout <= 0 || config.IdleTimeout <= 0 || config.ShutdownTimeout <= 0 {
		return nil, errors.New("server timeouts must be greater than zero")
	}
	if logger == nil {
		return nil, errors.New("logger is required")
	}
	if healthChecker == nil {
		return nil, errors.New("health checker is required")
	}
	if appRoutes == nil {
		return nil, errors.New("app routes are required")
	}

	router := NewRouter(logger, healthChecker, appRoutes)
	// gin trusts every proxy by default, which lets any client fake its IP with
	// X-Forwarded-For; nil trusts none.
	if err := router.SetTrustedProxies(config.TrustedProxies); err != nil {
		return nil, fmt.Errorf("set trusted proxies: %w", err)
	}

	return &Server{
		httpServer: &http.Server{
			Addr:              config.Address,
			Handler:           router,
			ReadHeaderTimeout: config.ReadHeaderTimeout,
			ReadTimeout:       config.ReadTimeout,
			WriteTimeout:      config.WriteTimeout,
			IdleTimeout:       config.IdleTimeout,
		},
		shutdownTimeout: config.ShutdownTimeout,
		logger:          logger,
	}, nil
}

// Run serves HTTP until the parent context is canceled or the server fails.
func (server *Server) Run(ctx context.Context) error {
	serveErrors := make(chan error, 1)
	go func() {
		serveErrors <- server.httpServer.ListenAndServe()
	}()

	select {
	case err := <-serveErrors:
		if errors.Is(err, http.ErrServerClosed) {
			return nil
		}
		return fmt.Errorf("serve HTTP: %w", err)
	case <-ctx.Done():
		server.logger.Info("server stopping", "reason", "shutdown signal")
		shutdownContext, cancel := context.WithTimeout(context.Background(), server.shutdownTimeout)
		defer cancel()

		if err := server.httpServer.Shutdown(shutdownContext); err != nil {
			return fmt.Errorf("shutdown HTTP server: %w", err)
		}
		return nil
	}
}
