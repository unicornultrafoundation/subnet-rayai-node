package api

import (
	"fmt"

	"github.com/gin-gonic/gin"
	"github.com/unicornultrafoundation/subnet-rayai-node/config"
	"github.com/unicornultrafoundation/subnet-rayai-node/internal/middleware"
	"github.com/unicornultrafoundation/subnet-rayai-node/ray"
)

// Server represents the API server
type Server struct {
	config     *config.Config
	router     *gin.Engine
	rayService *ray.Service
}

// NewServer creates a new API server
func NewServer(cfg *config.Config) *Server {
	// Create Ray service
	rayService := ray.NewService(cfg.RayBinPath)

	// Create Gin router with default middleware
	router := gin.Default()

	// Apply IP restriction middleware
	router.Use(middleware.IPRestrictionMiddleware(cfg.AllowedIPs))

	server := &Server{
		config:     cfg,
		router:     router,
		rayService: rayService,
	}
	server.setupRoutes()
	return server
}

// setupRoutes configures the API routes
func (s *Server) setupRoutes() {
	s.router.POST("/start/head", s.startHead)
	s.router.POST("/start/worker", s.startWorker)
	s.router.POST("/stop", s.stopNode) // Changed from /stop/:id to /stop
	s.router.GET("/status", s.getStatus)
}

// Run starts the API server
func (s *Server) Run() error {
	return s.router.Run(fmt.Sprintf(":%s", s.config.APIPort))
}
