package middleware

import (
	"net"
	"net/http"
	"strings"

	"github.com/gin-gonic/gin"
)

// IPRestrictionMiddleware creates a middleware that restricts access to specific IPs
func IPRestrictionMiddleware(allowedIPs []string) gin.HandlerFunc {
	return func(c *gin.Context) {
		// Get client IP address
		clientIP := getClientIP(c)

		// Check if client IP is allowed
		allowed := false
		for _, ip := range allowedIPs {
			if ip == clientIP || ip == "*" { // Allow all if "*" is in the list
				allowed = true
				break
			}

			// Check for CIDR notation
			if strings.Contains(ip, "/") {
				_, ipNet, err := net.ParseCIDR(ip)
				if err == nil {
					clientIPAddr := net.ParseIP(clientIP)
					if ipNet.Contains(clientIPAddr) {
						allowed = true
						break
					}
				}
			}
		}

		if !allowed {
			c.JSON(http.StatusForbidden, gin.H{
				"error": "Access denied. Your IP is not allowed to access this API.",
			})
			c.Abort()
			return
		}

		c.Next()
	}
}

// getClientIP extracts the client IP address from the request
func getClientIP(c *gin.Context) string {
	// Try X-Forwarded-For header first (for proxies)
	ip := c.GetHeader("X-Forwarded-For")
	if ip != "" {
		// X-Forwarded-For can contain multiple IPs, take the first one
		ips := strings.Split(ip, ",")
		if len(ips) > 0 {
			return strings.TrimSpace(ips[0])
		}
	}

	// Try X-Real-IP header next
	ip = c.GetHeader("X-Real-IP")
	if ip != "" {
		return ip
	}

	// Otherwise, use the remote address
	return c.ClientIP()
}
