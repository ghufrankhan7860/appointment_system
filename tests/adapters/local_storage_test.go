package adapters_test

import (
	"context"
	"errors"
	"io"
	"strings"
	"testing"

	"appointment_system/internal/apps/appointment/adapters/storage/local"
	"appointment_system/internal/apps/appointment/apperrors"
)

func TestLocalStorage(t *testing.T) {
	ctx := context.Background()
	storage, err := local.New(t.TempDir())
	if err != nil {
		t.Fatalf("New returned error: %v", err)
	}

	if err := storage.Put(ctx, "patients/3/report", strings.NewReader("lab values"), "application/pdf"); err != nil {
		t.Fatalf("Put returned error: %v", err)
	}
	file, err := storage.Open(ctx, "patients/3/report")
	if err != nil {
		t.Fatalf("Open returned error: %v", err)
	}
	content, err := io.ReadAll(file)
	file.Close()
	if err != nil || string(content) != "lab values" {
		t.Fatalf("Open read %q, %v; want the stored content", content, err)
	}

	if err := storage.Delete(ctx, "patients/3/report"); err != nil {
		t.Fatalf("Delete returned error: %v", err)
	}
	if err := storage.Delete(ctx, "patients/3/report"); err != nil {
		t.Fatalf("Delete of a missing file returned error: %v", err)
	}
	if _, err := storage.Open(ctx, "patients/3/report"); !errors.Is(err, apperrors.ErrStoredFileNotFound) {
		t.Fatalf("Open after Delete error = %v, want %v", err, apperrors.ErrStoredFileNotFound)
	}

	for _, key := range []string{"", "../outside", "patients/../../outside", "/etc/passwd", `patients\3`} {
		if err := storage.Put(ctx, key, strings.NewReader("x"), "text/plain"); err == nil {
			t.Fatalf("Put with key %q succeeded, want it refused", key)
		}
	}
}
