package main

import (
	"fmt"
	"log"

	"github.com/nmslite/nmslite/internal/server"
)

func main() {
	port := "8443"

	fmt.Println("Starting NMSlite Mock API Server...")
	fmt.Printf("Listening on https://localhost:%s\n", port)
	fmt.Println("\nAPI Documentation:")
	fmt.Println("  Base URL: http://localhost:" + port + "/api/v1")
	fmt.Println("\nEndpoints:")
	fmt.Println("  Health: GET http://localhost:" + port + "/health")
	fmt.Println("\n  Auth:")
	fmt.Println("    POST http://localhost:" + port + "/api/v1/auth/login")
	fmt.Println("    POST http://localhost:" + port + "/api/v1/auth/refresh")
	fmt.Println("\n  Credentials (CRUD):")
	fmt.Println("    GET    http://localhost:" + port + "/api/v1/credentials")
	fmt.Println("    POST   http://localhost:" + port + "/api/v1/credentials")
	fmt.Println("    GET    http://localhost:" + port + "/api/v1/credentials/{id}")
	fmt.Println("    PUT    http://localhost:" + port + "/api/v1/credentials/{id}")
	fmt.Println("    DELETE http://localhost:" + port + "/api/v1/credentials/{id}")
	fmt.Println("\n  Devices (CRUD + Provisioning):")
	fmt.Println("    GET    http://localhost:" + port + "/api/v1/devices")
	fmt.Println("    POST   http://localhost:" + port + "/api/v1/devices")
	fmt.Println("    GET    http://localhost:" + port + "/api/v1/devices/{id}")
	fmt.Println("    PUT    http://localhost:" + port + "/api/v1/devices/{id}")
	fmt.Println("    DELETE http://localhost:" + port + "/api/v1/devices/{id}")
	fmt.Println("    POST   http://localhost:" + port + "/api/v1/devices/discover")
	fmt.Println("    POST   http://localhost:" + port + "/api/v1/devices/{id}/provision")
	fmt.Println("    POST   http://localhost:" + port + "/api/v1/devices/{id}/deprovision")
	fmt.Println("\n  Metrics:")
	fmt.Println("    GET  http://localhost:" + port + "/api/v1/devices/{id}/metrics")
	fmt.Println("    POST http://localhost:" + port + "/api/v1/devices/{id}/metrics/history")
	fmt.Println("\n  Test Credentials:")
	fmt.Println("    Username: admin")
	fmt.Println("    Password: secret")
	fmt.Println()

	srv := server.NewServer(port)
	if err := srv.Start(); err != nil {
		log.Fatalf("Server failed to start: %v", err)
	}
}
