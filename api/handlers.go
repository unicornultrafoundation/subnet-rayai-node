package api

import (
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/gin-gonic/gin"
)

// Request and response structs
type StartHeadRequest struct{}

type StartWorkerRequest struct {
	HeadIP string `json:"headIP" binding:"required"`
}

// geoCache is a simple in-memory cache for the geolocation result
type geoCache struct {
	data      map[string]interface{}
	timestamp time.Time
	mu        sync.Mutex
}

var geoCacheDuration = 4 * time.Hour // cache duration

// BandwidthInfo represents network bandwidth usage
type BandwidthInfo struct {
	RxBytes uint64 `json:"rx_bytes"`
	TxBytes uint64 `json:"tx_bytes"`
}

// BandwidthSpeedtest represents speedtest results
type BandwidthSpeedtest struct {
	DownloadBps float64 `json:"download_bps"`
	UploadBps   float64 `json:"upload_bps"`
	PingMs      float64 `json:"ping_ms"`
	Error       string  `json:"error,omitempty"`
}

// Bandwidth cache for 1 day
type bandwidthCache struct {
	data      BandwidthSpeedtest
	timestamp time.Time
	mu        sync.Mutex
}

var bandwidthCacheDuration = 24 * time.Hour

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
	DownloadBps    float64                `json:"download_bps,omitempty"`
	UploadBps      float64                `json:"upload_bps,omitempty"`
	PingMs         float64                `json:"ping_ms,omitempty"`
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

	// Get bandwidth from cache or run test if needed
	var downloadBps, uploadBps, pingMs float64
	if s.bandwidth != nil && !s.bandwidth.timestamp.IsZero() && time.Since(s.bandwidth.timestamp) < 24*time.Hour {
		downloadBps = s.bandwidth.data.DownloadBps
		uploadBps = s.bandwidth.data.UploadBps
		pingMs = s.bandwidth.data.PingMs
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
		DownloadBps:    downloadBps,
		UploadBps:      uploadBps,
		PingMs:         pingMs,
	}

	if s.geo != nil {
		resp.Geo = s.geo.data
	}

	c.JSON(http.StatusOK, resp)
}

// getGeoLocation handles requests to get node geolocation
func (s *Server) getGeoLocation() (*geoCache, error) {
	// Use a pointer to geoCache in Server struct
	if s.geo == nil {
		s.geo = &geoCache{}
	}

	s.geo.mu.Lock()
	defer s.geo.mu.Unlock()

	// If cache is valid, return cached data
	if s.geo.data != nil && time.Since(s.geo.timestamp) < geoCacheDuration {
		return s.geo, nil
	}

	resp, err := http.Get("https://ipinfo.io/json")
	if err != nil {
		return nil, errors.New("failed to get geolocation")
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, errors.New("failed to read geolocation response")
	}

	var geo map[string]interface{}
	if err := json.Unmarshal(body, &geo); err != nil {
		return nil, errors.New("failed to parse geolocation response")
	}

	// Cache the result with timestamp
	s.geo.data = geo
	s.geo.timestamp = time.Now()

	return s.geo, nil
}

// getBandwidth handles requests to get network bandwidth using Go speedtest library
// To save bandwidth, you can skip the upload test or use a smaller download/upload size if supported by the library.
// Here, we only run the ping and download test (skip upload).
func (s *Server) getBandwidth() (*bandwidthCache, error) {
	if s.bandwidth == nil {
		s.bandwidth = &bandwidthCache{}
	}

	s.bandwidth.mu.Lock()
	defer s.bandwidth.mu.Unlock()

	// If cache is valid, return cached data
	if !s.bandwidth.timestamp.IsZero() && time.Since(s.bandwidth.timestamp) < bandwidthCacheDuration {
		return s.bandwidth, nil
	}

	serverList, _ := s.speedTest.FetchServers()
	targets, _ := serverList.FindServer([]int{})
	if len(targets) == 0 {
		return nil, errors.New("no speedtest server found")
	}
	server := targets[0]
	if err := server.PingTest(func(latency time.Duration) {}); err != nil {
		return nil, errors.New("ping test failed")
	}
	if err := server.DownloadTest(); err != nil {
		return nil, errors.New("download test failed")
	}
	// Skip upload test to save bandwidth
	result := BandwidthSpeedtest{
		DownloadBps: float64(server.DLSpeed), // bytes/sec
		UploadBps:   0,                       // not tested
		PingMs:      server.Latency.Seconds() * 1000,
	}
	// Cache the result with timestamp
	s.bandwidth.data = result
	s.bandwidth.timestamp = time.Now()

	return s.bandwidth, nil
}
