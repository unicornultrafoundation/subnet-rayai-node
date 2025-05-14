package api

import (
	"net/http"
	"strings"

	"github.com/gin-gonic/gin"
)

// Request and response structs
type StartHeadRequest struct {
	Username string `json:"username" binding:"required"`
	Password string `json:"password" binding:"required"`
}

type StartWorkerRequest struct {
	HeadIP   string `json:"headIP" binding:"required"`
	Username string `json:"username" binding:"required"`
	Password string `json:"password" binding:"required"`
}

// startHead handles requests to start a Ray head node
func (s *Server) startHead(c *gin.Context) {
	var req StartHeadRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	// Using port from server configuration instead of request
	port := s.config.RayHeadPort

	// Implement direct Ray head node startup logic
	id, err := s.rayService.StartHead(port, req.Username, req.Password)
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
		"port":   port,
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
	id, err := s.rayService.StartWorker(req.HeadIP, req.Username, req.Password)
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
