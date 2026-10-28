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
