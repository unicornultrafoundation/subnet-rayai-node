package api

import (
	"github.com/gin-gonic/gin"
	"github.com/unicornultrafoundation/subnet-rayai-node/internal/middleware"
)

// IPRestrictionMiddleware creates a middleware that restricts access to specific IPs
// Deprecated: Use middleware.IPRestrictionMiddleware instead
func IPRestrictionMiddleware(allowedIPs []string) gin.HandlerFunc {
	return middleware.IPRestrictionMiddleware(allowedIPs)
}
