package main

import (
	"log"

	"github.com/unicornultrafoundation/subnet-rayai-node/api"
	"github.com/unicornultrafoundation/subnet-rayai-node/config"
)

func main() {
	// Load configuration
	cfg, err := config.Load()
	if err != nil {
		log.Fatalf("Failed to load configuration: %v", err)
	}

	// Setup and run API server
	server := api.NewServer(cfg)
	log.Printf("Starting RayAI Node API server on port %s", cfg.APIPort)
	if err := server.Run(); err != nil {
		log.Fatalf("Server failed: %v", err)
	}
}
