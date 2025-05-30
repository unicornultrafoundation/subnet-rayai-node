package ray

import (
	"bytes"
	"fmt"
	"log"
	"os"
	"os/exec"
	"runtime"
	"strconv"
	"strings"
	"syscall"
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
