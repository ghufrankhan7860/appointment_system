package repositories

import (
	"appointment_system/internal/apps/appointment/apperrors"
	"context"
	"fmt"

	"gorm.io/gorm"
)

type txContextKey struct{}

// withTx returns a context carrying the GORM transaction. Repositories
// resolve it via dbFromContext so multi-repository service operations share
// one transaction without leaking GORM into service signatures.
func withTx(ctx context.Context, tx *gorm.DB) context.Context {
	return context.WithValue(ctx, txContextKey{}, tx)
}

func txFromContext(ctx context.Context) (*gorm.DB, bool) {
	tx, ok := ctx.Value(txContextKey{}).(*gorm.DB)
	return tx, ok && tx != nil
}

func dbFromContext(ctx context.Context, db *gorm.DB) *gorm.DB {
	if tx, ok := txFromContext(ctx); ok {
		return tx.WithContext(ctx)
	}
	return db.WithContext(ctx)
}

// GORMTransactionManager implements the service-owned transaction boundary
// (see services.TransactionManager) using GORM. It stays in the repository
// layer so services never import GORM.
type GORMTransactionManager struct {
	db *gorm.DB
}

func NewGORMTransactionManager(db *gorm.DB) (*GORMTransactionManager, error) {
	if db == nil {
		return nil, apperrors.ErrNilDB
	}
	return &GORMTransactionManager{db: db}, nil
}

func (m *GORMTransactionManager) WithinTransaction(ctx context.Context, fn func(context.Context) error) error {
	if err := ctx.Err(); err != nil {
		return fmt.Errorf("transaction context: %w", err)
	}
	if fn == nil {
		return fmt.Errorf("transaction function is nil")
	}
	return m.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		return fn(withTx(ctx, tx))
	})
}
