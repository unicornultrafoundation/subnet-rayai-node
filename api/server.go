package api

import (
	"fmt"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/unicornultrafoundation/subnet-rayai-node/config"
	"github.com/unicornultrafoundation/subnet-rayai-node/ray"
	"github.com/unicornultrafoundation/subnet-rayai-node/resource"
)

// Server represents the API server
type Server struct {
	config      *config.Config
	router      *gin.Engine
	rayService  *ray.Service
	resourceMgr *resource.Manager
}

// NewServer creates a new API server
func NewServer(cfg *config.Config) *Server {
	// Create Ray service
	rayService := ray.NewService(cfg)

	// Create Resource Manager
	resourceMgr := resource.NewManager(cfg, rayService)

	// Create Gin router with default middleware
	router := gin.Default()

	server := &Server{
		config:      cfg,
		router:      router,
		rayService:  rayService,
		resourceMgr: resourceMgr,
	}
	server.setupRoutes()

	// Start the resource manager background updater
	resourceMgr.StartBackgroundUpdater()

	// Start the resource manager heartbeat
	resourceMgr.StartHeartbeat(time.Minute)

	return server
}

// setupRoutes configures the API routes
func (s *Server) setupRoutes() {
	s.router.GET("/status", s.getStatus)
}

// Run starts the API server
func (s *Server) Run() error {
	return s.router.Run(fmt.Sprintf(":%s", s.config.APIPort))
}
