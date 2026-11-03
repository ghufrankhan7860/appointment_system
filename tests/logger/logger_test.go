package logger_test

import (
	"bytes"
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"appointment_system/internal/appcontext"
	"appointment_system/internal/platform/logger"
)

func decodeLine(t *testing.T, output *bytes.Buffer) map[string]any {
	t.Helper()

	var record map[string]any
	if err := json.Unmarshal(bytes.TrimSpace(output.Bytes()), &record); err != nil {
		t.Fatalf("log output %q is not one JSON line: %v", output.String(), err)
	}

	return record
}

func TestNewRejectsBadConfig(t *testing.T) {
	tests := []struct {
		name   string
		config logger.Config
	}{
		{name: "unknown level", config: logger.Config{Level: "loud", Format: logger.FormatJSON}},
		{name: "unknown format", config: logger.Config{Level: "info", Format: "xml"}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if _, err := logger.New(tt.config, &bytes.Buffer{}); err == nil {
				t.Fatalf("New(%+v) returned no error", tt.config)
			}
		})
	}
}

func TestLoggerAddsRequestContext(t *testing.T) {
	var output bytes.Buffer
	jsonLogger, err := logger.New(logger.Config{Level: "info", Format: logger.FormatJSON}, &output)
	if err != nil {
		t.Fatalf("New returned error: %v", err)
	}

	applicationContext := appcontext.New(context.Background(), "corr-1")
	applicationContext.PrincipalID = "7"
	applicationContext.PrincipalRole = "patient"
	ctx := appcontext.With(context.Background(), applicationContext)

	jsonLogger.InfoContext(ctx, "appointment booked", "appointment_id", 50)

	record := decodeLine(t, &output)
	if record["correlation_id"] != "corr-1" || record["user_id"] != "7" || record["role"] != "patient" || record["appointment_id"] != float64(50) {
		t.Fatalf("record = %v, want correlation_id, user_id, role and appointment_id", record)
	}
}

func TestLoggerWithoutRequestContext(t *testing.T) {
	var output bytes.Buffer
	jsonLogger, err := logger.New(logger.Config{Level: "info", Format: logger.FormatJSON}, &output)
	if err != nil {
		t.Fatalf("New returned error: %v", err)
	}

	jsonLogger.With("job", "expire_payments").InfoContext(context.Background(), "run finished", "expired", 2)

	record := decodeLine(t, &output)
	if _, ok := record["correlation_id"]; ok {
		t.Fatalf("record = %v, want no correlation_id outside a request", record)
	}
	if record["job"] != "expire_payments" || record["expired"] != float64(2) {
		t.Fatalf("record = %v, want the job and count", record)
	}
}

func TestLoggerRedactsSensitiveKeys(t *testing.T) {
	var output bytes.Buffer
	jsonLogger, err := logger.New(logger.Config{Level: "info", Format: logger.FormatJSON}, &output)
	if err != nil {
		t.Fatalf("New returned error: %v", err)
	}

	jsonLogger.Info("login",
		"password", "hunter22",
		"Email", "a@b.com",
		"token", "abc",
		"patient_notes", "fever",
		"appointment_id", 50,
	)

	record := decodeLine(t, &output)
	for _, key := range []string{"password", "Email", "token", "patient_notes"} {
		if record[key] != "[REDACTED]" {
			t.Fatalf("%s = %v, want [REDACTED]", key, record[key])
		}
	}
	if record["appointment_id"] != float64(50) {
		t.Fatalf("appointment_id = %v, want 50 kept", record["appointment_id"])
	}
	for _, secret := range []string{"hunter22", "a@b.com", "fever"} {
		if strings.Contains(output.String(), secret) {
			t.Fatalf("output %q leaks %q", output.String(), secret)
		}
	}
}

func TestOutputWithoutFile(t *testing.T) {
	output, closer, err := logger.Output("")
	if err != nil {
		t.Fatalf("Output returned error: %v", err)
	}
	if err := closer.Close(); err != nil {
		t.Fatalf("Close returned error: %v", err)
	}
	if output != os.Stdout {
		t.Fatalf("Output(\"\") = %v, want stdout only", output)
	}
}

func TestOutputAppendsToFile(t *testing.T) {
	path := filepath.Join(t.TempDir(), "log", "appointment.log")
	if err := os.MkdirAll(filepath.Dir(path), 0o750); err != nil {
		t.Fatalf("MkdirAll returned error: %v", err)
	}
	if err := os.WriteFile(path, []byte(`{"msg":"earlier run"}`+"\n"), 0o640); err != nil {
		t.Fatalf("WriteFile returned error: %v", err)
	}

	output, closer, err := logger.Output(path)
	if err != nil {
		t.Fatalf("Output returned error: %v", err)
	}
	t.Cleanup(func() { _ = closer.Close() })
	jsonLogger, err := logger.New(logger.Config{Level: "info", Format: logger.FormatJSON}, output)
	if err != nil {
		t.Fatalf("New returned error: %v", err)
	}
	jsonLogger.Info("run finished", "expired", 2)

	content, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("ReadFile returned error: %v", err)
	}
	lines := strings.Split(strings.TrimSpace(string(content)), "\n")
	if len(lines) != 2 || lines[0] != `{"msg":"earlier run"}` {
		t.Fatalf("log file = %q, want the earlier line kept and one new line appended", content)
	}
	var record map[string]any
	if err := json.Unmarshal([]byte(lines[1]), &record); err != nil {
		t.Fatalf("appended line %q is not JSON: %v", lines[1], err)
	}
	if record["msg"] != "run finished" || record["expired"] != float64(2) {
		t.Fatalf("appended record = %v, want the run finished line", record)
	}
}

func TestOutputCreatesFolder(t *testing.T) {
	path := filepath.Join(t.TempDir(), "nested", "log", "appointment.log")

	_, closer, err := logger.Output(path)
	if err != nil {
		t.Fatalf("Output returned error: %v", err)
	}
	t.Cleanup(func() { _ = closer.Close() })
	if _, err := os.Stat(path); err != nil {
		t.Fatalf("log file not created: %v", err)
	}
}

func TestLoggerLevelFilters(t *testing.T) {
	var output bytes.Buffer
	jsonLogger, err := logger.New(logger.Config{Level: "warn", Format: logger.FormatJSON}, &output)
	if err != nil {
		t.Fatalf("New returned error: %v", err)
	}

	jsonLogger.Info("hidden")
	if output.Len() != 0 {
		t.Fatalf("info logged at warn level: %q", output.String())
	}
	jsonLogger.Warn("shown")
	if record := decodeLine(t, &output); record["msg"] != "shown" {
		t.Fatalf("record = %v, want the warning", record)
	}
}
