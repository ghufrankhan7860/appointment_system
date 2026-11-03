package config_test

import (
	"strings"
	"testing"

	"appointment_system/config"
)

func TestLoadPaymentProvider(t *testing.T) {
	tests := []struct {
		name        string
		environment string
		provider    string
		wantErr     string
	}{
		{name: "none is the local default", environment: "development"},
		{name: "an unknown provider is refused", environment: "development", provider: "paypal", wantErr: "not supported"},
		{name: "none is refused in production", environment: "production", provider: "none", wantErr: "not allowed in production"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Setenv("APP_ENV", tt.environment)
			t.Setenv("JWT_SECRET", strings.Repeat("s", 40))
			if tt.provider != "" {
				t.Setenv("PAYMENT_PROVIDER", tt.provider)
			}

			loaded, err := config.Load()
			if tt.wantErr != "" {
				if err == nil || !strings.Contains(err.Error(), tt.wantErr) {
					t.Fatalf("Load error = %v, want one containing %q", err, tt.wantErr)
				}
				return
			}
			if err != nil {
				t.Fatalf("Load returned error: %v", err)
			}
			if loaded.Payment.Provider != "none" {
				t.Fatalf("Load payment provider = %q, want none", loaded.Payment.Provider)
			}
		})
	}
}

func TestLoadRazorpayProvider(t *testing.T) {
	secret := strings.Repeat("w", 32)

	tests := []struct {
		name          string
		environment   string
		keyID         string
		keySecret     string
		webhookSecret string
		wantErr       string
	}{
		{name: "test keys in development", environment: "development", keyID: "rzp_test_abc", keySecret: "key-secret", webhookSecret: secret},
		{name: "live keys in production", environment: "production", keyID: "rzp_live_abc", keySecret: "key-secret", webhookSecret: secret},
		{name: "live keys are refused outside production", environment: "development", keyID: "rzp_live_abc", keySecret: "key-secret", webhookSecret: secret, wantErr: `"rzp_test_"`},
		{name: "test keys are refused in production", environment: "production", keyID: "rzp_test_abc", keySecret: "key-secret", webhookSecret: secret, wantErr: `"rzp_live_"`},
		{name: "a missing key secret", environment: "development", keyID: "rzp_test_abc", webhookSecret: secret, wantErr: "RAZORPAY_KEY_SECRET"},
		{name: "a missing key id", environment: "development", keySecret: "key-secret", webhookSecret: secret, wantErr: "RAZORPAY_KEY_ID"},
		{name: "a short webhook secret", environment: "development", keyID: "rzp_test_abc", keySecret: "key-secret", webhookSecret: "short", wantErr: "RAZORPAY_WEBHOOK_SECRET"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Setenv("APP_ENV", tt.environment)
			t.Setenv("JWT_SECRET", strings.Repeat("s", 40))
			t.Setenv("PAYMENT_PROVIDER", "razorpay")
			t.Setenv("RAZORPAY_KEY_ID", tt.keyID)
			t.Setenv("RAZORPAY_KEY_SECRET", tt.keySecret)
			t.Setenv("RAZORPAY_WEBHOOK_SECRET", tt.webhookSecret)

			loaded, err := config.Load()
			if tt.wantErr != "" {
				if err == nil || !strings.Contains(err.Error(), tt.wantErr) {
					t.Fatalf("Load error = %v, want one containing %q", err, tt.wantErr)
				}
				return
			}
			if err != nil {
				t.Fatalf("Load returned error: %v", err)
			}
			if loaded.Payment.Provider != "razorpay" || loaded.Payment.RazorpayKeyID != tt.keyID ||
				loaded.Payment.RazorpayKeySecret != tt.keySecret || loaded.Payment.RazorpayWebhookSecret != tt.webhookSecret {
				t.Fatalf("Load payment provider = %q key id = %q, want razorpay with its settings", loaded.Payment.Provider, loaded.Payment.RazorpayKeyID)
			}
		})
	}
}

func TestLoadStorageProvider(t *testing.T) {
	t.Setenv("JWT_SECRET", strings.Repeat("s", 40))

	t.Setenv("STORAGE_PROVIDER", "s3")
	if _, err := config.Load(); err == nil || !strings.Contains(err.Error(), "STORAGE_PROVIDER") {
		t.Fatalf("Load error = %v, want an unsupported storage provider", err)
	}

	t.Setenv("STORAGE_PROVIDER", "local")
	t.Setenv("STORAGE_LOCAL_DIR", "/data/uploads")
	loaded, err := config.Load()
	if err != nil {
		t.Fatalf("Load returned error: %v", err)
	}
	if loaded.Storage.Provider != "local" || loaded.Storage.LocalDir != "/data/uploads" {
		t.Fatalf("Load storage = %+v, want local at /data/uploads", loaded.Storage)
	}
}

func TestLoadDatabaseSSLMode(t *testing.T) {
	t.Setenv("JWT_SECRET", strings.Repeat("s", 40))

	tests := []struct {
		name    string
		sslMode string
		want    string
		wantErr bool
	}{
		{name: "defaults to disable", want: "disable"},
		{name: "require for a hosted database", sslMode: "require", want: "require"},
		{name: "verify-full", sslMode: "verify-full", want: "verify-full"},
		{name: "prefer can fall back to plain text", sslMode: "prefer", wantErr: true},
		{name: "unknown mode", sslMode: "on", wantErr: true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if tt.sslMode != "" {
				t.Setenv("DB_SSLMODE", tt.sslMode)
			}
			loaded, err := config.Load()
			if tt.wantErr {
				if err == nil || !strings.Contains(err.Error(), "DB_SSLMODE") {
					t.Fatalf("Load error = %v, want an unsupported DB_SSLMODE", err)
				}
				return
			}
			if err != nil {
				t.Fatalf("Load returned error: %v", err)
			}
			if loaded.Database.SSLMode != tt.want {
				t.Fatalf("Load SSLMode = %q, want %q", loaded.Database.SSLMode, tt.want)
			}
		})
	}
}

func TestLoadLogging(t *testing.T) {
	t.Setenv("JWT_SECRET", strings.Repeat("s", 40))

	tests := []struct {
		name       string
		env        map[string]string
		want       config.LogConfig
		wantErrKey string
	}{
		{name: "defaults to json at info", want: config.LogConfig{Level: "info", Format: "json"}},
		{name: "local text with source", env: map[string]string{"LOG_LEVEL": "debug", "LOG_FORMAT": "text", "LOG_SOURCE": "true"}, want: config.LogConfig{Level: "debug", Format: "text", Source: true}},
		{name: "log file path", env: map[string]string{"LOG_FILE": " var/log/appointment.log "}, want: config.LogConfig{Level: "info", Format: "json", File: "var/log/appointment.log"}},
		{name: "unknown level", env: map[string]string{"LOG_LEVEL": "verbose"}, wantErrKey: "LOG_LEVEL"},
		{name: "unknown format", env: map[string]string{"LOG_FORMAT": "xml"}, wantErrKey: "LOG_FORMAT"},
		{name: "source not a boolean", env: map[string]string{"LOG_SOURCE": "sometimes"}, wantErrKey: "LOG_SOURCE"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			for name, value := range tt.env {
				t.Setenv(name, value)
			}
			loaded, err := config.Load()
			if tt.wantErrKey != "" {
				if err == nil || !strings.Contains(err.Error(), tt.wantErrKey) {
					t.Fatalf("Load error = %v, want an invalid %s", err, tt.wantErrKey)
				}
				return
			}
			if err != nil {
				t.Fatalf("Load returned error: %v", err)
			}
			if loaded.Log != tt.want {
				t.Fatalf("Load log = %+v, want %+v", loaded.Log, tt.want)
			}
		})
	}
}
