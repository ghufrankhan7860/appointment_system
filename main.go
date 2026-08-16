// Package main is the process entry point. Keep it limited to invoking the
// command registry and converting the final error into a process exit code.
package main

import (
	"log"
	"os"

	"appointment_system/cmd"
)

func main() {
	if err := cmd.Execute(); err != nil {
		log.Printf("application stopped: %v", err)
		os.Exit(1)
	}
}
