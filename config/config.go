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

const (
	defaultAppName        = "appointment-system"
	defaultHTTPAddress    = ":8080"
	defaultDBHost         = "127.0.0.1"
	defaultDBPort         = 5434
	defaultDBUser         = "appointment"
	defaultDBPassword     = "appointment-local"
	defaultDBName         = "appointment_system"
	defaultDBMaxOpenConns = 25
	defaultDBMaxIdleConns = 5
	defaultJWTSecret      = "appointment-local-jwt-secret-change-me"
	defaultJWTIssuer      = "appointment-system"
	defaultEnvironment    = "development"
	// paymentProviderNone moves no money and is refused in production.
	paymentProviderNone = "none"
)

const (
	readHeaderTimeout = 5 * time.Second
	readTimeout       = 10 * time.Second
	writeTimeout      = 15 * time.Second
	idleTimeout       = 60 * time.Second
	shutdownTimeout   = 10 * time.Second
	dbConnMaxLifetime = 5 * time.Minute
	dbConnectTimeout  = 5 * time.Second
	maxTCPPort        = 65535
)

// Config contains validated settings for the application runtime.
type Config struct {
	App      AppConfig
	HTTP     HTTPConfig
	Database DatabaseConfig
	Auth     AuthConfig
	Payment  PaymentConfig
}

type AppConfig struct {
	Name string
	// Location is the clinic timezone. Doctor availability times are local to it.
	Location *time.Location
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

type PaymentConfig struct {
	// Provider selects the payment gateway adapter.
	Provider string
}

type AuthConfig struct {
	JWTSecret string
	JWTIssuer string
	TokenTTL  time.Duration
}

// Load reads environment variables once and returns typed configuration.
func Load() (Config, error) {
	config := Config{
		App: AppConfig{Name: env("APP_NAME", defaultAppName)},
		HTTP: HTTPConfig{
			Address:           env("HTTP_ADDRESS", defaultHTTPAddress),
			ReadHeaderTimeout: readHeaderTimeout,
			ReadTimeout:       readTimeout,
			WriteTimeout:      writeTimeout,
			IdleTimeout:       idleTimeout,
			ShutdownTimeout:   shutdownTimeout,
		},
		Database: DatabaseConfig{
			Host:            env("DB_HOST", defaultDBHost),
			User:            env("DB_USER", defaultDBUser),
			Password:        env("DB_PASSWORD", defaultDBPassword),
			Name:            env("DB_NAME", defaultDBName),
			ConnMaxLifetime: dbConnMaxLifetime,
			ConnectTimeout:  dbConnectTimeout,
		},
		Auth: AuthConfig{
			JWTSecret: env("JWT_SECRET", defaultJWTSecret),
			JWTIssuer: env("JWT_ISSUER", defaultJWTIssuer),
			TokenTTL:  constant.AccessTokenTTL,
		},
		Payment: PaymentConfig{Provider: env("PAYMENT_PROVIDER", paymentProviderNone)},
	}

	integerValues := []struct {
		name     string
		fallback int
		target   *int
	}{
		{name: "DB_PORT", fallback: defaultDBPort, target: &config.Database.Port},
		{name: "DB_MAX_OPEN_CONNS", fallback: defaultDBMaxOpenConns, target: &config.Database.MaxOpenConns},
		{name: "DB_MAX_IDLE_CONNS", fallback: defaultDBMaxIdleConns, target: &config.Database.MaxIdleConns},
	}

	for _, value := range integerValues {
		parsed, err := envInt(value.name, value.fallback)
		if err != nil {
			return Config{}, err
		}
		*value.target = parsed
	}

	location, err := time.LoadLocation(env("APP_TIMEZONE", constant.DefaultTimezone))
	if err != nil {
		return Config{}, fmt.Errorf("APP_TIMEZONE is invalid: %w", err)
	}
	config.App.Location = location

	if config.App.Name == "" || config.HTTP.Address == "" {
		return Config{}, fmt.Errorf("APP_NAME and HTTP_ADDRESS are required")
	}
	if config.Database.Host == "" || config.Database.User == "" || config.Database.Name == "" {
		return Config{}, fmt.Errorf("DB_HOST, DB_USER, and DB_NAME are required")
	}
	if config.Database.Port < 1 || config.Database.Port > maxTCPPort {
		return Config{}, fmt.Errorf("DB_PORT must be between 1 and %d", maxTCPPort)
	}
	if config.Database.MaxOpenConns < 1 || config.Database.MaxIdleConns < 0 || config.Database.MaxIdleConns > config.Database.MaxOpenConns {
		return Config{}, fmt.Errorf("database pool limits are invalid")
	}
	if len(config.Auth.JWTSecret) < constant.MinJWTSecretLength || config.Auth.JWTIssuer == "" || config.Auth.TokenTTL <= 0 {
		return Config{}, fmt.Errorf("JWT_SECRET must contain at least %d characters and JWT settings must be valid", constant.MinJWTSecretLength)
	}
	if _, explicitlySet := os.LookupEnv("JWT_SECRET"); !explicitlySet {
		if isProductionEnvironment() {
			return Config{}, fmt.Errorf("JWT_SECRET must be set explicitly in production; default local secret is not allowed")
		}
	}

	if config.Payment.Provider != paymentProviderNone {
		return Config{}, fmt.Errorf("PAYMENT_PROVIDER %q is not supported; use %q", config.Payment.Provider, paymentProviderNone)
	}
	if config.Payment.Provider == paymentProviderNone && isProductionEnvironment() {
		return Config{}, fmt.Errorf("PAYMENT_PROVIDER %q takes no money and is not allowed in production", paymentProviderNone)
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

func isProductionEnvironment() bool {
	switch env("APP_ENV", env("GIN_MODE", defaultEnvironment)) {
	case "production", "prod", "release":
		return true
	default:
		return false
	}
}
