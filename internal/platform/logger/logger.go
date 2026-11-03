// Package logger initializes structured application logging.
//
// Output gives the terminal, plus a file when one is configured. New builds a
// log/slog logger that writes JSON (or text locally) to it, adds the
// request's correlation ID and principal from appcontext to every record, and
// blanks known-sensitive keys. Log IDs, statuses and counts only: never a
// free-text domain field, a credential, or personal data. redact sees keys, not
// string contents, so an error message must not interpolate personal data
// either.
package logger

import (
	"context"
	"fmt"
	"io"
	"log/slog"
	"os"
	"path/filepath"
	"strings"

	"appointment_system/internal/appcontext"
)

const (
	// FormatJSON and FormatText are the accepted Config.Format values.
	FormatJSON = "json"
	FormatText = "text"

	redactedValue = "[REDACTED]"
	// Log files hold audit and request lines, so only the owner and group read them.
	logFileMode = 0o640
	logDirMode  = 0o750
)

// Config selects the level (debug, info, warn, error), the format (json or
// text) and whether records carry their source file and line.
type Config struct {
	Level  string
	Format string
	Source bool
}

// Output returns where logs go: always the terminal (stdout), and also the file
// at path when path is not empty. The file and its folder are created when
// missing, and lines are appended. Processes keep the file open until they
// exit, when the OS closes it, and ignore the closer: writes are unbuffered,
// and the error main logs after its run returns still reaches the file. The
// closer is for callers that outlive the logger, such as tests; it does
// nothing when there is no file.
//
// shortcut: the file is never rotated or trimmed; use logrotate (copytruncate)
// or a collector once it grows, or before keeping logs past their retention.
func Output(path string) (io.Writer, io.Closer, error) {
	if path == "" {
		return os.Stdout, nopCloser{}, nil
	}

	if err := os.MkdirAll(filepath.Dir(path), logDirMode); err != nil {
		return nil, nil, fmt.Errorf("create log folder: %w", err)
	}
	file, err := os.OpenFile(path, os.O_CREATE|os.O_WRONLY|os.O_APPEND, logFileMode)
	if err != nil {
		return nil, nil, fmt.Errorf("open log file: %w", err)
	}

	return io.MultiWriter(os.Stdout, file), file, nil
}

// nopCloser is the closer Output returns when there is no file to close.
type nopCloser struct{}

func (nopCloser) Close() error {
	return nil
}

// New returns a logger writing to w as configured.
func New(config Config, w io.Writer) (*slog.Logger, error) {
	var level slog.Level
	if err := level.UnmarshalText([]byte(config.Level)); err != nil {
		return nil, fmt.Errorf("log level %q: %w", config.Level, err)
	}

	options := &slog.HandlerOptions{Level: level, AddSource: config.Source, ReplaceAttr: redact}
	var handler slog.Handler
	switch config.Format {
	case FormatJSON:
		handler = slog.NewJSONHandler(w, options)
	case FormatText:
		handler = slog.NewTextHandler(w, options)
	default:
		return nil, fmt.Errorf("log format %q is not supported; use %q or %q", config.Format, FormatJSON, FormatText)
	}

	return slog.New(contextHandler{handler}), nil
}

// contextHandler adds the request metadata kept in appcontext to every record
// logged with a context, so callers never pass it themselves.
type contextHandler struct {
	slog.Handler
}

func (ch contextHandler) Handle(ctx context.Context, record slog.Record) error {
	if applicationContext, ok := appcontext.From(ctx); ok {
		if applicationContext.CorrelationID != "" {
			record.AddAttrs(slog.String("correlation_id", applicationContext.CorrelationID))
		}
		if applicationContext.PrincipalID != "" {
			record.AddAttrs(
				slog.String("user_id", applicationContext.PrincipalID),
				slog.String("role", applicationContext.PrincipalRole),
			)
		}
	}

	return ch.Handler.Handle(ctx, record)
}

func (ch contextHandler) WithAttrs(attrs []slog.Attr) slog.Handler {
	return contextHandler{ch.Handler.WithAttrs(attrs)}
}

func (ch contextHandler) WithGroup(name string) slog.Handler {
	return contextHandler{ch.Handler.WithGroup(name)}
}

// redact blanks the value of an attribute whose key names a credential or
// personal data.
func redact(_ []string, attr slog.Attr) slog.Attr {
	switch strings.ToLower(attr.Key) {
	case "password", "token", "access_token", "refresh_token", "authorization", "secret", "signature",
		"email", "mobile", "mobile_no", "diagnosis", "notes", "patient_notes", "payload":
		return slog.String(attr.Key, redactedValue)
	default:
		return attr
	}
}
