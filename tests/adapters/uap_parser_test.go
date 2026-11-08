package adapters_test

import (
	"sync"
	"testing"

	"appointment_system/internal/apps/appointment/adapters/useragent/uap"
	"appointment_system/internal/apps/appointment/models"
)

func TestUAPParserParse(t *testing.T) {
	parser, err := uap.New()
	if err != nil {
		t.Fatalf("New returned error: %v", err)
	}

	tests := []struct {
		name      string
		userAgent string
		want      models.UserAgentDetails
	}{
		{
			name:      "chrome on android phone",
			userAgent: "Mozilla/5.0 (Linux; Android 14; Pixel 8) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/130.0.6723.58 Mobile Safari/537.36",
			want:      models.UserAgentDetails{BrowserName: "Chrome Mobile", BrowserVersion: "130.0", OSName: "Android", OSVersion: "14", DeviceBrand: "Google", DeviceModel: "Pixel 8"},
		},
		{
			name:      "safari on iphone",
			userAgent: "Mozilla/5.0 (iPhone; CPU iPhone OS 17_5 like Mac OS X) AppleWebKit/605.1.15 (KHTML, like Gecko) Version/17.5 Mobile/15E148 Safari/604.1",
			want:      models.UserAgentDetails{BrowserName: "Mobile Safari", BrowserVersion: "17.5", OSName: "iOS", OSVersion: "17.5", DeviceBrand: "Apple", DeviceModel: "iPhone"},
		},
		{
			name:      "chrome on windows",
			userAgent: "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/130.0.0.0 Safari/537.36",
			want:      models.UserAgentDetails{BrowserName: "Chrome", BrowserVersion: "130.0", OSName: "Windows", OSVersion: "10"},
		},
		{name: "unrecognised client leaves parts empty", userAgent: "appointment-load-test", want: models.UserAgentDetails{}},
		{name: "empty header", userAgent: "", want: models.UserAgentDetails{}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := parser.Parse(tt.userAgent); got != tt.want {
				t.Fatalf("Parse = %+v, want %+v", got, tt.want)
			}
		})
	}
}

func TestUAPParserConcurrentUse(t *testing.T) {
	parser, err := uap.New()
	if err != nil {
		t.Fatalf("New returned error: %v", err)
	}

	var wg sync.WaitGroup
	for range 8 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			if got := parser.Parse("Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/130.0.0.0 Safari/537.36"); got.BrowserName != "Chrome" {
				t.Errorf("Parse browser = %q, want Chrome", got.BrowserName)
			}
		}()
	}
	wg.Wait()
}
