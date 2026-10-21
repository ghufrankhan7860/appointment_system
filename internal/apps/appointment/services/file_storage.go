package services

import (
	"context"
	"io"
)

// FileStorage keeps uploaded files outside the database. Each provider is one
// adapter under adapters/storage, chosen by configuration, so moving to another
// provider (Cloudflare R2 first) needs no service change. Keys are made by the
// services and never come from a client.
type FileStorage interface {
	// Put stores content under key, replacing nothing: keys are never reused.
	Put(ctx context.Context, key string, content io.Reader, contentType string) error
	// Open returns the stored file, or apperrors.ErrStoredFileNotFound.
	Open(ctx context.Context, key string) (io.ReadCloser, error)
	// Delete removes the file; a missing file is not an error.
	Delete(ctx context.Context, key string) error
}
