//go:build swagger

package server

import (
	_ "appointment_system/docs/swagger"

	"github.com/gin-gonic/gin"
	swaggerFiles "github.com/swaggo/files"
	ginSwagger "github.com/swaggo/gin-swagger"
)

// registerSwaggerRoute is compiled only with -tags swagger (make run, make up,
// the VS Code launch). Production builds use swagger_off.go and contain no
// Swagger code or route.
func registerSwaggerRoute(router gin.IRoutes) {
	router.GET("/swagger/*any", ginSwagger.WrapHandler(swaggerFiles.Handler))
}
