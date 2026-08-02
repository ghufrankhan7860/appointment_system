// Package server owns HTTP engine construction, versioned route groups, health
// endpoints, startup, and bounded graceful shutdown.
//
// Build this package after handlers exist. Accept handlers and runtime policies
// through a constructor, register routes here, and return lifecycle errors to cmd.
// Do not construct repositories or put business rules in route registration.
package server

import "github.com/gin-gonic/gin"

func AppointmentServer() {
	router := gin.Default()

	
}
