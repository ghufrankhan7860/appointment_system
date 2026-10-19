//go:build !swagger

package server

import "github.com/gin-gonic/gin"

func registerSwaggerRoute(router gin.IRoutes) {}
