// Package middleware contains global transport concerns such as authentication,
// correlation IDs, request timeouts, recovery, secure headers, CORS, and rate limits.
//
// Middleware may enrich request context and reject malformed or unauthenticated
// requests. It must not implement appointment policy, query databases directly, or
// expose a broad service container through request context.
package middleware

import (
	"crypto/rand"
	"encoding/hex"
	"net/http"
	"strings"

	"github.com/gin-gonic/gin"

	"appointment_system/internal/appcontext"
)

const correlationIDHeader = "X-Correlation-ID"

// AppContext creates and propagates narrow request-scoped application context.
func AppContext() gin.HandlerFunc {
	return func(ctx *gin.Context) {
		correlationID := strings.TrimSpace(ctx.GetHeader(correlationIDHeader))
		if correlationID == "" {
			var err error
			correlationID, err = newCorrelationID()
			if err != nil {
				ctx.AbortWithStatus(http.StatusInternalServerError)
				return
			}
		}

		requestContext := ctx.Request.Context()
		applicationContext := appcontext.New(requestContext, correlationID)
		ctx.Request = ctx.Request.WithContext(appcontext.With(requestContext, applicationContext))
		ctx.Header(correlationIDHeader, correlationID)
		ctx.Next()
	}
}

const userIDKey = "userID"

// RequireAuth rejects requests without a valid Bearer token. verify returns the
// authenticated user ID; handlers read it with UserID.
func RequireAuth(verify func(token string) (uint64, error)) gin.HandlerFunc {
	return func(ctx *gin.Context) {
		scheme, token, ok := strings.Cut(ctx.GetHeader("Authorization"), " ")
		if !ok || !strings.EqualFold(scheme, "Bearer") {
			ctx.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{"success": false, "error": "Missing or invalid authorization header"})
			return
		}
		userID, err := verify(strings.TrimSpace(token))
		if err != nil {
			ctx.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{"success": false, "error": "Invalid or expired token"})
			return
		}
		ctx.Set(userIDKey, userID)
		ctx.Next()
	}
}

// UserID returns the user ID set by RequireAuth.
func UserID(ctx *gin.Context) (uint64, bool) {
	id, ok := ctx.Get(userIDKey)
	if !ok {
		return 0, false
	}
	userID, ok := id.(uint64)
	return userID, ok
}

func newCorrelationID()(string, error) {
	value := make([]byte, 16)
	if _, err := rand.Read(value); err != nil {
		return "", err
	}
	return hex.EncodeToString(value), nil
}
