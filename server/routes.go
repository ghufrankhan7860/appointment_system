package server

import (
	"context"
	"net/http"
	"time"

	"appointment_system/middleware"

	"github.com/gin-gonic/gin"
)

const healthCheckTimeout = 2 * time.Second

// HealthChecker describes the dependency readiness behavior consumed by HTTP.
type HealthChecker interface {
	Ping(ctx context.Context) error
}

// RouteRegistrar registers one application's HTTP routes under the API group.
type RouteRegistrar interface {
	RegisterRoutes(router *gin.RouterGroup)
}

// NewRouter constructs the HTTP transport and registers global middleware and routes.
func NewRouter(healthChecker HealthChecker, appRoutes RouteRegistrar) *gin.Engine {
	registerJSONFieldNames()

	router := gin.New()
	router.Use(
		gin.Logger(),
		gin.Recovery(),
		middleware.AppContext(),
	)

	registerHealthRoute(router, healthChecker)
	registerSwaggerRoute(router)

	api := router.Group("/api/v1")
	appRoutes.RegisterRoutes(api)
	return router
}

func registerHealthRoute(router gin.IRoutes, healthChecker HealthChecker) {
	router.GET("/health", func(ctx *gin.Context) {
		checkContext, cancel := context.WithTimeout(ctx.Request.Context(), healthCheckTimeout)
		defer cancel()

		if err := healthChecker.Ping(checkContext); err != nil {
			ctx.JSON(http.StatusServiceUnavailable, gin.H{
				"status":   "unavailable",
				"database": "unavailable",
			})
			return
		}

		ctx.JSON(http.StatusOK, gin.H{
			"status":   "ok",
			"database": "ok",
		})
	})
}
