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
	"log/slog"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/gin-gonic/gin"

	"appointment_system/internal/appcontext"
)

const (
	correlationIDHeader = "X-Correlation-ID"
	authorizationHeader = "Authorization"
	bearerScheme        = "Bearer"
	correlationIDBytes = 16
	// unmatchedRoute names requests no route matched, so their raw paths stay
	// out of the log.
	unmatchedRoute = "unmatched"
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

// TokenVerifier checks an access token and returns the user ID, role and session
// ID it carries (0 when the token predates session IDs).
type TokenVerifier interface {
	Verify(token string) (uint64, string, uint64, error)
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

		userID, role, sessionID, err := verifier.Verify(strings.TrimSpace(token))
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
		if sessionID != 0 {
			applicationContext.SessionID = strconv.FormatUint(sessionID, 10)
		}
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

// LogRequest writes one line per request after it is served: the route
// pattern (never the raw path, which carries IDs), status, duration and size.
// 5xx lines are errors and carry every error a handler attached with
// ctx.Error, so the cause of a 500 is logged here once. 4xx lines are
// warnings. The logger adds the correlation ID and principal from the request
// context, so AppContext must run first.
func LogRequest(logger *slog.Logger) gin.HandlerFunc {
	return func(ctx *gin.Context) {
		started := time.Now()
		ctx.Next()

		status := ctx.Writer.Status()
		level := slog.LevelInfo
		switch {
		case status >= http.StatusInternalServerError:
			level = slog.LevelError
		case status >= http.StatusBadRequest:
			level = slog.LevelWarn
		}

		attrs := []any{
			"method", ctx.Request.Method,
			"route", routePattern(ctx),
			"status", status,
			"duration_ms", time.Since(started).Milliseconds(),
			"bytes", ctx.Writer.Size(),
			"client_ip", ctx.ClientIP(),
		}
		if len(ctx.Errors) > 0 {
			attrs = append(attrs, "error", strings.Join(ctx.Errors.Errors(), "; "))
		}
		logger.Log(ctx.Request.Context(), level, "request served", attrs...)
	}
}

// LogAdminRequest writes an audit line for who called an admin route and the
// response status. It runs before the permission check so refused attempts
// are logged too.
func LogAdminRequest(logger *slog.Logger) gin.HandlerFunc {
	return func(ctx *gin.Context) {
		ctx.Next()

		logger.InfoContext(
			ctx.Request.Context(),
			"admin request",
			"audit", true,
			"method", ctx.Request.Method,
			"route", routePattern(ctx),
			"status", ctx.Writer.Status(),
		)
	}
}

func routePattern(ctx *gin.Context) string {
	if route := ctx.FullPath(); route != "" {
		return route
	}

	return unmatchedRoute
}
