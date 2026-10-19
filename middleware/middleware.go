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
	"strconv"
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

type TokenVerifier interface {
	Verify(token string) (uint64, error)
}

// Authenticate rejects requests without a valid Bearer token and records the
// authenticated user as the principal of the request-scoped application context.
func Authenticate(verifier TokenVerifier) gin.HandlerFunc {
	return func(ctx *gin.Context) {
		scheme, token, found := strings.Cut(ctx.GetHeader("Authorization"), " ")
		if !found || !strings.EqualFold(scheme, "Bearer") {
			ctx.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{"success": false, "error": "Missing or invalid authorization header"})
			return
		}

		userID, err := verifier.Verify(strings.TrimSpace(token))
		if err != nil {
			ctx.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{"success": false, "error": "Invalid or expired token"})
			return
		}

		applicationContext, ok := appcontext.From(ctx.Request.Context())
		if !ok {
			ctx.AbortWithStatus(http.StatusInternalServerError)
			return
		}

		applicationContext.PrincipalID = strconv.FormatUint(userID, 10)
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
