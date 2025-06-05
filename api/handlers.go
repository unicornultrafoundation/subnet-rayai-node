package api

import (
	"net/http"

	"github.com/gin-gonic/gin"
)

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
