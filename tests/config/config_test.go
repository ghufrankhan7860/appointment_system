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
