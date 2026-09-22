// Package config defines typed runtime configuration, loading, defaults, and startup
// validation.
//
// Add fields grouped by concern, such as HTTP, database, Redis, Kafka, and telemetry.
// Load environment values once, validate required URLs, durations, and limits, then
// pass the typed struct into cmd. Business packages must not read environment values
// or call Viper directly.
package config

import (
	"appointment_system/internal/apps/appointment/constant"
	"fmt"
	"os"
	"strconv"
	"time"
)

// Config contains validated settings for the application runtime.
type Config struct {
	App      AppConfig
	HTTP     HTTPConfig
	Database DatabaseConfig
	Auth     AuthConfig
}

type AppConfig struct {
	Name string
}

type HTTPConfig struct {
	Address           string
	ReadHeaderTimeout time.Duration
	ReadTimeout       time.Duration
	WriteTimeout      time.Duration
	IdleTimeout       time.Duration
	ShutdownTimeout   time.Duration
}

type DatabaseConfig struct {
	Host            string
	Port            int
	User            string
	Password        string
	Name            string
	MaxOpenConns    int
	MaxIdleConns    int
	ConnMaxLifetime time.Duration
	ConnectTimeout  time.Duration
}

type AuthConfig struct {
	JWTSecret string
	JWTIssuer string
	TokenTTL  time.Duration
}

// Load reads environment variables once and returns typed configuration.
func Load() (Config, error) {
	config := Config{
		App: AppConfig{Name: env("APP_NAME", "appointment-system")},
		HTTP: HTTPConfig{
			Address:           env("HTTP_ADDRESS", ":8080"),
			ReadHeaderTimeout: 5 * time.Second,
			ReadTimeout:       10 * time.Second,
			WriteTimeout:      15 * time.Second,
			IdleTimeout:       60 * time.Second,
			ShutdownTimeout:   10 * time.Second,
		},
		Database: DatabaseConfig{
			Host:            env("DB_HOST", "127.0.0.1"),
			User:            env("DB_USER", "appointment"),
			Password:        env("DB_PASSWORD", "appointment-local"),
			Name:            env("DB_NAME", "appointment_system"),
			ConnMaxLifetime: 5 * time.Minute,
			ConnectTimeout:  5 * time.Second,
		},
		Auth: AuthConfig{
			JWTSecret: env("JWT_SECRET", "appointment-local-jwt-secret-change-me"),
			JWTIssuer: env("JWT_ISSUER", "appointment-system"),
			TokenTTL:  constant.ACCESS_TOKEN_EXPIRATION * time.Second,
		},
	}

	integerValues := []struct {
		name     string
		fallback int
		target   *int
	}{
		{name: "DB_PORT", fallback: 5434, target: &config.Database.Port},
		{name: "DB_MAX_OPEN_CONNS", fallback: 25, target: &config.Database.MaxOpenConns},
		{name: "DB_MAX_IDLE_CONNS", fallback: 5, target: &config.Database.MaxIdleConns},
	}

	for _, value := range integerValues {
		parsed, err := envInt(value.name, value.fallback)
		if err != nil {
			return Config{}, err
		}
		*value.target = parsed
	}

	if config.App.Name == "" || config.HTTP.Address == "" {
		return Config{}, fmt.Errorf("APP_NAME and HTTP_ADDRESS are required")
	}
	if config.Database.Host == "" || config.Database.User == "" || config.Database.Name == "" {
		return Config{}, fmt.Errorf("DB_HOST, DB_USER, and DB_NAME are required")
	}
	if config.Database.Port < 1 || config.Database.Port > 65535 {
		return Config{}, fmt.Errorf("DB_PORT must be between 1 and 65535")
	}
	if config.Database.MaxOpenConns < 1 || config.Database.MaxIdleConns < 0 || config.Database.MaxIdleConns > config.Database.MaxOpenConns {
		return Config{}, fmt.Errorf("database pool limits are invalid")
	}
	if len(config.Auth.JWTSecret) < 32 || config.Auth.JWTIssuer == "" || config.Auth.TokenTTL <= 0 {
		return Config{}, fmt.Errorf("JWT_SECRET must contain at least 32 characters and JWT settings must be valid")
	}

	return config, nil
}

func env(name, fallback string) string {
	if value, ok := os.LookupEnv(name); ok {
		return value
	}
	return fallback
}

func envInt(name string, fallback int) (int, error) {
	value, ok := os.LookupEnv(name)
	if !ok {
		return fallback, nil
	}

	parsed, err := strconv.Atoi(value)
	if err != nil {
		return 0, fmt.Errorf("%s must be an integer: %w", name, err)
	}
	return parsed, nil
}
