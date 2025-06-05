package ray

import (
	"bytes"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"os"
	"os/exec"
	"runtime"
	"strconv"
	"strings"
	"syscall"
	"time"

	"github.com/unicornultrafoundation/subnet-rayai-node/config"
)

// NodeRole represents the role of a Ray node
type NodeRole string

const (
	RoleHead   NodeRole = "head"
	RoleWorker NodeRole = "worker"
	RoleNone   NodeRole = "none"
)

// RoleInfo contains information about the node's role in the Ray cluster
type RoleInfo struct {
	Role   NodeRole `json:"role"`
	HeadIP string   `json:"head_ip,omitempty"` // Only set for worker nodes
}

// Service manages Ray processes on the local system
type Service struct {
	binPath   string
	managerIP string
	client    *http.Client
}

// NewService creates a new Ray service manager
func NewService(cfg *config.Config) *Service {
	if cfg.RayBinPath == "" {
		cfg.RayBinPath = "ray" // Use ray from PATH if not specified
	}

	service := &Service{
		binPath:   cfg.RayBinPath,
		managerIP: cfg.ManagerIP,
		client: &http.Client{
			Timeout: 10 * time.Second,
		},
	}

	// Start periodic role setup if requested
	if cfg.ManagerIP != "" {
		service.StartPeriodicRoleSetup()
	}

	return service
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

// GetRole queries the manager to determine this node's role (head or worker)
func (s *Service) GetRole() (*RoleInfo, error) {
	if s.managerIP == "" {
		// If no manager is configured, assume head node by default
		return &RoleInfo{Role: RoleHead}, nil
	}

	// Get local hostname to uniquely identify this node
	hostname, err := os.Hostname()
	if err != nil {
		hostname = "unknown"
	}

	// Get resource information to send to manager for role decision
	resources, err := s.GetResources()
	if err != nil {
		return nil, fmt.Errorf("failed to get resource information: %w", err)
	}

	// Prepare request body
	requestBody, err := json.Marshal(map[string]interface{}{
		"hostname":  hostname,
		"resources": resources,
	})
	if err != nil {
		return nil, fmt.Errorf("failed to marshal request: %w", err)
	}

	// Call manager API to get role assignment
	url := fmt.Sprintf("http://%s/api/node/role", s.managerIP)
	req, err := http.NewRequest("POST", url, bytes.NewBuffer(requestBody))
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := s.client.Do(req)
	if err != nil {
		// If manager is unreachable, default to head role for resilience
		log.Printf("Failed to reach manager at %s: %v, defaulting to head node role", s.managerIP, err)
		return &RoleInfo{Role: RoleHead}, nil
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		// If manager returns an error, log it and default to head role
		log.Printf("Manager returned status %d, defaulting to head node role", resp.StatusCode)
		return &RoleInfo{Role: RoleHead}, nil
	}

	// Parse the response
	var roleInfo RoleInfo
	if err := json.NewDecoder(resp.Body).Decode(&roleInfo); err != nil {
		return nil, fmt.Errorf("failed to decode response: %w", err)
	}

	// Validate the role (now including RoleNone as valid)
	if roleInfo.Role != RoleHead && roleInfo.Role != RoleWorker && roleInfo.Role != RoleNone {
		return nil, fmt.Errorf("invalid role received from manager: %s", roleInfo.Role)
	}

	// If worker role but no head IP specified, return error
	if roleInfo.Role == RoleWorker && roleInfo.HeadIP == "" {
		return nil, fmt.Errorf("received worker role from manager but no head IP was provided")
	}

	log.Printf("Node assigned role: %s", roleInfo.Role)
	if roleInfo.Role == RoleWorker {
		log.Printf("Will connect to head node at: %s", roleInfo.HeadIP)
	} else if roleInfo.Role == RoleNone {
		log.Printf("No role assigned, node will be stopped if running")
	}

	return &roleInfo, nil
}

// SetupNodeByRole automatically sets up the node based on its assigned role
func (s *Service) SetupNodeByRole() (string, error) {
	// Get the node's role from manager
	roleInfo, err := s.GetRole()
	if err != nil {
		return "", fmt.Errorf("failed to determine node role: %w", err)
	}

	// Handle the case of no assigned role
	if roleInfo.Role == RoleNone {
		// If Ray is running, stop it
		if s.IsRunning() {
			log.Printf("No role assigned, stopping Ray")
			if err := s.StopNode(); err != nil {
				return "", fmt.Errorf("failed to stop Ray: %w", err)
			}

			// Clear data
			if err := s.ClearRayData(); err != nil {
				log.Printf("Warning: failed to clear Ray data: %v", err)
				// Continue despite errors
			}

			return "stopped", nil
		}
		return "idle", nil
	}

	// Check if Ray is already running
	if s.IsRunning() {
		return "", fmt.Errorf("ray is already running, please stop it first")
	}

	// Set up based on role
	switch roleInfo.Role {
	case RoleHead:
		return s.StartHead()
	case RoleWorker:
		if roleInfo.HeadIP == "" {
			return "", fmt.Errorf("cannot start worker: no head IP provided")
		}
		return s.StartWorker(roleInfo.HeadIP)
	default:
		return "", fmt.Errorf("unknown role: %s", roleInfo.Role)
	}
}

// StartHead starts a Ray head node
func (s *Service) StartHead() (string, error) {
	// Check if Ray is already running
	if s.IsRunning() {
		return "", fmt.Errorf("ray is already running, please stop it first")
	}

	args := []string{
		"start",
		"--head",
		"--port=6379",
		"--dashboard-host=0.0.0.0",
	}

	cmd := exec.Command(s.binPath, args...)

	output, err := cmd.CombinedOutput()
	if err != nil {
		return "", fmt.Errorf("failed to start Ray head node: %w, output: %s", err, string(output))
	}

	// Extract process ID or use port as identifier
	id := fmt.Sprintf("head-%d", 6379)
	log.Printf("Started Ray head node on port %d", 6379)

	return id, nil
}

// StartWorker starts a Ray worker node
func (s *Service) StartWorker(headIP string) (string, error) {
	// Check if Ray is already running locally
	if s.IsRunning() {
		return "", fmt.Errorf("ray is already running locally, please stop it first")
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
		return fmt.Errorf("ray is not running, nothing to stop")
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

type Resources struct {
	CPUCount       int    `json:"cpu"`
	CPUName        string `json:"cpu_name,omitempty"`
	MemoryTotal    uint64 `json:"memory_total"`
	MemoryFree     uint64 `json:"memory_free"`
	DiskTotal      uint64 `json:"disk_total"`
	DiskFree       uint64 `json:"disk_free"`
	GPUCount       int    `json:"gpu"`
	GPUModel       string `json:"gpu_model,omitempty"`
	GPUMemoryTotal uint64 `json:"gpu_memory_total,omitempty"` // MiB
	GPUMemoryFree  uint64 `json:"gpu_memory_free,omitempty"`  // MiB
}

// GetResources returns node resource information (CPU, RAM, Disk, GPU)
func (s *Service) GetResources() (*Resources, error) {
	// CPU
	cpuCount := runtime.NumCPU()

	// Memory
	var memStats syscall.Sysinfo_t
	memTotal := uint64(0)
	memFree := uint64(0)
	if err := syscall.Sysinfo(&memStats); err == nil {
		memTotal = memStats.Totalram * uint64(memStats.Unit)
		memFree = memStats.Freeram * uint64(memStats.Unit)
	}

	// Disk
	var stat syscall.Statfs_t
	diskTotal := uint64(0)
	diskFree := uint64(0)
	if err := syscall.Statfs("/data", &stat); err == nil {
		diskTotal = stat.Blocks * uint64(stat.Bsize)
		diskFree = stat.Bfree * uint64(stat.Bsize)
	}

	// GPU (try nvidia-smi, fallback to 0)
	gpuCount := 0
	gpuModel := ""
	var gpuMemTotal uint64 = 0
	var gpuMemFree uint64 = 0

	// Use nvidia-smi to query GPU name, total memory, and free memory (in MiB)
	cmd := exec.Command("nvidia-smi", "--query-gpu=name,memory.total,memory.free", "--format=csv,noheader,nounits")
	var out bytes.Buffer
	cmd.Stdout = &out
	if err := cmd.Run(); err == nil {
		lines := bytes.Split(bytes.TrimSpace(out.Bytes()), []byte("\n"))
		gpuCount = len(lines)
		if gpuCount > 0 {
			// Only extract info from the first GPU (if multiple GPUs exist)
			parts := bytes.Split(lines[0], []byte(","))
			if len(parts) >= 3 {
				gpuModel = string(bytes.TrimSpace(parts[0]))
				// memory.total and memory.free are in MiB (no "MiB" suffix due to nounits)
				if mt, err := parseUint64(bytes.TrimSpace(parts[1])); err == nil {
					gpuMemTotal = mt
				}
				if mf, err := parseUint64(bytes.TrimSpace(parts[2])); err == nil {
					gpuMemFree = mf
				}
			}
		}
	}

	// Get CPU name (Linux only)
	cpuName := ""
	if data, err := os.ReadFile("/proc/cpuinfo"); err == nil {
		lines := bytes.Split(data, []byte("\n"))
		for _, line := range lines {
			if bytes.HasPrefix(line, []byte("model name")) {
				parts := bytes.SplitN(line, []byte(":"), 2)
				if len(parts) == 2 {
					cpuName = strings.TrimSpace(string(parts[1]))
					break
				}
			}
		}
	}

	return &Resources{
		CPUCount:       cpuCount,
		CPUName:        cpuName,
		MemoryTotal:    memTotal,
		MemoryFree:     memFree,
		DiskTotal:      diskTotal,
		DiskFree:       diskFree,
		GPUCount:       gpuCount,
		GPUModel:       gpuModel,
		GPUMemoryTotal: gpuMemTotal,
		GPUMemoryFree:  gpuMemFree,
	}, nil

}

// parseUint64 parses a byte slice to uint64
func parseUint64(b []byte) (uint64, error) {
	return strconv.ParseUint(string(b), 10, 64)
}

// ClearRayData removes Ray session temporary files
func (s *Service) ClearRayData() error {
	// Ray stores session data in /tmp/ray
	rayDataDir := "/tmp/ray"

	// Check if directory exists
	_, err := os.Stat(rayDataDir)
	if os.IsNotExist(err) {
		// If directory doesn't exist, nothing to clean
		return nil
	}

	// Execute rm command for safety (more controlled than os.RemoveAll)
	cmd := exec.Command("rm", "-rf", rayDataDir)
	output, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("failed to clear Ray data: %w, output: %s", err, string(output))
	}

	log.Printf("Cleared Ray data directory: %s", rayDataDir)
	return nil
}

// Add a new method to start a periodic role checker

// StartPeriodicRoleSetup starts a background goroutine that checks and sets up
// the node's role every minute
func (s *Service) StartPeriodicRoleSetup() {
	go func() {
		// Initial setup without delay
		if _, err := s.SetupNodeByRole(); err != nil {
			log.Printf("Initial role setup failed: %v", err)
		}

		// Setup ticker for periodic checks
		ticker := time.NewTicker(time.Minute)
		defer ticker.Stop()

		for range ticker.C {
			log.Printf("Periodic role check running...")

			// Check current role
			currentRole, err := s.GetRole()
			if err != nil {
				log.Printf("Failed to get current role: %v", err)
				continue
			}

			// Setup node according to the current role
			result, err := s.SetupNodeByRole()
			if err != nil {
				log.Printf("Role setup failed: %v", err)
				continue
			}

			log.Printf("Role check completed: %s (%s)", currentRole.Role, result)
		}
	}()

	log.Println("Started periodic role checking (every 1 minute)")
}
