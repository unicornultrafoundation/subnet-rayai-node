package config

import (
	"os"
	"strconv"
	"strings"
)

// Config holds the application configuration
type Config struct {
	APIPort     string
	RayBinPath  string
	LogLevel    string
	AllowedIPs  []string
	RayHeadPort int // New field for Ray head node port
}

// Load returns a Config struct populated from the environment
func Load() (*Config, error) {
	// Default configuration
	config := &Config{
		APIPort:     getEnv("API_PORT", "3333"),    // Changed default from 8080 to 3333
		RayBinPath:  getEnv("RAY_BIN_PATH", "ray"), // Default to system PATH
		LogLevel:    getEnv("LOG_LEVEL", "info"),
		AllowedIPs:  parseAllowedIPs(getEnv("ALLOWED_IPS", "127.0.0.1")),
		RayHeadPort: getEnvAsInt("RAY_HEAD_PORT", 6379), // Default Ray port
	}

	return config, nil
}

// Helper functions for environment variables
func getEnv(key, defaultValue string) string {
	if value, exists := os.LookupEnv(key); exists {
		return value
	}
	return defaultValue
}

// getEnvAsInt parses an environment variable as an integer
func getEnvAsInt(key string, defaultValue int) int {
	if value, exists := os.LookupEnv(key); exists {
		if intValue, err := strconv.Atoi(value); err == nil {
			return intValue
		}
	}
	return defaultValue
}

// parseAllowedIPs parses comma-separated IPs into a slice
func parseAllowedIPs(ips string) []string {
	if ips == "" {
		return []string{"127.0.0.1"} // Default to localhost
	}
	return strings.Split(ips, ",")
}
