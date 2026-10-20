// Package main is the process entry point. Keep it limited to invoking the
// command registry and converting the final error into a process exit code.
package main

import (
	"log"
	"os"
	// Embeds the timezone database: the distroless image has no zoneinfo, and
	// APP_TIMEZONE must load there too.
	_ "time/tzdata"

	"appointment_system/cmd"
)

//go:generate swag init -g main.go -o docs/swagger --parseInternal

// @title Appointment System API
// @version 1.0
// @description Appointment management backend: accounts, patients and the user-patient access model.
// @BasePath /api/v1
// @securityDefinitions.apikey BearerAuth
// @in header
// @name Authorization
// @description Type "Bearer" followed by a space and the jwtToken returned by login.
func main() {
	if err := cmd.Execute(); err != nil {
		log.Printf("application stopped: %v", err)
		os.Exit(1)
	}
}
