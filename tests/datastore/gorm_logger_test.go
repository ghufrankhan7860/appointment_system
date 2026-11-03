package datastore_test

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgconn"
	"gorm.io/gorm"
	gormlogger "gorm.io/gorm/logger"

	"appointment_system/internal/platform/datastore"
	"appointment_system/internal/platform/logger"
)

func newTestGORMLogger(t *testing.T, output *bytes.Buffer) datastore.GORMLogger {
	t.Helper()

	jsonLogger, err := logger.New(logger.Config{Level: "debug", Format: logger.FormatJSON}, output)
	if err != nil {
		t.Fatalf("logger.New returned error: %v", err)
	}

	return datastore.NewGORMLogger(jsonLogger)
}

func TestGORMLoggerTrace(t *testing.T) {
	tests := []struct {
		name         string
		elapsed      time.Duration
		rows         int64
		err          error
		wantMsg      string
		wantLevel    string
		wantSQLState string
	}{
		{name: "fast query logs nothing", elapsed: time.Millisecond, rows: 1},
		{name: "record not found is expected", elapsed: time.Millisecond, rows: 0, err: gorm.ErrRecordNotFound},
		{name: "unique clash is mapped by repositories", elapsed: time.Millisecond, rows: -1, err: fmt.Errorf("create user: %w", &pgconn.PgError{Code: "23505"})},
		{name: "exclusion clash is mapped by repositories", elapsed: time.Millisecond, rows: -1, err: &pgconn.PgError{Code: "23P01"}},
		{name: "cancelled request is expected", elapsed: time.Millisecond, rows: -1, err: context.Canceled},
		{name: "unexpected error logs its SQLSTATE", elapsed: time.Millisecond, rows: -1, err: &pgconn.PgError{Code: "42P01"}, wantMsg: "query failed", wantLevel: "ERROR", wantSQLState: "42P01"},
		{name: "non-Postgres error logs without SQLSTATE", elapsed: time.Millisecond, rows: -1, err: errors.New("driver: bad connection"), wantMsg: "query failed", wantLevel: "ERROR"},
		{name: "slow query warns", elapsed: 300 * time.Millisecond, rows: 12, wantMsg: "slow query", wantLevel: "WARN"},
		{name: "slow query without a row count is skipped", elapsed: 300 * time.Millisecond, rows: -1},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var output bytes.Buffer
			gormLogger := newTestGORMLogger(t, &output)

			gormLogger.Trace(context.Background(), time.Now().Add(-tt.elapsed), func() (string, int64) {
				return "SELECT * FROM users WHERE email = 'a@b.com'", tt.rows
			}, tt.err)

			if tt.wantMsg == "" {
				if output.Len() != 0 {
					t.Fatalf("Trace logged %q, want nothing", output.String())
				}
				return
			}
			var record map[string]any
			if err := json.Unmarshal(bytes.TrimSpace(output.Bytes()), &record); err != nil {
				t.Fatalf("Trace output %q is not one JSON line: %v", output.String(), err)
			}
			if record["msg"] != tt.wantMsg || record["level"] != tt.wantLevel {
				t.Fatalf("record = %v, want %s %q", record, tt.wantLevel, tt.wantMsg)
			}
			if gotSQLState, _ := record["sqlstate"].(string); gotSQLState != tt.wantSQLState {
				t.Fatalf("sqlstate = %q, want %q", gotSQLState, tt.wantSQLState)
			}
			if bytes.Contains(output.Bytes(), []byte("a@b.com")) {
				t.Fatalf("Trace logged the SQL text: %s", output.String())
			}
		})
	}
}

func TestGORMLoggerLogModeReturnsCopy(t *testing.T) {
	var output bytes.Buffer
	gormLogger := newTestGORMLogger(t, &output)

	silent := gormLogger.LogMode(gormlogger.Silent)
	silent.Error(context.Background(), "silenced %d", 1)
	if output.Len() != 0 {
		t.Fatalf("silent copy logged %q", output.String())
	}

	gormLogger.Error(context.Background(), "still logged %d", 2)
	if !bytes.Contains(output.Bytes(), []byte("still logged 2")) {
		t.Fatalf("original logger output = %q, want its level unchanged by LogMode", output.String())
	}
}
