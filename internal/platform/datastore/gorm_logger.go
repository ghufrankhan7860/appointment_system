package datastore

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"time"

	"github.com/jackc/pgx/v5/pgconn"
	"gorm.io/gorm"
	gormlogger "gorm.io/gorm/logger"
)

const (
	slowQueryThreshold = 200 * time.Millisecond
	// Postgres codes the repositories turn into sentinels (ErrEmailAlreadyExists,
	// ErrSlotTaken and the like), so a clash is an expected outcome, not a fault.
	uniqueViolationCode    = "23505"
	exclusionViolationCode = "23P01"
)

// GORMLogger writes GORM's messages, unexpected query errors and slow queries
// through slog. SQL text is never logged: a WHERE clause can carry personal
// data.
//
// Trace skips record-not-found, cancelled contexts and unique or exclusion
// clashes. That is safe only because repositories map those clashes to
// sentinels; one they do not map still reaches the handler as an error that
// becomes a 500, and the request log records its cause from ctx.Errors.
type GORMLogger struct {
	logger *slog.Logger
	level  gormlogger.LogLevel
}

// NewGORMLogger returns a GORMLogger at the warn level: errors and slow queries.
func NewGORMLogger(logger *slog.Logger) GORMLogger {
	return GORMLogger{logger: logger, level: gormlogger.Warn}
}

// LogMode returns a copy at level, leaving the receiver unchanged.
func (gl GORMLogger) LogMode(level gormlogger.LogLevel) gormlogger.Interface {
	gl.level = level

	return gl
}

func (gl GORMLogger) Info(ctx context.Context, message string, args ...any) {
	if gl.level >= gormlogger.Info {
		gl.logger.InfoContext(ctx, fmt.Sprintf(message, args...))
	}
}

func (gl GORMLogger) Warn(ctx context.Context, message string, args ...any) {
	if gl.level >= gormlogger.Warn {
		gl.logger.WarnContext(ctx, fmt.Sprintf(message, args...))
	}
}

func (gl GORMLogger) Error(ctx context.Context, message string, args ...any) {
	if gl.level >= gormlogger.Error {
		gl.logger.ErrorContext(ctx, fmt.Sprintf(message, args...))
	}
}

// Trace logs an unexpected query error at error and a query slower than
// slowQueryThreshold at warn. rows is -1 when GORM has no row count.
func (gl GORMLogger) Trace(ctx context.Context, begin time.Time, fc func() (sql string, rowsAffected int64), err error) {
	if gl.level <= gormlogger.Silent {
		return
	}

	elapsed := time.Since(begin)
	switch {
	case err != nil && gl.level >= gormlogger.Error && !expectedQueryError(err):
		_, rows := fc()
		attrs := []any{"duration_ms", elapsed.Milliseconds(), "rows", rows, "error", err}
		var pgError *pgconn.PgError
		if errors.As(err, &pgError) {
			attrs = append(attrs, "sqlstate", pgError.Code)
		}
		gl.logger.ErrorContext(ctx, "query failed", attrs...)
	case err == nil && elapsed > slowQueryThreshold && gl.level >= gormlogger.Warn:
		_, rows := fc()
		if rows < 0 {
			return
		}
		gl.logger.WarnContext(ctx, "slow query", "duration_ms", elapsed.Milliseconds(), "rows", rows)
	}
}

func expectedQueryError(err error) bool {
	if errors.Is(err, gorm.ErrRecordNotFound) || errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
		return true
	}

	var pgError *pgconn.PgError

	return errors.As(err, &pgError) && (pgError.Code == uniqueViolationCode || pgError.Code == exclusionViolationCode)
}
