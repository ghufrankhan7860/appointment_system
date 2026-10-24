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

func TestLoadYouPIProvider(t *testing.T) {
	secret := strings.Repeat("w", 32)

	tests := []struct {
		name       string
		baseURL    string
		merchantID string
		secret     string
		returnURL  string
		wantErr    string
	}{
		{name: "youpi with every setting", baseURL: "https://pay.example.com", merchantID: "merchant-1", secret: secret, returnURL: "https://app.example.com/payments/return"},
		{name: "a missing merchant id", baseURL: "https://pay.example.com", secret: secret, returnURL: "https://app.example.com/return", wantErr: "YOUPI_MERCHANT_ID"},
		{name: "a short webhook secret", baseURL: "https://pay.example.com", merchantID: "merchant-1", secret: "short", returnURL: "https://app.example.com/return", wantErr: "YOUPI_WEBHOOK_SECRET"},
		{name: "a relative base url", baseURL: "pay.example.com", merchantID: "merchant-1", secret: secret, returnURL: "https://app.example.com/return", wantErr: "YOUPI_BASE_URL"},
		{name: "a missing return url", baseURL: "https://pay.example.com", merchantID: "merchant-1", secret: secret, wantErr: "PAYMENT_RETURN_URL"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Setenv("APP_ENV", "production")
			t.Setenv("JWT_SECRET", strings.Repeat("s", 40))
			t.Setenv("PAYMENT_PROVIDER", "youpi")
			t.Setenv("YOUPI_BASE_URL", tt.baseURL)
			t.Setenv("YOUPI_MERCHANT_ID", tt.merchantID)
			t.Setenv("YOUPI_WEBHOOK_SECRET", tt.secret)
			t.Setenv("PAYMENT_RETURN_URL", tt.returnURL)

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
			if loaded.Payment.Provider != "youpi" || loaded.Payment.YouPIWebhookSecret != secret || loaded.Payment.ReturnURL != tt.returnURL {
				t.Fatalf("Load payment = %+v, want youpi with its settings", loaded.Payment)
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
