package repositories_test

import (
	"appointment_system/config"
	"appointment_system/internal/apps/appointment/apperrors"
	"appointment_system/internal/apps/appointment/models"
	"appointment_system/internal/apps/appointment/repositories"
	"appointment_system/internal/platform/datastore"
	"context"
	"errors"
	"fmt"
	"log/slog"
	"testing"
	"time"

	"gorm.io/gorm"
)

// discardLogger satisfies the datastore and services the database tests build.
var discardLogger = slog.New(slog.DiscardHandler)

func openTestDB(t *testing.T) *gorm.DB {
	t.Helper()

	config, err := config.Load()
	if err != nil {
		t.Skipf("config not loadable, skipping database test: %v", err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()

	store, err := datastore.Open(ctx, datastore.Config{
		Host:            config.Database.Host,
		Port:            config.Database.Port,
		User:            config.Database.User,
		Password:        config.Database.Password,
		Database:        config.Database.Name,
		SSLMode:         config.Database.SSLMode,
		MaxOpenConns:    config.Database.MaxOpenConns,
		MaxIdleConns:    config.Database.MaxIdleConns,
		ConnMaxLifetime: config.Database.ConnMaxLifetime,
		ConnectTimeout:  2 * time.Second,
		Logger:          discardLogger,
	})
	if err != nil {
		t.Skipf("database not reachable, skipping database test (run make db): %v", err)
	}
	t.Cleanup(func() {
		if err := store.Close(); err != nil {
			t.Errorf("close datastore: %v", err)
		}
	})

	return store.DB
}

func newTestUser(t *testing.T, db *gorm.DB) *models.User {
	t.Helper()

	stamp := time.Now().UnixNano()
	user := &models.User{
		Name:         "transaction test",
		Email:        fmt.Sprintf("tx-%d@example.com", stamp),
		MobileNo:     fmt.Sprintf("tx-%d", stamp),
		PasswordHash: []byte("not-a-real-hash"),
		Role:         models.RolePatient,
	}
	t.Cleanup(func() {
		db.Exec("DELETE FROM users WHERE email = ?", user.Email)
	})

	return user
}

func userExists(t *testing.T, users *repositories.UserRepository, email string) bool {
	t.Helper()

	_, err := users.FindByEmail(context.Background(), email)
	if errors.Is(err, apperrors.ErrInvalidCredentials) {
		return false
	}
	if err != nil {
		t.Fatalf("FindByEmail returned error: %v", err)
	}

	return true
}

func TestNewGORMTransactionManagerValidation(t *testing.T) {
	if _, err := repositories.NewGORMTransactionManager(nil); !errors.Is(err, apperrors.ErrNilDB) {
		t.Fatalf("NewGORMTransactionManager error = %v, want %v", err, apperrors.ErrNilDB)
	}
}

func TestGORMTransactionManager(t *testing.T) {
	db := openTestDB(t)

	txManager, err := repositories.NewGORMTransactionManager(db)
	if err != nil {
		t.Fatalf("NewGORMTransactionManager returned error: %v", err)
	}
	users, err := repositories.NewUserRepository(db)
	if err != nil {
		t.Fatalf("NewUserRepository returned error: %v", err)
	}
	errRollback := errors.New("force rollback")

	t.Run("commit keeps the write", func(t *testing.T) {
		user := newTestUser(t, db)

		err := txManager.WithinTransaction(context.Background(), func(txCtx context.Context) error {
			_, err := users.Create(txCtx, user)
			return err
		})
		if err != nil {
			t.Fatalf("WithinTransaction returned error: %v", err)
		}
		if !userExists(t, users, user.Email) {
			t.Fatalf("WithinTransaction did not keep the committed user")
		}
	})

	t.Run("error rolls the write back", func(t *testing.T) {
		user := newTestUser(t, db)

		err := txManager.WithinTransaction(context.Background(), func(txCtx context.Context) error {
			if _, err := users.Create(txCtx, user); err != nil {
				return err
			}
			return errRollback
		})
		if !errors.Is(err, errRollback) {
			t.Fatalf("WithinTransaction error = %v, want %v", err, errRollback)
		}
		if userExists(t, users, user.Email) {
			t.Fatalf("WithinTransaction kept a user after rolling back")
		}
	})

	t.Run("nested call rolls back with its caller", func(t *testing.T) {
		user := newTestUser(t, db)

		err := txManager.WithinTransaction(context.Background(), func(outerCtx context.Context) error {
			err := txManager.WithinTransaction(outerCtx, func(innerCtx context.Context) error {
				_, err := users.Create(innerCtx, user)
				return err
			})
			if err != nil {
				return err
			}
			return errRollback
		})
		if !errors.Is(err, errRollback) {
			t.Fatalf("WithinTransaction error = %v, want %v", err, errRollback)
		}
		if userExists(t, users, user.Email) {
			t.Fatalf("WithinTransaction kept the nested write after the outer call rolled back")
		}
	})

	t.Run("nil function is rejected", func(t *testing.T) {
		if err := txManager.WithinTransaction(context.Background(), nil); err == nil {
			t.Fatalf("WithinTransaction error = nil, want an error for a nil function")
		}
	})
}
