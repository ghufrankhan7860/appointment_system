package services

import "context"

// Logger records the domain events services emit. It is implemented by
// *slog.Logger from internal/platform/logger, which adds the request's
// correlation ID and principal from ctx. Records carry IDs, statuses and
// counts only, never personal data or free text.
type Logger interface {
	DebugContext(ctx context.Context, msg string, args ...any)
	InfoContext(ctx context.Context, msg string, args ...any)
	WarnContext(ctx context.Context, msg string, args ...any)
}
