// Package appcontext contains framework-independent request metadata propagated
// through context.Context across handlers, controllers, services, and repositories.
package appcontext

import "context"

type contextKey struct{}

// AppContext contains request-scoped state only. Application services and raw
// infrastructure clients must be injected through constructors instead.
type AppContext struct {
	Context       context.Context
	CorrelationID string
	PrincipalID   string
	TenantID      string
}

// New creates request-scoped application context from a parent context.
func New(ctx context.Context, correlationID string) *AppContext {
	if ctx == nil {
		ctx = context.Background()
	}

	return &AppContext{
		Context:       ctx,
		CorrelationID: correlationID,
	}
}

// With stores application context in the standard context passed through layers.
func With(ctx context.Context, appContext *AppContext) context.Context {
	return context.WithValue(ctx, contextKey{}, appContext)
}

// From retrieves request-scoped application context.
func From(ctx context.Context) (*AppContext, bool) {
	appContext, ok := ctx.Value(contextKey{}).(*AppContext)
	return appContext, ok
}
