package api

import (
	"fmt"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/showwin/speedtest-go/speedtest"
	"github.com/unicornultrafoundation/subnet-rayai-node/config"
	"github.com/unicornultrafoundation/subnet-rayai-node/ray"
)

// Server represents the API server
type Server struct {
	config     *config.Config
	router     *gin.Engine
	rayService *ray.Service
	geo        *geoCache            // Add this line
	speedTest  *speedtest.Speedtest // Add this line
	bandwidth  *bandwidthCache
}

// NewServer creates a new API server
func NewServer(cfg *config.Config) *Server {
	// Create Ray service
	rayService := ray.NewService(cfg.RayBinPath)

	// Create Gin router with default middleware
	router := gin.Default()

	// Apply IP restriction middleware
	//router.Use(middleware.IPRestrictionMiddleware(cfg.AllowedIPs))

	server := &Server{
		config:     cfg,
		router:     router,
		rayService: rayService,
		speedTest:  speedtest.New(), // Initialize speedtest client
	}
	server.setupRoutes()

	server.startBackgroundResourceUpdater()

	return server
}

func (s *Server) startBackgroundResourceUpdater() {
	// Periodically update geo location every 4 hours, retry after 1m if error
	go func() {
		for {
			if _, err := s.getGeoLocation(); err != nil {
				fmt.Printf("Error fetching geolocation: %v\n", err)
				time.Sleep(time.Minute)
				continue
			}
			time.Sleep(4 * time.Hour)
		}
	}()

	// Periodically update bandwidth every 24 hours, retry after 1m if error
	go func() {
		for {
			if _, err := s.getBandwidth(); err != nil {
				fmt.Printf("Error fetching bandwidth: %v\n", err)
				time.Sleep(time.Minute)
				continue
			}
			time.Sleep(24 * time.Hour)
		}
	}()
}

// setupRoutes configures the API routes
func (s *Server) setupRoutes() {
	s.router.POST("/start/head", s.startHead)
	s.router.POST("/start/worker", s.startWorker)
	s.router.POST("/stop", s.stopNode)
	s.router.GET("/status", s.getStatus)
	s.router.GET("/resources", s.getResources)
}

// Run starts the API server
func (s *Server) Run() error {
	return s.router.Run(fmt.Sprintf(":%s", s.config.APIPort))
}
