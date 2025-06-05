package api

import (
	"net/http"
	"strings"

	"github.com/gin-gonic/gin"
)

// Request and response structs
type StartHeadRequest struct{}

type StartWorkerRequest struct {
	HeadIP string `json:"headIP" binding:"required"`
}

// ResourcesWithExtra represents node resources with additional information
type ResourcesWithExtra struct {
	CPUCount       int                    `json:"cpu"`
	CPUName        string                 `json:"cpu_name,omitempty"`
	MemoryTotal    uint64                 `json:"memory_total"`
	MemoryFree     uint64                 `json:"memory_free"`
	DiskTotal      uint64                 `json:"disk_total"`
	DiskFree       uint64                 `json:"disk_free"`
	GPUCount       int                    `json:"gpu"`
	GPUModel       string                 `json:"gpu_model,omitempty"`
	GPUMemoryTotal uint64                 `json:"gpu_memory_total,omitempty"`
	GPUMemoryFree  uint64                 `json:"gpu_memory_free,omitempty"`
	DownloadMbps   float64                `json:"download_mbps,omitempty"`
	UploadMbps     float64                `json:"upload_mbps,omitempty"`
	PingMs         int64                  `json:"ping_ms,omitempty"`
	Region         string                 `json:"region,omitempty"`
	Geo            map[string]interface{} `json:"geo,omitempty"`
}

// startHead handles requests to start a Ray head node
func (s *Server) startHead(c *gin.Context) {
	var req StartHeadRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	// Implement direct Ray head node startup logic
	id, err := s.rayService.StartHead()
	if err != nil {
		status := http.StatusInternalServerError

		// Check for specific error messages to determine appropriate status code
		if strings.Contains(err.Error(), "already running") {
			status = http.StatusConflict // 409 Conflict
		}

		c.JSON(status, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"status": "Ray head node started successfully",
		"id":     id,
	})
}

// startWorker handles requests to start a Ray worker node
func (s *Server) startWorker(c *gin.Context) {
	var req StartWorkerRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	// Implement direct Ray worker node startup logic
	id, err := s.rayService.StartWorker(req.HeadIP)
	if err != nil {
		status := http.StatusInternalServerError

		// Check for specific error messages to determine appropriate status code
		if strings.Contains(err.Error(), "already running") {
			status = http.StatusConflict // 409 Conflict
		}

		c.JSON(status, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"status": "Ray worker node started successfully",
		"headIP": req.HeadIP,
		"id":     id,
	})
}

// stopNode handles requests to stop Ray nodes
func (s *Server) stopNode(c *gin.Context) {
	if err := s.rayService.StopNode(); err != nil {
		status := http.StatusInternalServerError

		// Check for specific error messages to determine appropriate status code
		if strings.Contains(err.Error(), "not running") {
			status = http.StatusPreconditionFailed // 412 Precondition Failed
		}

		c.JSON(status, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"status": "All Ray nodes stopped successfully",
	})
}

// getStatus handles requests to get Ray cluster status
func (s *Server) getStatus(c *gin.Context) {
	status, err := s.rayService.GetStatus()
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"status": status,
	})
}

// getResources handles requests to get node resource information
func (s *Server) getResources(c *gin.Context) {
	// Get base resources
	res, err := s.rayService.GetResources()
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	// Get geo location
	geo, err := s.resourceMgr.GetGeoLocation()
	if err != nil {
		// Log but don't fail if geo can't be retrieved
		geo = nil
	}

	// Get bandwidth information
	bandwidth, err := s.resourceMgr.GetBandwidth(false)
	if err != nil {
		// Log but don't fail if bandwidth can't be retrieved
		bandwidth = nil
	}

	// Compose response
	resp := ResourcesWithExtra{
		CPUCount:       res.CPUCount,
		CPUName:        res.CPUName,
		MemoryTotal:    res.MemoryTotal,
		MemoryFree:     res.MemoryFree,
		DiskTotal:      res.DiskTotal,
		DiskFree:       res.DiskFree,
		GPUCount:       res.GPUCount,
		GPUModel:       res.GPUModel,
		GPUMemoryTotal: res.GPUMemoryTotal,
		GPUMemoryFree:  res.GPUMemoryFree,
	}

	// Add geo info if available
	if geo != nil {
		resp.Region = geo.Region
		// Convert geo to map for backward compatibility
		geoMap := map[string]interface{}{
			"ip":        geo.IP,
			"city":      geo.City,
			"region":    geo.Region,
			"country":   geo.Country,
			"latitude":  geo.Latitude,
			"longitude": geo.Longitude,
		}
		resp.Geo = geoMap
	}

	// Add bandwidth info if available
	if bandwidth != nil {
		resp.DownloadMbps = bandwidth.DownloadMbps
		resp.UploadMbps = bandwidth.UploadMbps
		resp.PingMs = bandwidth.PingMs
	}

	c.JSON(http.StatusOK, resp)
}

// getGeo handles requests to get node geolocation
func (s *Server) getGeo(c *gin.Context) {
	geo, err := s.resourceMgr.GetGeoLocation()
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, geo)
}

// getBandwidth handles requests to get network bandwidth
func (s *Server) getBandwidth(c *gin.Context) {
	forceUpdate := c.Query("force") == "true"
	bandwidth, err := s.resourceMgr.GetBandwidth(forceUpdate)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, bandwidth)
}
