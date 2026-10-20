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
	"log"
	"net/http"
	"strconv"
	"strings"

	"github.com/gin-gonic/gin"

	"appointment_system/internal/appcontext"
)

const (
	correlationIDHeader = "X-Correlation-ID"
	authorizationHeader = "Authorization"
	bearerScheme        = "Bearer"
	correlationIDBytes = 16
)

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
	Verify(token string) (uint64, string, error)
}

// Authenticate rejects requests without a valid Bearer token and records the
// authenticated user ID and role as the principal of the request-scoped application context.
func Authenticate(verifier TokenVerifier) gin.HandlerFunc {
	return func(ctx *gin.Context) {
		scheme, token, found := strings.Cut(ctx.GetHeader(authorizationHeader), " ")
		if !found || !strings.EqualFold(scheme, bearerScheme) {
			ctx.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{"success": false, "error": "Missing or invalid authorization header"})
			return
		}

		userID, role, err := verifier.Verify(strings.TrimSpace(token))
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
		applicationContext.PrincipalRole = role
		ctx.Next()
	}
}

func newCorrelationID() (string, error) {
	value := make([]byte, correlationIDBytes)
	if _, err := rand.Read(value); err != nil {
		return "", err
	}
	return hex.EncodeToString(value), nil
}

// LogAdminRequest logs who called an admin route and the response status. It
// runs before the permission check so refused attempts are logged too.
func LogAdminRequest() gin.HandlerFunc {
	return func(ctx *gin.Context) {
		ctx.Next()

		userID := ""
		if applicationContext, ok := appcontext.From(ctx.Request.Context()); ok {
			userID = applicationContext.PrincipalID
		}
		log.Printf(
			"admin request user_id=%s method=%s route=%s status=%d",
			userID,
			ctx.Request.Method,
			ctx.FullPath(),
			ctx.Writer.Status(),
		)
	}
}
