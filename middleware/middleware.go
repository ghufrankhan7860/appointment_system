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

func newCorrelationID() (string, error) {
	value := make([]byte, 16)
	if _, err := rand.Read(value); err != nil {
		return "", err
	}
	return hex.EncodeToString(value), nil
}
