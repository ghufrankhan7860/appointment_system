// Package local stores files on the local disk under one root folder. It suits
// development and a single server with a persistent volume; files written in a
// container without a volume are lost when the container goes away.
package local

import (
	"context"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"strings"

	"appointment_system/internal/apps/appointment/apperrors"
)

const (
	ProviderName   = "local"
	dirPermission  = 0o750
	tempFilePrefix = ".upload-"
)

type Storage struct {
	root string
}

// New creates the root folder if it does not exist yet.
func New(root string) (*Storage, error) {
	if strings.TrimSpace(root) == "" {
		return nil, errors.New("local storage root is empty")
	}

	absoluteRoot, err := filepath.Abs(root)
	if err != nil {
		return nil, fmt.Errorf("resolve local storage root: %w", err)
	}
	if err := os.MkdirAll(absoluteRoot, dirPermission); err != nil {
		return nil, fmt.Errorf("create local storage root: %w", err)
	}

	return &Storage{root: absoluteRoot}, nil
}

// Put writes to a temporary file in the target folder and renames it into
// place, so a failed upload never leaves half a file under the key.
func (s *Storage) Put(ctx context.Context, key string, content io.Reader, contentType string) error {
	if err := ctx.Err(); err != nil {
		return fmt.Errorf("put file context: %w", err)
	}

	path, err := s.path(key)
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(path), dirPermission); err != nil {
		return fmt.Errorf("create file folder: %w", err)
	}

	temp, err := os.CreateTemp(filepath.Dir(path), tempFilePrefix)
	if err != nil {
		return fmt.Errorf("create temporary file: %w", err)
	}
	defer os.Remove(temp.Name())

	if _, err := io.Copy(temp, content); err != nil {
		temp.Close()
		return fmt.Errorf("write file: %w", err)
	}
	if err := temp.Close(); err != nil {
		return fmt.Errorf("close file: %w", err)
	}
	if err := os.Rename(temp.Name(), path); err != nil {
		return fmt.Errorf("move file into place: %w", err)
	}

	return nil
}

func (s *Storage) Open(ctx context.Context, key string) (io.ReadCloser, error) {
	if err := ctx.Err(); err != nil {
		return nil, fmt.Errorf("open file context: %w", err)
	}

	path, err := s.path(key)
	if err != nil {
		return nil, err
	}

	file, err := os.Open(path)
	if err != nil {
		if errors.Is(err, fs.ErrNotExist) {
			return nil, apperrors.ErrStoredFileNotFound
		}
		return nil, fmt.Errorf("open file: %w", err)
	}

	return file, nil
}

func (s *Storage) Delete(ctx context.Context, key string) error {
	path, err := s.path(key)
	if err != nil {
		return err
	}

	if err := os.Remove(path); err != nil && !errors.Is(err, fs.ErrNotExist) {
		return fmt.Errorf("delete file: %w", err)
	}

	return nil
}

// path maps a key such as "patients/3/ab12..." to a file under the root and
// refuses any key that would land outside it.
func (s *Storage) path(key string) (string, error) {
	if key == "" || strings.HasPrefix(key, "/") || filepath.IsAbs(key) || strings.Contains(key, "..") || strings.Contains(key, "\\") {
		return "", fmt.Errorf("storage key %q is not allowed", key)
	}

	path := filepath.Join(s.root, filepath.FromSlash(key))
	if !strings.HasPrefix(path, s.root+string(filepath.Separator)) {
		return "", fmt.Errorf("storage key %q is not allowed", key)
	}

	return path, nil
}
