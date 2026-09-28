package services

import "context"

// TransactionManager owns the atomic unit of work for multi-repository
// service operations. It is implemented with GORM outside this package;
// the service sees only context propagation, never vendor types.
type TransactionManager interface {
	WithinTransaction(
		ctx context.Context,
		fn func(context.Context) error,
	) error
}
