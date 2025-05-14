package ray

import (
	"fmt"
	"log"
	"os/exec"
	"strings"
)

// Service manages Ray processes on the local system
type Service struct {
	binPath string
}

// NewService creates a new Ray service manager
func NewService(rayBinPath string) *Service {
	if rayBinPath == "" {
		rayBinPath = "ray" // Use ray from PATH if not specified
	}
	return &Service{
		binPath: rayBinPath,
	}
}

// IsRunning checks if any Ray node is currently running
func (s *Service) IsRunning() bool {
	cmd := exec.Command(s.binPath, "status")
	output, err := cmd.CombinedOutput()
	if err != nil {
		// If we get an error, assume Ray is not running
		return false
	}

	// Check output for indicators that Ray is running
	outputStr := string(output)
	return strings.Contains(outputStr, "Ray runtime started") ||
		strings.Contains(outputStr, "ClientServerID:") ||
		!strings.Contains(outputStr, "Ray is not running")
}

// StartHead starts a Ray head node
func (s *Service) StartHead(port int, username, password string) (string, error) {
	// Check if Ray is already running
	if s.IsRunning() {
		return "", fmt.Errorf("Ray is already running, please stop it first")
	}

	args := []string{
		"start",
		"--head",
		fmt.Sprintf("--port=%d", port),
		"--dashboard-host=0.0.0.0",
	}

	// Set credentials if provided
	if username != "" && password != "" {
		// Note: Ray doesn't directly support username/password in CLI
		// This is a placeholder for custom auth implementation
		log.Printf("Using credentials for Ray head: %s", username)
	}

	cmd := exec.Command(s.binPath, args...)

	output, err := cmd.CombinedOutput()
	if err != nil {
		return "", fmt.Errorf("failed to start Ray head node: %w, output: %s", err, string(output))
	}

	// Extract process ID or use port as identifier
	id := fmt.Sprintf("head-%d", port)
	log.Printf("Started Ray head node on port %d", port)

	return id, nil
}

// StartWorker starts a Ray worker node
func (s *Service) StartWorker(headIP string, username, password string) (string, error) {
	// Check if Ray is already running locally
	if s.IsRunning() {
		return "", fmt.Errorf("Ray is already running locally, please stop it first")
	}

	args := []string{
		"start",
		"--address", fmt.Sprintf("%s:6379", headIP),
	}

	cmd := exec.Command(s.binPath, args...)

	output, err := cmd.CombinedOutput()
	if err != nil {
		return "", fmt.Errorf("failed to start Ray worker node: %w, output: %s", err, string(output))
	}

	// Generate a session ID for this worker
	id := fmt.Sprintf("worker-%s", strings.Replace(headIP, ".", "-", -1))
	log.Printf("Started Ray worker node connecting to %s", headIP)

	return id, nil
}

// StopNode stops all Ray nodes (head and workers)
func (s *Service) StopNode() error {
	// Check if Ray is running before trying to stop it
	if !s.IsRunning() {
		return fmt.Errorf("Ray is not running, nothing to stop")
	}

	cmd := exec.Command(s.binPath, "stop")
	output, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("failed to stop Ray nodes: %w, output: %s", err, string(output))
	}

	log.Printf("Stopped all Ray nodes")
	return nil
}

// GetStatus returns the status of Ray cluster
func (s *Service) GetStatus() (string, error) {
	cmd := exec.Command(s.binPath, "status")
	output, err := cmd.CombinedOutput()
	if err != nil {
		return "", fmt.Errorf("failed to get Ray status: %w", err)
	}

	return string(output), nil
}
