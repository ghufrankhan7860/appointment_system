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
	"strings"
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
	defaultDBSSLMode      = "disable"
	defaultDBMaxOpenConns = 25
	defaultDBMaxIdleConns = 5
	defaultJWTSecret      = "appointment-local-jwt-secret-change-me"
	defaultJWTIssuer      = "appointment-system"
	defaultEnvironment    = "development"
	// paymentProviderNone moves no money and is refused in production.
	paymentProviderNone = "none"
	// paymentProviderRazorpay collects payments through Razorpay Checkout.
	paymentProviderRazorpay = "razorpay"
	// minRazorpayWebhookSecretLength keeps the secret that signs Razorpay
	// webhooks out of guessing range.
	minRazorpayWebhookSecretLength = 32
	// Razorpay key ids start with the mode they belong to.
	razorpayTestKeyPrefix = "rzp_test_"
	razorpayLiveKeyPrefix = "rzp_live_"
	// storageProviderLocal keeps uploads on the local disk under StorageConfig.LocalDir.
	storageProviderLocal = "local"
	defaultStorageDir    = "var/storage"
	// defaultLogLevel keeps debug lines out of production unless LOG_LEVEL asks.
	defaultLogLevel = "info"
	// logFormatJSON is for production, where a collector parses each line;
	// logFormatText is easier to read locally.
	logFormatJSON = "json"
	logFormatText = "text"
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
	Storage  StorageConfig
	Log      LogConfig
}

type AppConfig struct {
	Name string
	// Environment is APP_ENV, else GIN_MODE, else development. It is logged at
	// startup and selects production-only rules.
	Environment string
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
	SSLMode         string
	MaxOpenConns    int
	MaxIdleConns    int
	ConnMaxLifetime time.Duration
	ConnectTimeout  time.Duration
}

type PaymentConfig struct {
	// Provider selects the payment gateway adapter.
	Provider string
	// The Razorpay settings are required when Provider is razorpay. The key id
	// is public (Checkout needs it); the key secret and webhook secret are not.
	RazorpayKeyID         string
	RazorpayKeySecret     string
	RazorpayWebhookSecret string
}

type StorageConfig struct {
	// Provider selects the file storage adapter for uploaded medical files.
	Provider string
	// LocalDir is the root folder of the local provider.
	LocalDir string
}

type LogConfig struct {
	// Level is debug, info, warn or error.
	Level string
	// Format is json or text.
	Format string
	// Source adds the file and line of each log call.
	Source bool
	// File, when set, is a path that receives every log line too; logs always
	// go to the terminal. Empty writes no file.
	File string
}

type AuthConfig struct {
	JWTSecret string
	JWTIssuer string
	TokenTTL  time.Duration
}

// Load reads environment variables once and returns typed configuration.
func Load() (Config, error) {
	config := Config{
		App: AppConfig{Name: env("APP_NAME", defaultAppName), Environment: environment()},
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
			SSLMode:         env("DB_SSLMODE", defaultDBSSLMode),
			ConnMaxLifetime: dbConnMaxLifetime,
			ConnectTimeout:  dbConnectTimeout,
		},
		Auth: AuthConfig{
			JWTSecret: env("JWT_SECRET", defaultJWTSecret),
			JWTIssuer: env("JWT_ISSUER", defaultJWTIssuer),
			TokenTTL:  constant.AccessTokenTTL,
		},
		Payment: PaymentConfig{
			Provider:              env("PAYMENT_PROVIDER", paymentProviderNone),
			RazorpayKeyID:         env("RAZORPAY_KEY_ID", ""),
			RazorpayKeySecret:     env("RAZORPAY_KEY_SECRET", ""),
			RazorpayWebhookSecret: env("RAZORPAY_WEBHOOK_SECRET", ""),
		},
		Storage: StorageConfig{
			Provider: env("STORAGE_PROVIDER", storageProviderLocal),
			LocalDir: env("STORAGE_LOCAL_DIR", defaultStorageDir),
		},
		Log: LogConfig{
			Level:  env("LOG_LEVEL", defaultLogLevel),
			Format: env("LOG_FORMAT", logFormatJSON),
			File:   strings.TrimSpace(env("LOG_FILE", "")),
		},
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

	switch config.Log.Level {
	case "debug", "info", "warn", "error":
	default:
		return Config{}, fmt.Errorf("LOG_LEVEL %q is not supported; use debug, info, warn or error", config.Log.Level)
	}
	if config.Log.Format != logFormatJSON && config.Log.Format != logFormatText {
		return Config{}, fmt.Errorf("LOG_FORMAT %q is not supported; use %q or %q", config.Log.Format, logFormatJSON, logFormatText)
	}
	if value, ok := os.LookupEnv("LOG_SOURCE"); ok {
		source, err := strconv.ParseBool(value)
		if err != nil {
			return Config{}, fmt.Errorf("LOG_SOURCE must be true or false: %w", err)
		}
		config.Log.Source = source
	}

	if config.App.Name == "" || config.HTTP.Address == "" {
		return Config{}, fmt.Errorf("APP_NAME and HTTP_ADDRESS are required")
	}
	if config.Database.Host == "" || config.Database.User == "" || config.Database.Name == "" {
		return Config{}, fmt.Errorf("DB_HOST, DB_USER, and DB_NAME are required")
	}
	if config.Database.Port < 1 || config.Database.Port > maxTCPPort {
		return Config{}, fmt.Errorf("DB_PORT must be between 1 and %d", maxTCPPort)
	}
	// allow and prefer silently fall back to plain text, so they are refused.
	switch config.Database.SSLMode {
	case "disable", "require", "verify-ca", "verify-full":
	default:
		return Config{}, fmt.Errorf("DB_SSLMODE %q is not supported; use disable, require, verify-ca or verify-full", config.Database.SSLMode)
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

	switch config.Payment.Provider {
	case paymentProviderNone:
	case paymentProviderRazorpay:
		if config.Payment.RazorpayKeyID == "" || config.Payment.RazorpayKeySecret == "" {
			return Config{}, fmt.Errorf("RAZORPAY_KEY_ID and RAZORPAY_KEY_SECRET are required for the %q payment provider", paymentProviderRazorpay)
		}
		if len(config.Payment.RazorpayWebhookSecret) < minRazorpayWebhookSecretLength {
			return Config{}, fmt.Errorf("RAZORPAY_WEBHOOK_SECRET must contain at least %d characters", minRazorpayWebhookSecretLength)
		}
		// Live keys only in production and test keys never there, so a local
		// run cannot charge real money and production cannot take fake money.
		wantPrefix := razorpayTestKeyPrefix
		if isProductionEnvironment() {
			wantPrefix = razorpayLiveKeyPrefix
		}
		if !strings.HasPrefix(config.Payment.RazorpayKeyID, wantPrefix) {
			return Config{}, fmt.Errorf("RAZORPAY_KEY_ID must start with %q in this environment", wantPrefix)
		}
	default:
		return Config{}, fmt.Errorf("PAYMENT_PROVIDER %q is not supported; use %q or %q", config.Payment.Provider, paymentProviderNone, paymentProviderRazorpay)
	}
	if config.Storage.Provider != storageProviderLocal {
		return Config{}, fmt.Errorf("STORAGE_PROVIDER %q is not supported; use %q", config.Storage.Provider, storageProviderLocal)
	}
	if config.Storage.Provider == storageProviderLocal && config.Storage.LocalDir == "" {
		return Config{}, fmt.Errorf("STORAGE_LOCAL_DIR is required for the %q storage provider", storageProviderLocal)
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

func environment() string {
	return env("APP_ENV", env("GIN_MODE", defaultEnvironment))
}

func isProductionEnvironment() bool {
	switch environment() {
	case "production", "prod", "release":
		return true
	default:
		return false
	}
}
